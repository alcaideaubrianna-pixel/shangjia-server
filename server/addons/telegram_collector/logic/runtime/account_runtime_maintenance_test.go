package runtime

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

func TestMaintenanceAccountRotationIsBounded(t *testing.T) {
	r := &accountRuntime{workers: make(map[int64]*accountWorker)}
	for id := int64(1); id <= 10; id++ {
		r.maintenanceAccounts = append(r.maintenanceAccounts, id)
		r.workers[id] = &accountWorker{done: make(chan struct{})}
	}
	for _, want := range [][]int64{{1, 2, 3, 4}, {5, 6, 7, 8}, {9, 10, 1, 2}} {
		got := r.nextMaintenanceAccounts()
		if len(got) != len(want) {
			t.Fatalf("batch=%v want=%v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("batch=%v want=%v", got, want)
			}
		}
	}
}

func TestMaintenanceRetainsStoppedWorkers(t *testing.T) {
	done := make(chan struct{})
	close(done)
	r := &accountRuntime{
		maintenanceAccounts: []int64{1, 2},
		workers:             map[int64]*accountWorker{1: {done: done}, 2: {done: make(chan struct{})}},
	}
	got := r.nextMaintenanceAccounts()
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("offline maintenance scope lost: %v", got)
	}
}

// Shared scope selection must drive maintenance even with no local worker.
type offlineMaintenanceTasks struct {
	collectorservice.IAccountTasks
	recovered []int64
}

func (s *offlineMaintenanceTasks) MaintenanceAccounts(context.Context, int) ([]int64, error) {
	return []int64{91}, nil
}
func (s *offlineMaintenanceTasks) RecoverExpired(_ context.Context, id int64, _ int) (int, error) {
	s.recovered = append(s.recovered, id)
	return 0, nil
}
func (s *offlineMaintenanceTasks) ExpirePending(context.Context, int64, string, time.Time, int) (int, error) {
	return 0, nil
}
func (s *offlineMaintenanceTasks) ListFinalizing(context.Context, int64, int) ([]*sysin.AccountTask, error) {
	return nil, nil
}
func TestMaintenanceSharedScopeIncludesOfflineAccount(t *testing.T) {
	mock := &offlineMaintenanceTasks{}
	r := &accountRuntime{}
	r.recoverAndObserveAccountTasksWith(context.Background(), mock)
	if len(mock.recovered) != 1 || mock.recovered[0] != 91 {
		t.Fatalf("offline account not recovered: %v", mock.recovered)
	}
}

func TestFinalizationCancellationStopsLaunchAndKeepsStartedOwnership(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	slots := make(chan struct{}, 1)
	var active sync.Map
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	finalize := func(context.Context, *sysin.AccountTask) {
		calls.Add(1)
		close(started)
		<-release // independent cleanup for an already-started callback
		close(finished)
	}
	done := make(chan struct{})
	go func() {
		finalizeAccountTaskBatch(ctx, []*sysin.AccountTask{{ID: 1}, {ID: 2}}, slots, &active, finalize)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first callback did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("maintenance blocked beyond cancellation")
	}
	if calls.Load() != 1 {
		t.Fatalf("new callback launched after cancellation: %d", calls.Load())
	}
	if _, ok := active.Load(int64(1)); !ok {
		t.Fatal("started callback lost ownership")
	}
	// A new pass must not start the same still-running callback again.
	finalizeAccountTaskBatch(context.Background(), []*sysin.AccountTask{{ID: 1}}, slots, &active, finalize)
	if calls.Load() != 1 {
		t.Fatal("duplicate callback across passes")
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not finish")
	}
}

func TestFinalizationAlreadyCancelledStartsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var active sync.Map
	calls := 0
	finalizeAccountTaskBatch(ctx, []*sysin.AccountTask{{ID: 5}}, make(chan struct{}, 1), &active, func(context.Context, *sysin.AccountTask) { calls++ })
	if calls != 0 {
		t.Fatal("cancelled batch launched cleanup")
	}
}
