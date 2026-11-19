package accountschedule

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/util/guid"
)

func TestConfiguredIdlePreservesChangedWake(t *testing.T) {
	q := testQueue(t)
	q = NewWithIdleInterval(q.client, strings.TrimSuffix(q.keys[0], ":ready"), 5*time.Minute)
	ctx := context.Background()
	scope := Scope{TenantID: 1, AccountID: 2}
	if err := q.Mark(ctx, scope, Changed); err != nil {
		t.Fatal(err)
	}
	claim, err := q.Claim(ctx, time.Minute)
	if err != nil || claim == nil {
		t.Fatalf("claim: %v %v", claim, err)
	}
	if ok, err := q.Ack(ctx, *claim, Success); err != nil || !ok {
		t.Fatalf("ack: %v %v", ok, err)
	}
	score, err := q.client.Do(ctx, "ZSCORE", q.keys[0], scope.Member())
	if err != nil {
		t.Fatal(err)
	}
	remaining := time.Until(time.UnixMilli(score.Int64()))
	if remaining < 290*time.Second || remaining > 310*time.Second {
		t.Fatalf("idle interval: %v", remaining)
	}
	if c, err := q.Claim(ctx, time.Minute); err != nil || c != nil {
		t.Fatalf("premature claim: %v %v", c, err)
	}
	if err := q.Schedule(ctx, scope, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	c, err := q.Claim(ctx, time.Minute)
	if err != nil || c == nil {
		t.Fatalf("due wake delayed: %v %v", c, err)
	}
	if ok, err := q.Ack(ctx, *c, Success); err != nil || !ok {
		t.Fatalf("ack due: %v %v", ok, err)
	}
	if err := q.Mark(ctx, scope, Changed); err != nil {
		t.Fatal(err)
	}
	if c, err := q.Claim(ctx, time.Minute); err != nil || c == nil {
		t.Fatalf("changed wake delayed: %v %v", c, err)
	}
}

func TestStalledBackoffAndImmediateWake(t *testing.T) {
	q, r, now := faultQueue(t)
	scope := Scope{1, 1}
	mustMark(t, q, scope, Ensure)
	for i := 0; i < 12; i++ {
		c := mustClaim(t, q, time.Minute)
		mustAck(t, q, c, Stalled)
		score, err := r.ZScore(q.keys[0], scope.Member())
		delay := time.Duration(1<<min(i, 9)) * time.Second
		if delay > 5*time.Minute {
			delay = 5 * time.Minute
		}
		if err != nil || score != float64(now.Add(delay).UnixMilli()) {
			t.Fatalf("round %d score=%v delay=%v err=%v", i, score, delay, err)
		}
		now = now.Add(delay)
		r.SetTime(now)
	}
	c := mustClaim(t, q, time.Minute)
	mustAck(t, q, c, Stalled)
	mustMark(t, q, scope, Changed)
	c = mustClaim(t, q, time.Minute)
	mustAck(t, q, c, Stalled)
	if err := q.Schedule(context.Background(), scope, now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	c = mustClaim(t, q, time.Minute)
	mustAck(t, q, c, Success)
}

func TestRetireFencesWakeAndOwner(t *testing.T) {
	for _, mode := range []string{"clean", "changed", "plan", "expired"} {
		t.Run(mode, func(t *testing.T) {
			q, r, now := faultQueue(t)
			ctx := context.Background()
			scope := Scope{1, 2}
			mustMark(t, q, scope, Ensure)
			c := mustClaim(t, q, time.Minute)
			switch mode {
			case "changed":
				mustMark(t, q, scope, Changed)
			case "plan":
				if err := q.Schedule(ctx, scope, now.Add(time.Hour)); err != nil {
					t.Fatal(err)
				}
			case "expired":
				r.SetTime(now.Add(time.Minute))
			}
			ok, err := q.Ack(ctx, *c, Retired)
			if err != nil || ok == (mode == "expired") {
				t.Fatalf("ack %v %v", ok, err)
			}
			value, err := q.client.Do(ctx, "HEXISTS", q.keys[2], scope.Member())
			if err != nil || value.Bool() != (mode != "clean") {
				t.Fatalf("state retention %v %v", value, err)
			}
			if mode == "clean" {
				for _, key := range []string{q.keys[0], q.keys[1], q.keys[3]} {
					if _, err := r.ZScore(key, scope.Member()); err == nil {
						t.Fatal("retired index remains", key)
					}
				}
				mustMark(t, q, scope, Changed)
				mustClaim(t, q, time.Minute)
			}
		})
	}
}

func TestRepeatedContinuationYieldsWithoutDroppingAccount(t *testing.T) {
	q, r, now := faultQueue(t)
	scope := Scope{1, 3}
	mustMark(t, q, scope, Ensure)
	for i := 0; i < 12; i++ {
		c := mustClaim(t, q, time.Minute)
		mustAck(t, q, c, More)
		score, err := r.ZScore(q.keys[0], scope.Member())
		delay := time.Duration(score-float64(now.UnixMilli())) * time.Millisecond
		if err != nil || delay <= 0 || delay > time.Second {
			t.Fatalf("delay %v err %v", delay, err)
		}
		if i >= 7 && delay != time.Second {
			t.Fatalf("hot continuation not limited: %v", delay)
		}
		now = now.Add(delay)
		r.SetTime(now)
	}
}

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
