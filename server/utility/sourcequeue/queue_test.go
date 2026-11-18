package sourcequeue

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/database/gredis"
)

type testQueue struct {
	q       *Queue
	advance func(time.Duration)
}

// No project configuration is read. With no env, all scripts run against a
// temporary in-process Redis. An explicit env also exercises a real localhost
// Redis (DB 0, no password); remote addresses and hostnames are rejected.
func setupQueue(t *testing.T) testQueue {
	t.Helper()
	address := os.Getenv("SOURCEQUEUE_TEST_REDIS_ADDR")
	advance := func(d time.Duration) { time.Sleep(d) }
	if address == "" {
		server := miniredis.RunT(t)
		address = server.Addr()
		now := time.Now()
		server.SetTime(now)
		advance = func(d time.Duration) {
			now = now.Add(d)
			server.SetTime(now)
			server.FastForward(d)
		}
	} else {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			t.Fatal(e)
		}
		ip := net.ParseIP(host)
		n, e := strconv.Atoi(port)
		if ip == nil || !ip.IsLoopback() || e != nil || n <= 0 || n > 65535 {
			t.Fatal("SOURCEQUEUE_TEST_REDIS_ADDR must be explicit loopback IP:port")
		}
	}
	client, e := gredis.New(&gredis.Config{Address: address, Db: 0})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = client.Close(context.Background()) })
	q, e := newQueue(fmt.Sprintf("test:%s:%d", t.Name(), time.Now().UnixNano()), client)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		// Delete only deterministically known test keys, never enumerate the DB.
		for _, lane := range []Lane{Realtime, History} {
			for tenant := int64(1); tenant <= 2; tenant++ {
				for source := int64(1); source <= 4; source++ {
					keys := q.keys(Scope{tenant, source}, lane)
					keys = append(keys, keys[1]+":recover-turn", keys[8]+":budget")
					args := make([]interface{}, len(keys))
					for i, k := range keys {
						args[i] = k
					}
					_, _ = client.Do(context.Background(), "DEL", args...)
				}
			}
		}
	})
	return testQueue{q, advance}
}
func mustEnqueue(t *testing.T, q *Queue, s Scope, l Lane, id string, p []byte) {
	t.Helper()
	if e := q.Enqueue(context.Background(), s, l, id, p); e != nil {
		t.Fatal(e)
	}
}
func mustClaim(t *testing.T, q *Queue, l Lane) *Claim {
	t.Helper()
	c, e := q.Claim(context.Background(), l, 5*time.Second)
	if e != nil || c == nil {
		t.Fatalf("claim=%+v err=%v", c, e)
	}
	return c
}
func mustEmpty(t *testing.T, q *Queue, l Lane) {
	t.Helper()
	c, e := q.Claim(context.Background(), l, time.Second)
	if e != nil || c != nil {
		t.Fatalf("want empty: %+v %v", c, e)
	}
}
func mustAck(t *testing.T, q *Queue, c *Claim) {
	t.Helper()
	if e := q.Ack(context.Background(), c); e != nil {
		t.Fatal(e)
	}
}

func TestQueueFairnessAndLaneIsolation(t *testing.T) {
	r := setupQueue(t)
	q := r.q
	a, b := Scope{1, 1}, Scope{1, 2}
	for i := 0; i < 3; i++ {
		mustEnqueue(t, q, a, Realtime, fmt.Sprint(i), nil)
	}
	mustEnqueue(t, q, b, Realtime, "other", nil)
	mustEnqueue(t, q, a, History, "history", nil)
	first := mustClaim(t, q, Realtime)
	if first.Scope != a {
		t.Fatal(first)
	}
	second := mustClaim(t, q, Realtime)
	if second.Scope != b {
		t.Fatal(second)
	}
	mustEmpty(t, q, Realtime) // A still owns its sole realtime slot.
	historical := mustClaim(t, q, History)
	mustAck(t, q, historical)
	mustAck(t, q, first)
	mustAck(t, q, second)
	for i := 1; i < 3; i++ {
		c := mustClaim(t, q, Realtime)
		if c.ID != fmt.Sprint(i) {
			t.Fatal(c)
		}
		mustAck(t, q, c)
	}
	mustEmpty(t, q, Realtime)
}
func TestQueueEnqueueDuringClaimSurvivesOldAck(t *testing.T) {
	q := setupQueue(t).q
	s := Scope{1, 1}
	mustEnqueue(t, q, s, Realtime, "same", []byte("old"))
	old := mustClaim(t, q, Realtime)
	mustEnqueue(t, q, s, Realtime, "same", []byte{0, 1, 255})
	mustEnqueue(t, q, s, Realtime, "next", nil)
	mustEmpty(t, q, Realtime) // Enqueue never resets inflight.
	mustAck(t, q, old)
	next := mustClaim(t, q, Realtime)
	if next.ID != "next" {
		t.Fatal(next)
	}
	mustAck(t, q, next)
	current := mustClaim(t, q, Realtime)
	if string(current.Payload) != string([]byte{0, 1, 255}) {
		t.Fatal(current)
	}
	if e := q.Ack(context.Background(), old); !errors.Is(e, ErrLeaseLost) {
		t.Fatal(e)
	}
	mustAck(t, q, current)
	mustEmpty(t, q, Realtime)
	mustEnqueue(t, q, s, Realtime, "same", []byte("after ack"))
	mustAck(t, q, mustClaim(t, q, Realtime))
}
func TestQueueDelayedTaskDoesNotOccupySource(t *testing.T) {
	r := setupQueue(t)
	q := r.q
	s := Scope{1, 1}
	ctx := context.Background()
	if e := q.EnqueueAfter(ctx, s, Realtime, "wait", []byte("media"), time.Second); e != nil {
		t.Fatal(e)
	}
	mustEnqueue(t, q, s, Realtime, "live", nil)
	c := mustClaim(t, q, Realtime)
	if c.ID != "live" {
		t.Fatal(c)
	}
	mustAck(t, q, c)
	if e := q.Recover(ctx, Realtime, 1); e != nil {
		t.Fatal(e)
	}
	mustEmpty(t, q, Realtime)
	r.advance(1100 * time.Millisecond)
	if e := q.Recover(ctx, Realtime, 1); e != nil {
		t.Fatal(e)
	}
	c = mustClaim(t, q, Realtime)
	if c.ID != "wait" {
		t.Fatal(c)
	}
	mustAck(t, q, c)
}
func TestQueueDelayedGenerationSurvivesAck(t *testing.T) {
	r := setupQueue(t)
	q := r.q
	s := Scope{1, 1}
	ctx := context.Background()
	mustEnqueue(t, q, s, Realtime, "same", nil)
	c := mustClaim(t, q, Realtime)
	if e := q.EnqueueAfter(ctx, s, Realtime, "same", []byte("new"), time.Second); e != nil {
		t.Fatal(e)
	}
	mustAck(t, q, c)
	mustEmpty(t, q, Realtime)
	r.advance(1100 * time.Millisecond)
	if e := q.Recover(ctx, Realtime, 1); e != nil {
		t.Fatal(e)
	}
	c = mustClaim(t, q, Realtime)
	if string(c.Payload) != "new" {
		t.Fatal(c)
	}
	mustAck(t, q, c)
}
func TestQueueExpiredOwnerFencingAndLeaseOnlyRecoveryLimitOne(t *testing.T) {
	r := setupQueue(t)
	q := r.q
	s := Scope{1, 1}
	ctx := context.Background()
	mustEnqueue(t, q, s, Realtime, "crashed", nil)
	old, e := q.Claim(ctx, Realtime, 100*time.Millisecond)
	if e != nil || old == nil {
		t.Fatal(old, e)
	}
	r.advance(150 * time.Millisecond)
	for _, e := range []error{q.Renew(ctx, old, time.Second), q.Ack(ctx, old), q.Nack(ctx, old, 0)} {
		if !errors.Is(e, ErrLeaseLost) {
			t.Fatal(e)
		}
	}
	if e = q.Recover(ctx, Realtime, 1); e != nil {
		t.Fatal(e)
	}
	current := mustClaim(t, q, Realtime)
	if current.ID != old.ID || current.Token == old.Token {
		t.Fatal(current)
	}
	for _, e := range []error{q.Ack(ctx, old), q.Nack(ctx, old, 0), q.Renew(ctx, old, time.Second)} {
		if !errors.Is(e, ErrLeaseLost) {
			t.Fatal(e)
		}
	}
	mustAck(t, q, current)
}
func TestQueueRenewKeepsOwnership(t *testing.T) {
	r := setupQueue(t)
	q := r.q
	ctx := context.Background()
	mustEnqueue(t, q, Scope{1, 1}, Realtime, "renew", nil)
	c, e := q.Claim(ctx, Realtime, 200*time.Millisecond)
	if e != nil || c == nil {
		t.Fatal(c, e)
	}
	if e = q.Renew(ctx, c, time.Second); e != nil {
		t.Fatal(e)
	}
	r.advance(300 * time.Millisecond)
	if e = q.Recover(ctx, Realtime, 1); e != nil {
		t.Fatal(e)
	}
	mustEmpty(t, q, Realtime)
	mustAck(t, q, c)
}
func TestQueueNackReleasesSlotAndRetainsLatestPayload(t *testing.T) {
	r := setupQueue(t)
	q := r.q
	ctx := context.Background()
	s := Scope{1, 1}
	mustEnqueue(t, q, s, Realtime, "retry", nil)
	c := mustClaim(t, q, Realtime)
	mustEnqueue(t, q, s, Realtime, "retry", []byte("latest"))
	mustEnqueue(t, q, s, Realtime, "next", nil)
	if e := q.Nack(ctx, c, time.Second); e != nil {
		t.Fatal(e)
	}
	next := mustClaim(t, q, Realtime)
	if next.ID != "next" {
		t.Fatal(next)
	}
	mustAck(t, q, next)
	r.advance(1100 * time.Millisecond)
	if e := q.Recover(ctx, Realtime, 1); e != nil {
		t.Fatal(e)
	}
	retry := mustClaim(t, q, Realtime)
	if string(retry.Payload) != "latest" {
		t.Fatal(retry)
	}
	mustAck(t, q, retry)
}
func TestQueueConcurrentAckEnqueueCannotLoseWakeup(t *testing.T) {
	q := setupQueue(t).q
	ctx := context.Background()
	s := Scope{1, 1}
	for i := 0; i < 30; i++ {
		id := fmt.Sprint(i)
		mustEnqueue(t, q, s, Realtime, id, []byte("old"))
		c := mustClaim(t, q, Realtime)
		start := make(chan struct{})
		errs := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); <-start; errs <- q.Ack(ctx, c) }()
		go func() { defer wg.Done(); <-start; errs <- q.Enqueue(ctx, s, Realtime, id, []byte("new")) }()
		close(start)
		wg.Wait()
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
		current := mustClaim(t, q, Realtime)
		if current.ID != id || string(current.Payload) != "new" {
			t.Fatal(current)
		}
		mustAck(t, q, current)
	}
}
func TestQueueConcurrentClaimsHaveSingleOwner(t *testing.T) {
	q := setupQueue(t).q
	ctx := context.Background()
	s := Scope{1, 1}
	mustEnqueue(t, q, s, Realtime, "one", nil)
	mustEnqueue(t, q, s, Realtime, "two", nil)
	var wg sync.WaitGroup
	claims := make(chan *Claim, 16)
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c, e := q.Claim(ctx, Realtime, 5*time.Second); claims <- c; errs <- e }()
	}
	wg.Wait()
	close(claims)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var owner *Claim
	for c := range claims {
		if c != nil {
			if owner != nil {
				t.Fatal("two owners")
			}
			owner = c
		}
	}
	if owner == nil {
		t.Fatal("no owner")
	}
	mustAck(t, q, owner)
	mustAck(t, q, mustClaim(t, q, Realtime))
}
func TestQueueRecoveryBoundAndNoIndexStarvation(t *testing.T) {
	r := setupQueue(t)
	q := r.q
	ctx := context.Background()
	for i := int64(1); i <= 2; i++ {
		s := Scope{1, i}
		mustEnqueue(t, q, s, Realtime, "lease", nil)
		if c, e := q.Claim(ctx, Realtime, 100*time.Millisecond); e != nil || c == nil {
			t.Fatal(c, e)
		}
		if e := q.EnqueueAfter(ctx, Scope{2, i}, Realtime, "delay", nil, 100*time.Millisecond); e != nil {
			t.Fatal(e)
		}
	}
	r.advance(150 * time.Millisecond)
	for i := 0; i < 4; i++ {
		if e := q.Recover(ctx, Realtime, 1); e != nil {
			t.Fatal(e)
		}
		c := mustClaim(t, q, Realtime)
		if i == 0 && c.ID != "lease" {
			t.Fatal("first lease missing", c)
		}
		if i == 1 && c.ID != "delay" {
			t.Fatal("delay index starved", c)
		}
		mustAck(t, q, c)
		mustEmpty(t, q, Realtime)
	}
}
func TestQueueCanceledEnqueueCanBeRetried(t *testing.T) {
	q := setupQueue(t).q
	s := Scope{1, 1}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := q.Enqueue(ctx, s, Realtime, "retry", nil); e == nil {
		t.Fatal("expected cancellation")
	}
	mustEnqueue(t, q, s, Realtime, "retry", []byte("ok"))
	mustAck(t, q, mustClaim(t, q, Realtime))
	mustEmpty(t, q, Realtime)
}
func TestQueueLostClaimResponseRecovers(t *testing.T) {
	r := setupQueue(t)
	q := r.q
	s := Scope{1, 1}
	ctx := context.Background()
	mustEnqueue(t, q, s, Realtime, "lost-response", nil)
	// Simulate server committed Claim but worker never received its response.
	if c, e := q.Claim(ctx, Realtime, 100*time.Millisecond); e != nil || c == nil {
		t.Fatal(c, e)
	}
	mustEnqueue(t, q, s, Realtime, "later", nil)
	r.advance(150 * time.Millisecond)
	if e := q.Recover(ctx, Realtime, 1); e != nil {
		t.Fatal(e)
	}
	c := mustClaim(t, q, Realtime)
	if c.ID != "lost-response" {
		t.Fatal(c)
	}
	mustAck(t, q, c)
	c = mustClaim(t, q, Realtime)
	if c.ID != "later" {
		t.Fatal(c)
	}
	mustAck(t, q, c)
}

// A's backlog length never contributes to its ready score. Run against the
// actual enqueue/claim/ack Lua scripts, not a Go scheduler model.
func TestQueueLargeBacklogCannotStarveNewSource(t *testing.T) {
	q := setupQueue(t).q
	a := Scope{TenantID: 1, SourceID: 1}
	b := Scope{TenantID: 1, SourceID: 2}
	c := Scope{TenantID: 2, SourceID: 1} // Same source number, different tenant.
	for i := 0; i < 1000; i++ {
		mustEnqueue(t, q, a, Realtime, fmt.Sprintf("backlog-%04d", i), nil)
	}
	// Same task ID is independent across BOTH scope dimensions.
	mustEnqueue(t, q, b, Realtime, "backlog-0000", []byte("B"))
	mustEnqueue(t, q, c, Realtime, "backlog-0000", []byte("C"))
	first := mustClaim(t, q, Realtime)
	if first.Scope != a {
		t.Fatalf("first scope=%+v want A", first.Scope)
	}
	mustAck(t, q, first) // A still has 999 entries and re-enters at the tail.
	second := mustClaim(t, q, Realtime)
	if second.Scope != b || string(second.Payload) != "B" {
		t.Fatalf("B starved behind A backlog: %+v", second)
	}
	mustAck(t, q, second)
	third := mustClaim(t, q, Realtime)
	if third.Scope != c || string(third.Payload) != "C" {
		t.Fatalf("cross-tenant source collision/starvation: %+v", third)
	}
	mustAck(t, q, third)
	nextA := mustClaim(t, q, Realtime)
	if nextA.Scope != a || nextA.ID != "backlog-0001" {
		t.Fatalf("A lost its independent backlog: %+v", nextA)
	}
	mustAck(t, q, nextA)
}

func TestQueueHasImmediateWork(t *testing.T) {
	q := setupQueue(t).q
	ctx := context.Background()
	scope := Scope{TenantID: 1, SourceID: 1}
	assertWork := func(s Scope, lane Lane, want bool) {
		t.Helper()
		got, err := q.HasImmediateWork(ctx, s, lane)
		if err != nil || got != want {
			t.Fatalf("HasImmediateWork(%+v,%s)=%v,%v want %v", s, lane, got, err, want)
		}
	}
	assertWork(scope, Realtime, false)
	mustEnqueue(t, q, scope, Realtime, "live", nil)
	assertWork(scope, Realtime, true) // Pending list contains the task.
	assertWork(scope, History, false) // A different lane cannot affect this answer.
	assertWork(Scope{TenantID: 1, SourceID: 2}, Realtime, false)
	assertWork(Scope{TenantID: 2, SourceID: 1}, Realtime, false)
	c := mustClaim(t, q, Realtime)
	assertWork(scope, Realtime, true) // Pending is empty; inflight still counts.
	mustAck(t, q, c)
	assertWork(scope, Realtime, false)
	if err := q.EnqueueAfter(ctx, scope, Realtime, "media-wait", nil, time.Hour); err != nil {
		t.Fatal(err)
	}
	assertWork(scope, Realtime, false) // Delayed-only work does not reserve a slot.
	mustEnqueue(t, q, Scope{TenantID: 1, SourceID: 2}, Realtime, "other-source", nil)
	mustEnqueue(t, q, Scope{TenantID: 2, SourceID: 1}, Realtime, "other-tenant", nil)
	mustEnqueue(t, q, scope, History, "historical", nil)
	assertWork(scope, Realtime, false)
	assertWork(Scope{TenantID: 1, SourceID: 2}, Realtime, true)
	assertWork(Scope{TenantID: 2, SourceID: 1}, Realtime, true)
	assertWork(scope, History, true)
}

func TestQueueHasTask(t *testing.T) {
	q := setupQueue(t).q
	ctx := context.Background()
	scope := Scope{TenantID: 1, SourceID: 1}
	const id = "event-123"
	assertTask := func(s Scope, lane Lane, taskID string, want bool) {
		t.Helper()
		got, err := q.HasTask(ctx, s, lane, taskID)
		if err != nil || got != want {
			t.Fatalf("HasTask(%+v,%s,%q)=%v,%v want %v", s, lane, taskID, got, err, want)
		}
	}
	assertTask(scope, Realtime, id, false)
	mustEnqueue(t, q, scope, Realtime, id, nil)
	assertTask(scope, Realtime, id, true) // pending
	assertTask(scope, Realtime, "missing", false)
	assertTask(scope, History, id, false)
	assertTask(Scope{TenantID: 1, SourceID: 2}, Realtime, id, false)
	assertTask(Scope{TenantID: 2, SourceID: 1}, Realtime, id, false)
	c := mustClaim(t, q, Realtime)
	assertTask(scope, Realtime, id, true) // inflight
	mustAck(t, q, c)
	assertTask(scope, Realtime, id, false)
	if err := q.EnqueueAfter(ctx, scope, Realtime, id, nil, time.Hour); err != nil {
		t.Fatal(err)
	}
	assertTask(scope, Realtime, id, true) // delayed
	assertTask(scope, History, id, false)
	mustEnqueue(t, q, scope, History, id, nil)
	historical := mustClaim(t, q, History)
	mustAck(t, q, historical)
	assertTask(scope, History, id, false)
	assertTask(scope, Realtime, id, true) // Other lane Ack cannot remove this task.
}

func TestQueueCancelScopeFencesOwnersAndPreservesOtherSources(t *testing.T) {
	q := setupQueue(t).q
	ctx := context.Background()
	scope, other, otherTenant := Scope{1, 1}, Scope{1, 2}, Scope{2, 1}
	var oldClaims []*Claim
	for _, lane := range []Lane{Realtime, History} {
		mustEnqueue(t, q, scope, lane, "old-owner", nil)
		oldClaims = append(oldClaims, mustClaim(t, q, lane))
		mustEnqueue(t, q, scope, lane, "old-pending", nil)
		if err := q.EnqueueAfter(ctx, scope, lane, "old-delayed", nil, time.Hour); err != nil {
			t.Fatal(err)
		}
		mustEnqueue(t, q, other, lane, "other-source", nil)
		mustEnqueue(t, q, otherTenant, lane, "other-tenant", nil)
	}
	if err := q.CancelScope(ctx, scope); err != nil {
		t.Fatal(err)
	}
	if err := q.CancelScope(ctx, scope); err != nil {
		t.Fatal("cancel must be idempotent", err)
	}
	for _, old := range oldClaims {
		for _, err := range []error{q.Ack(ctx, old), q.Nack(ctx, old, 0), q.Renew(ctx, old, time.Second)} {
			if !errors.Is(err, ErrLeaseLost) {
				t.Fatalf("canceled owner accepted: %v", err)
			}
		}
	}
	for _, lane := range []Lane{Realtime, History} {
		for _, id := range []string{"old-owner", "old-pending", "old-delayed"} {
			exists, err := q.HasTask(ctx, scope, lane, id)
			if err != nil || exists {
				t.Fatal(id, exists, err)
			}
		}
		for _, s := range []Scope{other, otherTenant} {
			c := mustClaim(t, q, lane)
			if c.Scope != s {
				t.Fatalf("cancel affected another scope: %+v", c)
			}
			mustAck(t, q, c)
		}
		mustEmpty(t, q, lane)
		mustEnqueue(t, q, scope, lane, "old-owner", []byte("new-generation"))
		current := mustClaim(t, q, lane)
		if string(current.Payload) != "new-generation" {
			t.Fatal(current)
		}
		for _, old := range oldClaims {
			if old.Lane == lane {
				if err := q.Ack(ctx, old); !errors.Is(err, ErrLeaseLost) {
					t.Fatal("old owner acknowledged replacement", err)
				}
			}
		}
		mustAck(t, q, current)
	}
}

func TestQueueCancelScopeOldDelayedMembersCannotDeleteReplacement(t *testing.T) {
	r := setupQueue(t)
	q := r.q
	ctx := context.Background()
	scope := Scope{1, 1}
	for _, lane := range []Lane{Realtime, History} {
		for _, id := range []string{"replace-delayed", "replace-immediate", "discard"} {
			if err := q.EnqueueAfter(ctx, scope, lane, id, []byte("old"), 40*time.Millisecond); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := q.CancelScope(ctx, scope); err != nil {
		t.Fatal(err)
	}
	for _, lane := range []Lane{Realtime, History} {
		if err := q.EnqueueAfter(ctx, scope, lane, "replace-delayed", []byte("new delayed"), 500*time.Millisecond); err != nil {
			t.Fatal(err)
		}
		mustEnqueue(t, q, scope, lane, "replace-immediate", []byte("new immediate"))
	}
	r.advance(80 * time.Millisecond)
	for _, lane := range []Lane{Realtime, History} {
		if err := q.Recover(ctx, lane, 10); err != nil {
			t.Fatal(err)
		}
		c := mustClaim(t, q, lane)
		if c.ID != "replace-immediate" || string(c.Payload) != "new immediate" {
			t.Fatal(c)
		}
		mustAck(t, q, c)
		mustEmpty(t, q, lane)
		exists, err := q.HasTask(ctx, scope, lane, "replace-delayed")
		if err != nil || !exists {
			t.Fatal(exists, err)
		}
	}
	r.advance(500 * time.Millisecond)
	for _, lane := range []Lane{Realtime, History} {
		if err := q.Recover(ctx, lane, 10); err != nil {
			t.Fatal(err)
		}
		c := mustClaim(t, q, lane)
		if c.ID != "replace-delayed" || string(c.Payload) != "new delayed" {
			t.Fatal(c)
		}
		mustAck(t, q, c)
		mustEmpty(t, q, lane)
	}
}

func TestHistoryPriorityPreservesTaskAndLease(t *testing.T) {
	q := setupQueue(t).q
	ctx := context.Background()
	scope := Scope{1, 1}
	mustEnqueue(t, q, scope, History, "active", nil)
	active := mustClaim(t, q, History)
	mustEnqueue(t, q, scope, History, "ordinary", nil)
	mustEnqueue(t, q, scope, History, "promote", []byte("original"))
	if err := q.EnqueueAfter(ctx, scope, History, "delayed", nil, time.Hour); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"active", "delayed", "missing", "promote", "promote"} {
		if err := q.EnqueueHistoryPriority(ctx, scope, id, []byte("priority")); err != nil {
			t.Fatal(err)
		}
	}
	mustEmpty(t, q, History)
	mustAck(t, q, active)
	for _, id := range []string{"promote", "missing", "ordinary"} {
		got := mustClaim(t, q, History)
		if got.ID != id || (id == "promote" && string(got.Payload) != "original") {
			t.Fatalf("want %s, got %+v", id, got)
		}
		mustAck(t, q, got)
	}
	mustEmpty(t, q, History)
}

func TestReserveHistorySlotIsScopedAndExpires(t *testing.T) {
	fixture := setupQueue(t)
	ctx := context.Background()
	scope := Scope{TenantID: 1, SourceID: 1}
	reserve := func(scope Scope, want bool) {
		t.Helper()
		got, err := fixture.q.ReserveHistorySlot(ctx, scope, time.Second)
		if err != nil || got != want {
			t.Fatalf("scope=%+v got=%v want=%v err=%v", scope, got, want, err)
		}
	}
	reserve(scope, true)
	reserve(scope, false)
	reserve(Scope{TenantID: 1, SourceID: 2}, true)
	reserve(Scope{TenantID: 2, SourceID: 1}, true)
	fixture.advance(2 * time.Second)
	reserve(scope, true)
}
