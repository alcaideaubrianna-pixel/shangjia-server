package runtime

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestAccountTaskDispatcherReturnsUnhandedBatchOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	accounted := make(chan int64, 8)
	runAccountTaskDispatcher(ctx, 8, func(context.Context, int) ([]*sysin.AccountTask, bool) {
		cancel() // Cancellation races the handoff after the durable claim.
		return []*sysin.AccountTask{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}, {ID: 6}, {ID: 7}, {ID: 8}}, true
	}, func(_ context.Context, task *sysin.AccountTask) {
		accounted <- task.ID
	}, func(context.Context, time.Duration) {}, cancel, func(tasks []*sysin.AccountTask) {
		for _, task := range tasks {
			accounted <- task.ID
		}
	})
	close(accounted)
	seen := make(map[int64]bool)
	for id := range accounted {
		if seen[id] {
			t.Fatalf("claimed task %d was both dispatched and returned", id)
		}
		seen[id] = true
	}
	if len(seen) != 8 {
		t.Fatalf("unhanded claims leaked on shutdown: %v", seen)
	}
}

func TestAccountTaskDispatcherIdleUsesSinglePoller(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls int
	var delays []time.Duration
	runAccountTaskDispatcher(ctx, 8, func(context.Context, int) ([]*sysin.AccountTask, bool) {
		calls++
		return nil, true
	}, func(context.Context, *sysin.AccountTask) { t.Error("unexpected task") }, func(_ context.Context, delay time.Duration) {
		delays = append(delays, delay)
		if len(delays) == 4 {
			cancel()
		}
	}, cancel)
	if calls != 4 {
		t.Fatalf("idle claims=%d, want 4", calls)
	}
	for i, want := range []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 8 * time.Second} {
		if delays[i] != want {
			t.Fatalf("delay[%d]=%v want %v", i, delays[i], want)
		}
	}
}

func TestAccountTaskDispatcherDoesNotClaimBeyondFreeWorkers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var claims atomic.Int32
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		runAccountTaskDispatcher(ctx, 2, func(_ context.Context, limit int) ([]*sysin.AccountTask, bool) {
			claims.Add(1)
			if limit != 2 {
				t.Errorf("initial capacity=%d", limit)
			}
			return []*sysin.AccountTask{{ID: 1}, {ID: 2}}, true
		}, func(ctx context.Context, _ *sysin.AccountTask) {
			started <- struct{}{}
			select {
			case <-ctx.Done():
			case <-release:
			}
		}, func(context.Context, time.Duration) {}, cancel)
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("workers did not start")
		}
	}
	if claims.Load() != 1 {
		t.Fatalf("claims while saturated=%d", claims.Load())
	}
	cancel()
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("dispatcher did not stop")
	}
}

// A partial claim must execute returned tasks and release unused capacity.
func TestAccountTaskDispatcherExecutesPartialBatch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	executed := make(chan int64, 1)
	calls := 0
	runAccountTaskDispatcher(ctx, 8, func(context.Context, int) ([]*sysin.AccountTask, bool) {
		calls++
		if calls == 1 {
			return []*sysin.AccountTask{{ID: 77}}, true
		}
		return nil, true
	}, func(_ context.Context, task *sysin.AccountTask) {
		executed <- task.ID
	}, func(ctx context.Context, _ time.Duration) {
		select {
		case id := <-executed:
			if id != 77 {
				t.Errorf("executed task=%d, want 77", id)
			}
			cancel()
		case <-ctx.Done():
			t.Error("partial batch was not executed")
		}
	}, cancel)
}
