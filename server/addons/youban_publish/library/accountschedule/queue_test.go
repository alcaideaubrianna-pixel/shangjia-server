package accountschedule

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/util/guid"
)

func testQueue(t *testing.T) *Queue {
	t.Helper()
	addr := os.Getenv("YOUBAN_ACCOUNT_SCHEDULE_TEST_REDIS")
	if addr == "" {
		addr = miniredis.RunT(t).Addr()
	} else {
		host, _, err := net.SplitHostPort(addr)
		if err != nil || host != "127.0.0.1" {
			t.Fatal("tests require explicit loopback Redis")
		}
	}
	c, err := gredis.New(&gredis.Config{Address: addr, Db: 15})
	if err != nil {
		t.Fatal(err)
	}
	q := New(c, "test:{account_schedule_"+guid.S()+"}")
	t.Cleanup(func() {
		_, _ = c.GroupScript().Eval(context.Background(), `return redis.call('DEL',unpack(KEYS))`, int64(len(q.keys)), q.keys, nil)
		_ = c.Close(context.Background())
	})
	return q
}
func mustMark(t *testing.T, q *Queue, s Scope, w Wake) {
	t.Helper()
	if err := q.Mark(context.Background(), s, w); err != nil {
		t.Fatal(err)
	}
}
func mustClaim(t *testing.T, q *Queue, ttl time.Duration) *Claim {
	t.Helper()
	c, e := q.Claim(context.Background(), ttl)
	if e != nil {
		t.Fatal(e)
	}
	if c == nil {
		t.Fatal("missing claim")
	}
	return c
}
func mustAck(t *testing.T, q *Queue, c *Claim, o Outcome) {
	t.Helper()
	ok, e := q.Ack(context.Background(), *c, o)
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
}
func TestAccountScheduleCoalescesAndPreservesWake(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	s := Scope{78, 615}
	for i := 0; i < 100; i++ {
		mustMark(t, q, s, Ensure)
	}
	first := mustClaim(t, q, time.Second)
	for i := 0; i < 100; i++ {
		mustMark(t, q, s, Ensure)
	}
	mustMark(t, q, s, Changed)
	mustAck(t, q, first, Success)
	time.Sleep(3 * time.Millisecond)
	second := mustClaim(t, q, time.Second)
	if second.Generation <= first.Generation {
		t.Fatal("lost wake")
	}
	ok, e := q.Ack(ctx, *first, Success)
	if e != nil || ok {
		t.Fatal("stale ack accepted", ok, e)
	}
	mustAck(t, q, second, Success)
	c, e := q.Claim(ctx, time.Second)
	if c != nil || e != nil {
		t.Fatal("periodic ensure kept account spinning", c, e)
	}
	mustMark(t, q, s, Ensure)
	third := mustClaim(t, q, time.Second)
	if third.Turn <= second.Turn {
		t.Fatal("stage fairness cursor reset")
	}
	mustAck(t, q, third, Success)
}
func TestAccountScheduleCrashRecoveryAndTokenFencing(t *testing.T) {
	q := testQueue(t)
	s := Scope{1, 1}
	mustMark(t, q, s, Ensure)
	first := mustClaim(t, q, 15*time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	second := mustClaim(t, q, time.Second)
	if first.Token == second.Token {
		t.Fatal("token reused")
	}
	ok, e := q.Ack(context.Background(), *first, Success)
	if e != nil || ok {
		t.Fatal("expired owner acknowledged", ok, e)
	}
	mustAck(t, q, second, Success)
}
func TestAccountScheduleFailureDoesNotBlockOtherAccounts(t *testing.T) {
	q := testQueue(t)
	mustMark(t, q, Scope{1, 1}, Ensure)
	a := mustClaim(t, q, time.Second)
	mustAck(t, q, a, Failure)
	mustMark(t, q, Scope{1, 2}, Ensure)
	b := mustClaim(t, q, time.Second)
	if b.Scope.AccountID != 2 {
		t.Fatal("failed account blocked peer")
	}
	mustAck(t, q, b, Success)
	c, e := q.Claim(context.Background(), time.Second)
	if e != nil || c != nil {
		t.Fatal("backoff bypassed", c, e)
	}
	mustMark(t, q, Scope{1, 1}, Changed)
	c, e = q.Claim(context.Background(), time.Second)
	if e != nil || c != nil {
		t.Fatal("mark bypassed backoff", c, e)
	}
}
func TestAccountScheduleConcurrentMarksClaimsAndAcks(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for n := 0; n < 100; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if e := q.Mark(ctx, Scope{1, int64(n%10 + 1)}, Ensure); e != nil {
				t.Error(e)
			}
		}(n)
	}
	wg.Wait()
	seen := sync.Map{}
	count := 0
	var mu sync.Mutex
	for n := 0; n < 32; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, e := q.Claim(ctx, time.Second)
			if e != nil {
				t.Error(e)
				return
			}
			if c == nil {
				return
			}
			if _, exists := seen.LoadOrStore(c.Scope.Member(), true); exists {
				t.Error("concurrent duplicate account")
			}
			mu.Lock()
			count++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if count != 10 {
		t.Fatal(fmt.Sprintf("claimed %d accounts, want 10", count))
	}
}
func TestAccountScheduleWakeAckRace(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	for i := int64(1); i <= 50; i++ {
		s := Scope{1, i}
		mustMark(t, q, s, Ensure)
		c := mustClaim(t, q, time.Second)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if e := q.Mark(ctx, s, Changed); e != nil {
				t.Error(e)
			}
		}()
		go func() {
			defer wg.Done()
			ok, e := q.Ack(ctx, *c, Success)
			if e != nil || !ok {
				t.Error(ok, e)
			}
		}()
		wg.Wait()
		time.Sleep(2 * time.Millisecond)
		next := mustClaim(t, q, time.Second)
		if next.Scope != s {
			t.Fatal("wake lost")
		}
		mustAck(t, q, next, Success)
	}
}

func TestAccountScheduleDeferredAndFairness(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	mustMark(t, q, Scope{1, 1}, Ensure)
	a := mustClaim(t, q, time.Second)
	mustMark(t, q, Scope{1, 2}, Ensure)
	mustMark(t, q, a.Scope, Changed)
	mustAck(t, q, a, Success)
	time.Sleep(3 * time.Millisecond)
	b := mustClaim(t, q, time.Second)
	if b.Scope.AccountID != 2 {
		t.Fatal("hot account overtook waiting peer")
	}
	mustAck(t, q, b, Success)
	next := mustClaim(t, q, time.Second)
	mustAck(t, q, next, Deferred)
	empty, err := q.Claim(ctx, time.Second)
	if err != nil || empty != nil {
		t.Fatal("busy account dropped or bypassed delay", empty, err)
	}
}

func TestAccountScheduleMoreAndTimedWake(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	s := Scope{1, 1}
	mustMark(t, q, s, Ensure)
	first := mustClaim(t, q, time.Second)
	mustAck(t, q, first, More)
	time.Sleep(20 * time.Millisecond)
	second := mustClaim(t, q, time.Second)
	mustAck(t, q, second, Success)
	raw, err := q.client.GroupScript().Eval(ctx, `local score=redis.call('ZSCORE',KEYS[1],ARGV[1]); return score or ''`, 1, q.keys[:1], []interface{}{s.Member()})
	if err != nil || raw.String() == "" {
		t.Fatal("successful account lost timed wake", err)
	}
	// Fast-forward only this test member's scheduled score, without touching any other namespace.
	_, err = q.client.GroupScript().Eval(ctx, `return redis.call('ZADD',KEYS[1],0,ARGV[1])`, 1, q.keys[:1], []interface{}{s.Member()})
	if err != nil {
		t.Fatal(err)
	}
	third := mustClaim(t, q, time.Second)
	if third.Turn <= second.Turn {
		t.Fatal("timed wake lost fairness cursor")
	}
	mustAck(t, q, third, Success)
}

func TestAccountScheduleEarliestPlanWake(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	s := Scope{1, 1}
	if err := q.Schedule(ctx, s, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := q.Schedule(ctx, s, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	claim := mustClaim(t, q, time.Second)
	mustAck(t, q, claim, Success)
	if err := q.Schedule(ctx, s, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	next := mustClaim(t, q, time.Second)
	if next.Generation <= claim.Generation {
		t.Fatal("timed plan wake lost")
	}
	mustAck(t, q, next, Success)
}

func TestAccountScheduleMarkAdvancesIdleTimer(t *testing.T) {
	q := testQueue(t)
	ctx := context.Background()
	s := Scope{1, 1}
	mustMark(t, q, s, Ensure)
	first := mustClaim(t, q, time.Second)
	mustAck(t, q, first, Success)
	before, e := q.Claim(ctx, time.Second)
	if e != nil || before != nil {
		t.Fatal("idle timer fired immediately", before, e)
	}
	mustMark(t, q, s, Changed)
	next := mustClaim(t, q, time.Second)
	if next.Generation <= first.Generation {
		t.Fatal("mark failed to advance idle wake")
	}
	mustAck(t, q, next, Success)
}
