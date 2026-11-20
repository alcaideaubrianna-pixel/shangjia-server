package sourcequeue

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func setupFreshQueue(t *testing.T) testQueue {
	t.Helper()
	r := setupQueue(t)
	t.Cleanup(func() {
		// setupQueue predates Fresh. Clean only this test's known Fresh scopes,
		// including its indexes, when exercising an explicit loopback Redis.
		for _, scope := range []Scope{{1, 1}, {1, 2}} {
			keys := r.q.keys(scope, Fresh)
			keys = append(keys, keys[1]+":recover-turn", keys[8]+":budget")
			args := make([]interface{}, len(keys))
			for i, k := range keys {
				args[i] = k
			}
			_, _ = r.q.client.Do(context.Background(), "DEL", args...)
		}
	})
	return r
}

func TestFreshLaneBypassesRealtimeBacklogAndPreservesFIFO(t *testing.T) {
	r := setupFreshQueue(t)
	q := r.q
	scope := Scope{1, 1}
	for i := 0; i < 64; i++ {
		mustEnqueue(t, q, scope, Realtime, fmt.Sprintf("old-%02d", i), []byte("old"))
	}
	realtime := mustClaim(t, q, Realtime)
	if realtime.ID != "old-00" {
		t.Fatal(realtime)
	}
	for _, id := range []string{"fresh-a", "fresh-b", "fresh-c"} {
		mustEnqueue(t, q, scope, Fresh, id, []byte(id))
	}
	for _, id := range []string{"fresh-a", "fresh-b", "fresh-c"} {
		c := mustClaim(t, q, Fresh)
		if c.ID != id || c.Scope != scope || c.Lane != Fresh {
			t.Fatalf("fresh FIFO lost: %+v want=%s", c, id)
		}
		mustEmpty(t, q, Realtime) // Its old inflight owner remains intact.
		mustAck(t, q, c)
	}
	mustEmpty(t, q, Fresh)
	mustAck(t, q, realtime)
	next := mustClaim(t, q, Realtime)
	if next.ID != "old-01" {
		t.Fatalf("fresh consumption changed realtime FIFO: %+v", next)
	}
	mustAck(t, q, next)
}

func TestFreshLaneGenerationAndDuplicateAck(t *testing.T) {
	r := setupFreshQueue(t)
	q := r.q
	scope := Scope{1, 1}
	ctx := context.Background()
	mustEnqueue(t, q, scope, Fresh, "same", []byte("first"))
	mustEnqueue(t, q, scope, Fresh, "same", []byte("latest-before-claim"))
	old := mustClaim(t, q, Fresh)
	if string(old.Payload) != "latest-before-claim" {
		t.Fatal(old)
	}
	mustEnqueue(t, q, scope, Fresh, "same", []byte("arrived-during-claim"))
	mustAck(t, q, old)
	current := mustClaim(t, q, Fresh)
	if current.ID != "same" || string(current.Payload) != "arrived-during-claim" || current.Token == old.Token {
		t.Fatalf("dirty generation lost: %+v", current)
	}
	for _, err := range []error{q.Ack(ctx, old), q.Nack(ctx, old, 0), q.Renew(ctx, old, time.Second)} {
		if !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("stale token accepted: %v", err)
		}
	}
	mustAck(t, q, current)
	if err := q.Ack(ctx, current); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("duplicate ACK accepted: %v", err)
	}
	mustEmpty(t, q, Fresh)
}

func TestFreshLaneLeaseRecoveryDoesNotAffectOtherLanes(t *testing.T) {
	r := setupFreshQueue(t)
	q := r.q
	scope := Scope{1, 1}
	ctx := context.Background()
	owners := map[Lane]*Claim{}
	for _, lane := range []Lane{Realtime, History, Fresh} {
		mustEnqueue(t, q, scope, lane, "same-id", []byte(lane))
		lease := 5 * time.Second
		if lane == Fresh {
			lease = 100 * time.Millisecond
		}
		c, err := q.Claim(ctx, lane, lease)
		if err != nil || c == nil || string(c.Payload) != string(lane) {
			t.Fatalf("lane isolation failed: %+v %v", c, err)
		}
		owners[lane] = c
	}
	// Each scope/lane has separate task state, generation, lease and ready indexes.
	for _, a := range []Lane{Realtime, History, Fresh} {
		for _, b := range []Lane{Realtime, History, Fresh} {
			if a == b {
				continue
			}
			ka, kb := q.keys(scope, a), q.keys(scope, b)
			for i := range ka {
				if ka[i] == kb[i] {
					t.Fatalf("key collision between %s and %s: %s", a, b, ka[i])
				}
			}
		}
	}
	r.advance(150 * time.Millisecond)
	old := owners[Fresh]
	if err := q.Ack(ctx, old); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("expired Fresh ACK accepted: %v", err)
	}
	if err := q.Recover(ctx, Fresh, 1); err != nil {
		t.Fatal(err)
	}
	current := mustClaim(t, q, Fresh)
	if current.ID != old.ID || current.Token == old.Token {
		t.Fatalf("Fresh recovery failed: %+v", current)
	}
	if err := q.Ack(ctx, old); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("recovered owner accepted old ACK: %v", err)
	}
	for _, lane := range []Lane{Realtime, History} {
		if err := q.Renew(ctx, owners[lane], 5*time.Second); err != nil {
			t.Fatalf("Fresh recovery touched %s owner: %v", lane, err)
		}
		mustAck(t, q, owners[lane])
	}
	mustAck(t, q, current)
	for _, lane := range []Lane{Realtime, History, Fresh} {
		mustEmpty(t, q, lane)
	}
}

func TestCancelScopeIncludesFreshAndFencesAllOwners(t *testing.T) {
	r := setupFreshQueue(t)
	q := r.q
	ctx := context.Background()
	scope, other := Scope{1, 1}, Scope{1, 2}
	lanes := []Lane{Realtime, History, Fresh}
	owners := make(map[Lane]*Claim)
	for _, lane := range lanes {
		mustEnqueue(t, q, scope, lane, "owned", []byte("old"))
		owners[lane] = mustClaim(t, q, lane)
		mustEnqueue(t, q, scope, lane, "pending", nil)
		if err := q.EnqueueAfter(ctx, scope, lane, "delayed", nil, time.Second); err != nil {
			t.Fatal(err)
		}
		mustEnqueue(t, q, other, lane, "other-source", []byte(lane))
	}
	if err := q.CancelScope(ctx, scope); err != nil {
		t.Fatal(err)
	}
	for _, lane := range lanes {
		for _, err := range []error{q.Ack(ctx, owners[lane]), q.Nack(ctx, owners[lane], 0), q.Renew(ctx, owners[lane], time.Second)} {
			if !errors.Is(err, ErrLeaseLost) {
				t.Fatalf("%s canceled owner still valid: %v", lane, err)
			}
		}
		c := mustClaim(t, q, lane)
		if c.Scope != other || c.ID != "other-source" {
			t.Fatalf("cancel touched other source or leaked task: %+v", c)
		}
		mustAck(t, q, c)
	}
	r.advance(2 * time.Second)
	for _, lane := range lanes {
		if err := q.Recover(ctx, lane, 10); err != nil {
			t.Fatal(err)
		}
		mustEmpty(t, q, lane)
	}
}
