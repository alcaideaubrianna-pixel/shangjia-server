package accountschedule

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gredis"
)

// The clock is Redis TIME, so control server time rather than sleeping or
// depending on the caller's wall clock. Each test owns a loopback-only server.
func faultQueue(t *testing.T) (*Queue, *miniredis.Miniredis, time.Time) {
	t.Helper()
	r := miniredis.RunT(t)
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	r.SetTime(now)
	c, err := gredis.New(&gredis.Config{Address: r.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	return New(c, "test:{independent_account_fault}"), r, now
}

func TestFaultAccountReadyMarksDoNotPostponePeer(t *testing.T) {
	q, r, now := faultQueue(t)
	a, b := Scope{78, 615}, Scope{78, 616}
	mustMark(t, q, a, Ensure)
	before, err := r.ZScore(q.keys[0], a.Member())
	if err != nil {
		t.Fatal(err)
	}
	r.SetTime(now.Add(time.Second))
	mustMark(t, q, b, Ensure)
	for _, wake := range []Wake{Ensure, Changed, Ensure, Changed} {
		r.SetTime(now.Add(2 * time.Second))
		mustMark(t, q, a, wake)
	}
	after, err := r.ZScore(q.keys[0], a.Member())
	if err != nil || before != after {
		t.Fatalf("ready account moved: before=%v after=%v error=%v", before, after, err)
	}
	first := mustClaim(t, q, time.Minute)
	if first.Scope != a {
		t.Fatalf("older account lost place to peer: %+v", first)
	}
	mustMark(t, q, a, Changed)
	mustAck(t, q, first, Success)
	r.SetTime(now.Add(3 * time.Second))
	second := mustClaim(t, q, time.Minute)
	if second.Scope != b {
		t.Fatalf("hot account starved waiting peer: %+v", second)
	}
}

func TestFaultAccountExpiredAckRejectedBeforeReclaim(t *testing.T) {
	q, r, now := faultQueue(t)
	s := Scope{78, 615}
	mustMark(t, q, s, Ensure)
	old := mustClaim(t, q, time.Second)
	r.SetTime(now.Add(time.Second))
	accepted, err := q.Ack(context.Background(), *old, Success)
	if err != nil || accepted {
		t.Fatalf("expired owner acknowledged without successor: accepted=%v err=%v", accepted, err)
	}
	next := mustClaim(t, q, time.Minute)
	if next.Scope != s || next.Token == old.Token || next.Turn <= old.Turn {
		t.Fatalf("expired claim did not recover safely: old=%+v next=%+v", old, next)
	}
	accepted, err = q.Ack(context.Background(), *old, Failure)
	if err != nil || accepted {
		t.Fatalf("late failure replaced successor: accepted=%v err=%v", accepted, err)
	}
	mustAck(t, q, next, Success)
}

func TestFaultAccountBusyDeferredPreservesPendingAndDeadline(t *testing.T) {
	q, r, now := faultQueue(t)
	s := Scope{78, 615}
	mustMark(t, q, s, Ensure)
	first := mustClaim(t, q, time.Minute)
	// Busy is a scheduling deferral, not a business failure or completed turn.
	mustAck(t, q, first, Deferred)
	deadline, err := r.ZScore(q.keys[0], s.Member())
	if err != nil {
		t.Fatal("busy lease removed pending account", err)
	}
	r.SetTime(now.Add(500 * time.Millisecond))
	for i := 0; i < 25; i++ {
		mustMark(t, q, s, Ensure)
	}
	after, err := r.ZScore(q.keys[0], s.Member())
	if err != nil || after != deadline {
		t.Fatalf("discovery postponed busy retry: before=%v after=%v err=%v", deadline, after, err)
	}
	if claim, err := q.Claim(context.Background(), time.Minute); err != nil || claim != nil {
		t.Fatalf("busy retry ran before deadline: claim=%+v err=%v", claim, err)
	}
	r.SetTime(now.Add(2 * time.Second))
	next := mustClaim(t, q, time.Minute)
	if next.Scope != s {
		t.Fatal("busy account not recovered", next)
	}
	// Repeated lease contention must retain the fixed short delay rather than
	// increase the business-failure backoff exponentially.
	mustAck(t, q, next, Deferred)
	retry, err := r.ZScore(q.keys[0], s.Member())
	if err != nil || retry != float64(now.Add(3*time.Second).UnixMilli()) {
		t.Fatalf("lease contention escalated retry delay: score=%v err=%v", retry, err)
	}
	r.SetTime(now.Add(4 * time.Second))
	last := mustClaim(t, q, time.Minute)
	mustAck(t, q, last, Success)
}

func TestFaultAccountChangedDuringFailedExecutionSurvives(t *testing.T) {
	q, r, now := faultQueue(t)
	s := Scope{78, 615}
	mustMark(t, q, s, Ensure)
	first := mustClaim(t, q, time.Minute)
	for i := 0; i < 3; i++ {
		mustMark(t, q, s, Changed)
	}
	mustAck(t, q, first, Failure)
	r.SetTime(now.Add(2 * time.Second))
	next := mustClaim(t, q, time.Minute)
	if next.Generation != first.Generation+3 {
		t.Fatalf("failed execution lost new wakes: first=%+v next=%+v", first, next)
	}
	mustAck(t, q, next, Success)
	if claim, err := q.Claim(context.Background(), time.Minute); err != nil || claim != nil {
		t.Fatalf("completed generation still pending: claim=%+v err=%v", claim, err)
	}
}

func TestFaultAccountFutureScheduleKeepsEarliestWake(t *testing.T) {
	q, r, now := faultQueue(t)
	ctx := context.Background()
	s := Scope{78, 615}
	for _, delay := range []time.Duration{20 * time.Second, 5 * time.Second, time.Minute} {
		if err := q.Schedule(ctx, s, now.Add(delay)); err != nil {
			t.Fatal(err)
		}
	}
	r.SetTime(now.Add(4 * time.Second))
	if claim, err := q.Claim(ctx, time.Minute); err != nil || claim != nil {
		t.Fatalf("future plan ran early: claim=%+v err=%v", claim, err)
	}
	r.SetTime(now.Add(5 * time.Second))
	first := mustClaim(t, q, time.Minute)
	if first.Scope != s {
		t.Fatal("earliest wake lost", first)
	}
	// A plan becoming due while running must survive the first owner's ACK.
	if err := q.Schedule(ctx, s, now.Add(6*time.Second)); err != nil {
		t.Fatal(err)
	}
	r.SetTime(now.Add(6 * time.Second))
	if claim, err := q.Claim(ctx, time.Minute); err != nil || claim != nil {
		t.Fatalf("scheduled running account claimed twice: claim=%+v err=%v", claim, err)
	}
	mustAck(t, q, first, Success)
	r.SetTime(now.Add(7 * time.Second))
	second := mustClaim(t, q, time.Minute)
	if second.Generation <= first.Generation {
		t.Fatal("due wake while running lost", second)
	}
	mustAck(t, q, second, Success)
}

func TestFaultAccountMoreYieldsToPeerAndAdvancesTurn(t *testing.T) {
	q, r, now := faultQueue(t)
	a, b := Scope{78, 615}, Scope{78, 616}
	mustMark(t, q, a, Ensure)
	first := mustClaim(t, q, time.Minute)
	mustMark(t, q, b, Ensure)
	mustAck(t, q, first, More)
	peer := mustClaim(t, q, time.Minute)
	if peer.Scope != b {
		t.Fatal("More account starved ready peer", peer)
	}
	mustAck(t, q, peer, Success)
	r.SetTime(now.Add(time.Second))
	next := mustClaim(t, q, time.Minute)
	if next.Scope != a || next.Turn <= first.Turn {
		t.Fatalf("More did not continue stage cursor: first=%+v next=%+v", first, next)
	}
}

func TestFaultAccountConcurrentScheduleAndAckPreserveWake(t *testing.T) {
	q, r, now := faultQueue(t)
	ctx := context.Background()
	for i := int64(1); i <= 20; i++ {
		s := Scope{78, i}
		mustMark(t, q, s, Ensure)
		claim := mustClaim(t, q, time.Minute)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := q.Schedule(ctx, s, now.Add(time.Second)); err != nil {
				t.Error(err)
			}
		}()
		go func() {
			defer wg.Done()
			if ok, err := q.Ack(ctx, *claim, Success); err != nil || !ok {
				t.Errorf("ACK failed: %v %v", ok, err)
			}
		}()
		wg.Wait()
	}
	r.SetTime(now.Add(2 * time.Second))
	seen := make(map[Scope]bool)
	for i := 0; i < 20; i++ {
		claim := mustClaim(t, q, time.Minute)
		if seen[claim.Scope] {
			t.Fatal("same account claimed twice", claim.Scope)
		}
		seen[claim.Scope] = true
	}
	if len(seen) != 20 {
		t.Fatal("concurrent Schedule/ACK lost account wake", len(seen))
	}
}
