package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/gotd/td/telegram"
)

func TestAccountRuntimeRefreshCoalescesSignals(t *testing.T) {
	runtime := &accountRuntime{refresh: make(chan struct{}, 1), workers: make(map[int64]*accountWorker)}
	runtime.Refresh()
	runtime.Refresh()
	if got := len(runtime.refresh); got != 1 {
		t.Fatalf("refresh signals = %d, want 1", got)
	}
}

func TestAccountRuntimeRestartRemovesWorker(t *testing.T) {
	runtime := &accountRuntime{refresh: make(chan struct{}, 1), workers: make(map[int64]*accountWorker)}
	workerCtx, cancel := context.WithCancel(context.Background())
	worker := &accountWorker{cancel: cancel, done: make(chan struct{})}
	runtime.workers[7] = worker
	runtime.Restart(7)
	if _, ok := runtime.workers[7]; ok {
		t.Fatal("restarted worker must be removed before resync")
	}
	select {
	case <-workerCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("restarted worker context was not canceled")
	}
	if got := len(runtime.refresh); got != 1 {
		t.Fatalf("refresh signals = %d, want 1", got)
	}
}

func TestAccountRuntimeExecuteWithoutWorker(t *testing.T) {
	runtime := &accountRuntime{refresh: make(chan struct{}, 1), workers: make(map[int64]*accountWorker)}
	used, err := runtime.Execute(context.Background(), 9, time.Second, func(context.Context, *telegram.Client) error { return nil })
	if err != nil || used {
		t.Fatalf("execute used=%t err=%v", used, err)
	}
}
