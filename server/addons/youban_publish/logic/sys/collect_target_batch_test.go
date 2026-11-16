package sys

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCollectTargetBatchCoalescesAndRetainsDirty(t *testing.T) {
	now := time.Now()
	generation := "one"
	done := collectTargetBatchCompletion{}
	queries := 0
	ops := collectTargetBatchOps{
		targetID:   1,
		coverage:   func() []int64 { return []int64{1} },
		generation: func(context.Context) (string, error) { return generation, nil },
		completed:  func(context.Context) (collectTargetBatchCompletion, error) { return done, nil },
		owned:      func(context.Context) (bool, error) { return true, nil },
		run:        func(context.Context) error { queries++; return nil },
		finish:     func(_ context.Context, value collectTargetBatchCompletion) error { done = value; return nil },
	}
	for i := 0; i < 20; i++ {
		_ = runCollectTargetBatch(context.Background(), ops, now)
	}
	if queries != 1 {
		t.Fatalf("20 targets queried %d windows", queries)
	}
	generation = "two"
	_ = runCollectTargetBatch(context.Background(), ops, now)
	if queries != 2 || done.Generation != "two" {
		t.Fatal("new generation lost wakeup")
	}
}

func TestCollectTargetBatchRejectsDirtyOwnerAndRedisFailure(t *testing.T) {
	for _, scenario := range []string{"dirty", "owner", "redis", "cancel", "database"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			generation := "one"
			owned := true
			runs, writes := 0, 0
			ops := collectTargetBatchOps{
				generation: func(context.Context) (string, error) {
					if scenario == "redis" {
						return "", errors.New("Redis unavailable")
					}
					return generation, nil
				},
				completed: func(context.Context) (collectTargetBatchCompletion, error) {
					return collectTargetBatchCompletion{}, nil
				},
				owned: func(context.Context) (bool, error) { return owned, nil },
				run: func(context.Context) error {
					runs++
					switch scenario {
					case "dirty":
						generation = "two"
					case "owner":
						owned = false
					case "cancel":
						cancel()
					case "database":
						return errors.New("DB failure")
					}
					return nil
				},
				finish: func(context.Context, collectTargetBatchCompletion) error { writes++; return nil },
			}
			if err := runCollectTargetBatch(ctx, ops, time.Now()); err == nil {
				t.Fatal("durable task was acknowledged")
			}
			if writes != 0 {
				t.Fatal("unsafe completion")
			}
			if scenario == "redis" && runs != 0 {
				t.Fatal("Redis failure reached DB")
			}
		})
	}
}

func TestCollectTargetBatchScopeIsolation(t *testing.T) {
	base := collectProcessQueuePayload{TenantId: 1, AccountId: 0, SourceId: 3, SourceChatId: "-10042"}
	seen := map[string]bool{collectTargetBatchKey(base): true}
	variants := []collectProcessQueuePayload{base, base, base, base}
	variants[0].TenantId++
	variants[1].AccountId++
	variants[2].SourceId++
	variants[3].SourceChatId = "-10043"
	for _, p := range variants {
		key := collectTargetBatchKey(p)
		if seen[key] {
			t.Fatal("batch crossed scope")
		}
		seen[key] = true
	}
}

// The same chat generation may contain more candidates than one SQL batch.
// Membership, not a chat-wide done bit, controls which durable task can yield.
func TestCollectTargetBatchOverflowDoesNotBlockUncoveredTarget(t *testing.T) {
	now := time.Now()
	covered := make([]int64, 100)
	for i := range covered {
		covered[i] = int64(i + 1)
	}
	done := collectTargetBatchCompletion{Generation: "same", CoveredEventIDs: covered, FinishedAt: now, RetryAt: now.Add(time.Minute)}
	runs := 0
	ops := collectTargetBatchOps{
		targetID: 101, coverage: func() []int64 { return []int64{101} },
		generation: func(context.Context) (string, error) { return "same", nil },
		completed:  func(context.Context) (collectTargetBatchCompletion, error) { return done, nil },
		owned:      func(context.Context) (bool, error) { return true, nil },
		run:        func(context.Context) error { runs++; return nil },
		finish:     func(_ context.Context, v collectTargetBatchCompletion) error { done = v; return nil },
	}
	_ = runCollectTargetBatch(context.Background(), ops, now)
	if runs != 1 || !collectTargetBatchCovers(done, 101) {
		t.Fatal("overflow target was suppressed")
	}
	_ = runCollectTargetBatch(context.Background(), ops, now)
	if runs != 1 {
		t.Fatal("covered overflow target repeated the window")
	}
}

func TestCollectTargetBatchHistoryAndSeparateContextAdvance(t *testing.T) {
	now := time.Now()
	// Old waiting/history batches do not cover the realtime target, even when
	// all messages share one source/chat generation.
	done := collectTargetBatchCompletion{Generation: "same", CoveredEventIDs: []int64{1, 2, 3}, FinishedAt: now, RetryAt: now.Add(time.Hour)}
	targets := []int64{1000, 2000}
	runs := 0
	for _, target := range targets {
		ops := collectTargetBatchOps{
			targetID: target, coverage: func() []int64 { return []int64{target} },
			generation: func(context.Context) (string, error) { return "same", nil },
			completed:  func(context.Context) (collectTargetBatchCompletion, error) { return done, nil },
			owned:      func(context.Context) (bool, error) { return true, nil },
			run:        func(context.Context) error { runs++; return nil },
			finish:     func(_ context.Context, v collectTargetBatchCompletion) error { done = v; return nil },
		}
		_ = runCollectTargetBatch(context.Background(), ops, now)
	}
	if runs != 2 {
		t.Fatal("history or another time context suppressed realtime")
	}
}

func TestCollectTargetBatchChatAliases(t *testing.T) {
	p := collectProcessQueuePayload{TenantId: 1, AccountId: 2, SourceId: 3, SourceChatId: "-10042"}
	key := collectTargetBatchKey(p)
	p.SourceChatId = "42"
	if collectTargetBatchKey(p) != key {
		t.Fatal("chat aliases use different generations/leases")
	}
}

func TestCollectTargetBatchEventsContinuesWaitingAndUsesEarliestRetry(t *testing.T) {
	ctx := context.WithValue(context.Background(), collectTargetBatchVisitKey{}, &collectTargetBatchVisit{})
	visited := []int{}
	err := runCollectTargetBatchEvents(ctx, 4, func(index int) error {
		visited = append(visited, index)
		visitCollectTargetBatch(ctx, int64(index+1))
		switch index {
		case 0:
			return newCollectProcessRetryError(time.Minute, "waiting_verify")
		case 2:
			return newCollectProcessRetryError(5*time.Second, "media pending")
		}
		return nil
	})
	var retry *collectProcessRetryError
	if !errors.As(err, &retry) || retry.delay != 5*time.Second {
		t.Fatalf("earliest retry lost: %v", err)
	}
	if len(visited) != 4 {
		t.Fatalf("waiting first event starved peers: %v", visited)
	}
	if got := ctx.Value(collectTargetBatchVisitKey{}).(*collectTargetBatchVisit); len(got.ids) != 4 {
		t.Fatal("coverage did not record visits")
	}
}

func TestCollectTargetBatchEventsRealErrorAndCancellation(t *testing.T) {
	failure := errors.New("database unavailable")
	visits := &collectTargetBatchVisit{}
	ctx := context.WithValue(context.Background(), collectTargetBatchVisitKey{}, visits)
	err := runCollectTargetBatchEvents(ctx, 4, func(index int) error {
		visitCollectTargetBatch(ctx, int64(index+1))
		if index == 1 {
			return failure
		}
		return nil
	})
	if !errors.Is(err, failure) || len(visits.ids) != 2 {
		t.Fatal("DB failure did not stop unvisited work")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_ = runCollectTargetBatchEvents(canceled, 4, func(int) error { t.Fatal("canceled batch ran"); return nil })
}

func TestCollectTargetBatchLongRetryDoesNotSuppressNewStateForTwentyMinutes(t *testing.T) {
	now := time.Now()
	done := collectTargetBatchCompletion{}
	calls := 0
	ops := collectTargetBatchOps{
		targetID: 1, mergeTTL: 3 * time.Second, coverage: func() []int64 { return []int64{1} },
		generation: func(context.Context) (string, error) { return "one", nil },
		completed:  func(context.Context) (collectTargetBatchCompletion, error) { return done, nil },
		owned:      func(context.Context) (bool, error) { return true, nil },
		run: func(context.Context) error {
			calls++
			return newCollectProcessRetryError(20*time.Minute, "waiting_verify")
		},
		finish: func(_ context.Context, value collectTargetBatchCompletion) error { done = value; return nil },
	}
	err := runCollectTargetBatch(context.Background(), ops, now)
	var retry *collectProcessRetryError
	if !errors.As(err, &retry) || retry.delay != 20*time.Minute {
		t.Fatal("event retry semantics changed")
	}
	if done.RetryAt.Sub(done.FinishedAt) > 3*time.Second {
		t.Fatal("long event retry leaked into chat suppression")
	}
	_ = runCollectTargetBatch(context.Background(), ops, done.FinishedAt.Add(time.Second))
	if calls != 1 {
		t.Fatal("covered targets did not merge")
	}
	_ = runCollectTargetBatch(context.Background(), ops, done.FinishedAt.Add(4*time.Second))
	if calls != 2 {
		t.Fatal("new state blocked past merge TTL")
	}
}
