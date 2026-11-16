package sys

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCollectMediaSourceRecoveryRotatesExhaustedPhasesAndLanes(t *testing.T) {
	scopes := []publishAccountScope{{TenantID: 1, AccountID: 2, SourceID: 3}, {TenantID: 1, AccountID: 2, SourceID: 4}}
	names := []string{"stale_download", "pending_media", "bot_media", "ready_media", "missing_dispatch"}
	seen := map[string]bool{}
	pairs := map[int64]map[string]bool{}
	steps := make([]collectMediaSourceRecoveryStep, len(names))
	var cancelPass context.CancelFunc
	for i, name := range names {
		steps[i] = collectMediaSourceRecoveryStep{name: name, run: func(ctx context.Context, budget int, got ...publishAccountScope) error {
			if budget != 7 || len(got) != 1 {
				t.Fatalf("scope/cap lost: %d %v", budget, got)
			}
			seen[name] = true
			if pairs[got[0].SourceID] == nil {
				pairs[got[0].SourceID] = map[string]bool{}
			}
			pairs[got[0].SourceID][name] = true
			// Model a phase consuming the entire pass without real DB or sleeps.
			cancelPass()
			return ctx.Err()
		}}
	}
	for turn := uint64(0); turn < 10; turn++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancelPass = cancel
		count := 0
		runCollectMediaSourceRecovery(ctx, scopes, steps, 7, turn, time.Second, func(scope publishAccountScope, phase string, err error) {
			count++
			if scope.SourceID != scopes[int((turn/uint64(len(names)))%2)].SourceID {
				t.Fatal("lane did not rotate")
			}
		})
		cancel()
		if count != 1 {
			t.Fatalf("canceled pass ran %d phases", count)
		}
	}
	for _, scope := range scopes {
		if len(pairs[scope.SourceID]) != len(names) {
			t.Fatalf("source %d starved: %v", scope.SourceID, pairs)
		}
	}
	if len(seen) != len(names) || !seen["missing_dispatch"] {
		t.Fatalf("starved phases: %v", seen)
	}
}

func TestCollectMediaSourceRecoveryErrorsAreIndependent(t *testing.T) {
	scopes := []publishAccountScope{{TenantID: 1, AccountID: 2, SourceID: 3}, {TenantID: 1, AccountID: 2, SourceID: 4}}
	failure := errors.New("isolated failure")
	calls, reports := 0, 0
	steps := []collectMediaSourceRecoveryStep{
		{name: "failure", run: func(context.Context, int, ...publishAccountScope) error { calls++; return failure }},
		{name: "missing_dispatch", run: func(ctx context.Context, _ int, _ ...publishAccountScope) error {
			calls++
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > time.Second {
				t.Fatal("lane budget missing")
			}
			return nil
		}},
	}
	runCollectMediaSourceRecovery(context.Background(), scopes, steps, 5, 0, time.Second, func(_ publishAccountScope, _ string, err error) {
		reports++
		if !errors.Is(err, failure) {
			t.Fatal(err)
		}
	})
	if calls != 4 || reports != 2 {
		t.Fatalf("calls=%d reports=%d", calls, reports)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runCollectMediaSourceRecovery(ctx, scopes, steps, 5, 0, time.Second, nil)
	if calls != 4 {
		t.Fatal("canceled pass executed work")
	}
}
