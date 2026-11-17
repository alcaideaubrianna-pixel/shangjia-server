package sys

import (
	"context"
	"errors"
	"testing"
	"time"
)

type collectReuseTestFixture struct {
	now          time.Time
	done         collectTargetBatchCompletion
	observations []collectBatchReuseObservation
	runs         int
	writes       int
	ops          collectTargetBatchOps
}

func newCollectReuseTestFixture() *collectReuseTestFixture {
	f := &collectReuseTestFixture{now: time.Now()}
	f.done = collectTargetBatchCompletion{
		Generation: "current", CoveredEventIDs: []int64{41},
		FinishedAt: f.now.Add(-time.Second), RetryAt: f.now.Add(time.Second),
	}
	f.ops = collectTargetBatchOps{
		targetID: 41,
		mergeTTL: 3 * time.Second,
		coverage: func() []int64 { return []int64{41} },
		generation: func(context.Context) (string, error) {
			return "current", nil
		},
		completed: func(context.Context) (collectTargetBatchCompletion, error) {
			return f.done, nil
		},
		owned: func(context.Context) (bool, error) { return true, nil },
		run:   func(context.Context) error { f.runs++; return nil },
		finish: func(_ context.Context, done collectTargetBatchCompletion) error {
			f.writes++
			f.done = done
			return nil
		},
		observeReuse: func(observation collectBatchReuseObservation) {
			f.observations = append(f.observations, observation)
		},
	}
	return f
}

func assertCollectReuseObservation(t *testing.T, f *collectReuseTestFixture, outcome collectBatchReuseOutcome, reason collectBatchReuseReason) {
	t.Helper()
	if len(f.observations) != 1 {
		t.Fatalf("observed %d decisions, want exactly one: %+v", len(f.observations), f.observations)
	}
	got := f.observations[0]
	if got.Outcome != outcome || got.Reason != reason {
		t.Fatalf("observation=%+v, want outcome=%s reason=%s", got, outcome, reason)
	}
}

func TestCollectWindowReuseHitPreservesDurableRetry(t *testing.T) {
	f := newCollectReuseTestFixture()
	for call := 0; call < 3; call++ {
		f.observations = nil
		err := runCollectTargetBatch(context.Background(), f.ops, f.now)
		var retry *collectProcessRetryError
		if !errors.As(err, &retry) || retry.delay != time.Second {
			t.Fatalf("hit must defer durable task for remaining merge time, got %v", err)
		}
		assertCollectReuseObservation(t, f, collectBatchReuseHit, collectBatchReuseValid)
		if f.runs != 0 || f.writes != 0 {
			t.Fatalf("hit executed work or rewrote completion: runs=%d writes=%d", f.runs, f.writes)
		}
		if !f.observations[0].MergeUntil.Equal(f.done.RetryAt) {
			t.Fatal("hit lost its existing retry boundary")
		}
	}
}

func TestCollectWindowReuseMissReasonsRunAndPreserveRetry(t *testing.T) {
	for _, reason := range []collectBatchReuseReason{
		collectBatchReuseNoCompletion, collectBatchReuseGenerationChange,
		collectBatchReuseExpired, collectBatchReuseRetryDue, collectBatchReuseNotCovered,
	} {
		t.Run(string(reason), func(t *testing.T) {
			f := newCollectReuseTestFixture()
			switch reason {
			case collectBatchReuseNoCompletion:
				f.done = collectTargetBatchCompletion{}
			case collectBatchReuseGenerationChange:
				f.done.Generation = "previous"
			case collectBatchReuseExpired:
				f.done.FinishedAt = f.now.Add(-4 * time.Second)
				f.done.RetryAt = f.now.Add(time.Hour)
			case collectBatchReuseRetryDue:
				f.done.RetryAt = f.now
			case collectBatchReuseNotCovered:
				f.done.CoveredEventIDs = []int64{40, 42}
			}
			originalRetry := newCollectProcessRetryError(20*time.Minute, "waiting_verify")
			f.ops.run = func(context.Context) error { f.runs++; return originalRetry }
			err := runCollectTargetBatch(context.Background(), f.ops, f.now)
			if err != originalRetry {
				t.Fatalf("run retry identity changed: got %v want %v", err, originalRetry)
			}
			assertCollectReuseObservation(t, f, collectBatchReuseMiss, reason)
			if f.runs != 1 || f.writes != 1 || !collectTargetBatchCovers(f.done, 41) {
				t.Fatalf("miss must run and complete covered target once: runs=%d writes=%d done=%+v",
					f.runs, f.writes, f.done)
			}
			if f.done.Generation != "current" || f.done.RetryAt.Sub(f.done.FinishedAt) != f.ops.mergeTTL {
				t.Fatalf("retry must retain bounded merge completion: %+v", f.done)
			}
			// The next invocation may reuse only the newly covered target.
			f.observations = nil
			err = runCollectTargetBatch(context.Background(), f.ops, f.done.FinishedAt.Add(time.Second))
			if !isCollectProcessRetryError(err) || f.runs != 1 || f.writes != 1 {
				t.Fatalf("completed target repeated work or ACKed: err=%v runs=%d writes=%d", err, f.runs, f.writes)
			}
			assertCollectReuseObservation(t, f, collectBatchReuseHit, collectBatchReuseValid)
		})
	}
}

func TestCollectWindowReuseSuccessfulRunStillDefersTerminalACK(t *testing.T) {
	f := newCollectReuseTestFixture()
	f.done = collectTargetBatchCompletion{}
	err := runCollectTargetBatch(context.Background(), f.ops, f.now)
	var retry *collectProcessRetryError
	if !errors.As(err, &retry) || retry.delay != collectProcessMinimumDelay {
		t.Fatalf("successful batch must retain terminal confirmation retry: %v", err)
	}
	assertCollectReuseObservation(t, f, collectBatchReuseMiss, collectBatchReuseNoCompletion)
	if f.runs != 1 || f.writes != 1 {
		t.Fatalf("runs=%d writes=%d, want one each", f.runs, f.writes)
	}
}

func TestCollectWindowReuseReadErrorsAreNotEvaluated(t *testing.T) {
	for _, reason := range []collectBatchReuseReason{collectBatchReuseGenerationError, collectBatchReuseCompletionError} {
		t.Run(string(reason), func(t *testing.T) {
			f := newCollectReuseTestFixture()
			readError := errors.New("cache unavailable")
			completionReads, ownershipReads := 0, 0
			f.ops.completed = func(context.Context) (collectTargetBatchCompletion, error) {
				completionReads++
				return f.done, readError
			}
			f.ops.owned = func(context.Context) (bool, error) {
				ownershipReads++
				return true, nil
			}
			if reason == collectBatchReuseGenerationError {
				f.ops.generation = func(context.Context) (string, error) { return "", readError }
			}
			err := runCollectTargetBatch(context.Background(), f.ops, f.now)
			var retry *collectProcessRetryError
			if !errors.As(err, &retry) || retry.delay != collectProcessRedisRetryDelay {
				t.Fatalf("cache read failure must retain durable retry: %v", err)
			}
			assertCollectReuseObservation(t, f, collectBatchReuseNotEvaluated, reason)
			// Only evaluated hit/miss outcomes belong to the reuse denominator.
			for _, observation := range f.observations {
				if observation.Outcome == collectBatchReuseHit || observation.Outcome == collectBatchReuseMiss {
					t.Fatal("cache read failure entered the evaluated reuse denominator")
				}
			}
			if f.runs != 0 || f.writes != 0 || ownershipReads != 0 {
				t.Fatal("unevaluated lookup reached ownership, run or finish")
			}
			wantReads := 1
			if reason == collectBatchReuseGenerationError {
				wantReads = 0
			}
			if completionReads != wantReads {
				t.Fatalf("completion reads=%d, want %d", completionReads, wantReads)
			}
		})
	}
}

func TestCollectWindowReuseLeaseLossDoesNotObserveAgain(t *testing.T) {
	for _, loseAt := range []int{1, 2} {
		name := "before_run"
		if loseAt == 2 {
			name = "after_run"
		}
		t.Run(name, func(t *testing.T) {
			f := newCollectReuseTestFixture()
			f.done = collectTargetBatchCompletion{}
			claims := 0
			f.ops.owned = func(context.Context) (bool, error) {
				claims++
				return claims != loseAt, nil
			}
			err := runCollectTargetBatch(context.Background(), f.ops, f.now)
			if !isCollectProcessRetryError(err) {
				t.Fatalf("lost lease must not ACK: %v", err)
			}
			assertCollectReuseObservation(t, f, collectBatchReuseMiss, collectBatchReuseNoCompletion)
			if f.runs != loseAt-1 || f.writes != 0 || claims != loseAt {
				t.Fatalf("unsafe lease-loss work: runs=%d writes=%d claims=%d", f.runs, f.writes, claims)
			}
		})
	}
}

func TestCollectWindowReuseRunFailureKeepsSingleDecision(t *testing.T) {
	f := newCollectReuseTestFixture()
	f.done = collectTargetBatchCompletion{}
	failure := errors.New("window query failed")
	f.ops.run = func(context.Context) error { f.runs++; return failure }
	err := runCollectTargetBatch(context.Background(), f.ops, f.now)
	if !errors.Is(err, failure) || f.runs != 1 || f.writes != 0 {
		t.Fatalf("real run failure changed: err=%v runs=%d writes=%d", err, f.runs, f.writes)
	}
	assertCollectReuseObservation(t, f, collectBatchReuseMiss, collectBatchReuseNoCompletion)
}
