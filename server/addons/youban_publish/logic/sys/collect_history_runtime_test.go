package sys

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gotd/td/telegram"
)

func TestCollectHistoryRuntimeWaitError(t *testing.T) {
	err := newCollectHistoryRuntimeWaitError(5)
	if err.delay != 5*time.Second {
		t.Fatalf("delay = %s, want 5s", err.delay)
	}
	if got, want := err.Error(), "TG账号共享连接正在建立，历史采集等待后自动重试 tgAccountId:5"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestAccountCollectOperationsShareWorker(t *testing.T) {
	service := NewSysPublish()
	worker := &accountCollectWorker{
		tgAccountId: 5,
		operations:  make(chan accountCollectOperationTask, 2),
		mediaSlots:  make(chan struct{}, 1),
	}
	service.registerAccountCollectWorker(worker)
	defer service.unregisterAccountCollectWorker(worker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go worker.runOperationLoop(ctx, nil)

	var active atomic.Int32
	var maxActive atomic.Int32
	var executed atomic.Int32
	run := func(context.Context, *telegram.Client) error {
		current := active.Add(1)
		for {
			maximum := maxActive.Load()
			if current <= maximum || maxActive.CompareAndSwap(maximum, current) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		active.Add(-1)
		executed.Add(1)
		return nil
	}

	var wait sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			usedRuntime, err := service.executeAccountCollectOperation(ctx, 5, time.Second, run)
			if !usedRuntime {
				errors <- context.Canceled
				return
			}
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("operation failed: %v", err)
		}
	}
	if got := executed.Load(); got != 2 {
		t.Fatalf("executed = %d, want 2", got)
	}
	if got := maxActive.Load(); got != 1 {
		t.Fatalf("max active operations = %d, want 1 shared serial client", got)
	}
}

func TestAccountCollectOperationWaitsWithoutWorker(t *testing.T) {
	service := NewSysPublish()
	usedRuntime, err := service.executeAccountCollectOperation(context.Background(), 5, time.Second, func(context.Context, *telegram.Client) error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usedRuntime {
		t.Fatal("operation must not report a runtime when the account worker is unavailable")
	}
}

func TestRefreshAccountCollectSupervisorCoalescesSignals(t *testing.T) {
	service := NewSysPublish()
	service.refreshAccountCollectSupervisor()
	service.refreshAccountCollectSupervisor()
	if got := len(service.accountRuntimeRefresh); got != 1 {
		t.Fatalf("refresh signals = %d, want 1", got)
	}
}

func TestCollectHistoryTransientClientError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "context canceled", err: context.Canceled, want: true},
		{name: "wrapped client closed", err: errors.New("拉取历史消息失败: client closed: context canceled"), want: true},
		{name: "dc closed", err: errors.New("rpcDoRequest: DC is closed"), want: true},
		{name: "business error", err: errors.New("频道AccessHash无效"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := collectHistoryTransientClientError(test.err); got != test.want {
				t.Fatalf("transient = %v, want %v", got, test.want)
			}
		})
	}
}

func TestAccountCollectWorkerFailsPendingOperationsOnExit(t *testing.T) {
	worker := &accountCollectWorker{operations: make(chan accountCollectOperationTask, 1)}
	done := make(chan error, 1)
	worker.operations <- accountCollectOperationTask{done: done}
	want := newCollectMediaRetryError("共享连接启动失败", time.Second)
	worker.failPendingOperations(want)
	select {
	case got := <-done:
		if got != want {
			t.Fatalf("pending operation error = %v, want %v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("pending operation was not released")
	}
}

func TestEnqueueCollectProcessWakesDispatcher(t *testing.T) {
	service := NewSysPublish()
	err := service.enqueueCollectProcess(context.Background(), collectProcessQueuePayload{
		TenantId:  2,
		AccountId: 1,
		SourceId:  19,
	}, 0)
	if err != nil {
		t.Fatalf("enqueue collect process failed: %v", err)
	}
	if got := len(service.collectProcessRefresh); got != 1 {
		t.Fatalf("dispatcher signals = %d, want 1", got)
	}
}

func TestAccountCollectWorkerConfigHotUpdate(t *testing.T) {
	worker := &accountCollectWorker{
		signature: "old",
		sources:   []accountCollectSourceRuntime{{Id: 1}},
	}
	changed := worker.updateConfig(
		"new",
		[]accountCollectSourceRuntime{{Id: 20}},
		[]accountListenPlanRuntime{{Id: 30}},
	)
	if !changed {
		t.Fatal("config update must report a change")
	}
	sources, listeners := worker.configSnapshot()
	if len(sources) != 1 || sources[0].Id != 20 {
		t.Fatalf("sources = %+v, want source 20", sources)
	}
	if len(listeners) != 1 || listeners[0].Id != 30 {
		t.Fatalf("listeners = %+v, want listener 30", listeners)
	}
	if worker.updateConfig("new", sources, listeners) {
		t.Fatal("same signature must not trigger another update")
	}
}
