package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	"hotgo/addons/telegram_collector/model/input/sysin"
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

func TestAccountRuntimePriorityOperationUsesReservedSlot(t *testing.T) {
	workerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	worker := &accountWorker{
		done:                   make(chan struct{}),
		operations:             make(chan accountOperationTask, 8),
		priorityOperations:     make(chan accountOperationTask, 2),
		operationSlots:         make(chan struct{}, accountRuntimeOperationConcurrency),
		priorityOperationSlots: make(chan struct{}, accountRuntimePriorityOperationConcurrency),
	}
	runtime := &accountRuntime{refresh: make(chan struct{}, 1), workers: map[int64]*accountWorker{7: worker}}
	go worker.runOperations(workerCtx, nil)
	go worker.runPriorityOperations(workerCtx, nil)

	release := make(chan struct{})
	for index := 0; index < accountRuntimeOperationConcurrency; index++ {
		go func() {
			_, _ = runtime.Execute(context.Background(), 7, time.Second, func(context.Context, *telegram.Client) error {
				<-release
				return nil
			})
		}()
	}
	deadline := time.Now().Add(time.Second)
	for len(worker.operationSlots) < accountRuntimeOperationConcurrency && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(worker.operationSlots) != accountRuntimeOperationConcurrency {
		close(release)
		t.Fatal("normal operation slots were not saturated")
	}

	used, err := runtime.ExecutePriority(context.Background(), 7, time.Second, func(context.Context, *telegram.Client) error { return nil })
	close(release)
	if err != nil || !used {
		t.Fatalf("priority operation used=%t err=%v", used, err)
	}
}

func TestAccountMessageEventPreservesMediaGroupAndMetadata(t *testing.T) {
	message := &tg.Message{
		ID: 17, Date: 1786435200, Message: "资料正文",
		PeerID: &tg.PeerChannel{ChannelID: 123},
	}
	message.SetMedia(&tg.MessageMediaPhoto{Photo: &tg.Photo{
		ID: 88, AccessHash: 99, FileReference: []byte("ref"), DCID: 2,
		Sizes: []tg.PhotoSizeClass{&tg.PhotoSize{Type: "x", W: 800, H: 600, Size: 1024}},
	}})
	message.SetGroupedID(9001)
	event := accountMessageEvent(&sysin.AccountRuntimeBinding{AccountID: 5}, accountMessageTask{
		source:  sysin.AccountRuntimeSource{TenantID: 1, AccountID: 2, SourceID: 3},
		message: message, chatID: "-100123",
	})
	if event == nil || event.SourceGroupedID != "9001" || len(event.Media) != 1 {
		t.Fatalf("unexpected event: %+v", event)
	}
	media := event.Media[0]
	if media.SourceMediaID != 88 || media.SourceAccessHash != 99 || media.SourceThumbSize != "x" {
		t.Fatalf("unexpected media metadata: %+v", media)
	}
}

func TestBuildAccountMessageEventIsStableForRealtimeAndHistory(t *testing.T) {
	source := sysin.AccountRuntimeSource{TenantID: 1, AccountID: 2, SourceID: 3, ChatID: "-100123"}
	message := &tg.Message{ID: 17, Date: 1786435200, Message: "资料正文"}
	message.SetGroupedID(9001)
	message.SetMedia(&tg.MessageMediaPhoto{Photo: &tg.Photo{ID: 88, AccessHash: 99, FileReference: []byte("ref"), DCID: 2}})

	realtime := BuildAccountMessageEvent(5, source, message, source.ChatID)
	history := BuildAccountMessageEvent(5, source, message, source.ChatID)
	if realtime == nil || history == nil {
		t.Fatal("account message event must be built")
	}
	if realtime.SourceUniqueKey != history.SourceUniqueKey || realtime.SourceGroupedID != history.SourceGroupedID {
		t.Fatalf("realtime/history identity drift: realtime=%+v history=%+v", realtime, history)
	}
	if len(realtime.Media) != 1 || realtime.Media[0].SourceMediaID != 88 || realtime.Media[0].SourceAccessHash != 99 {
		t.Fatalf("media metadata lost: %+v", realtime.Media)
	}
}

func TestMatchAccountRuntimeSourcesSupportsChannelFormats(t *testing.T) {
	sources := []sysin.AccountRuntimeSource{
		{SourceID: 1, ChatID: "123"},
		{SourceID: 2, ChatID: "-100123"},
		{SourceID: 3, ChatID: "999"},
	}
	matches := matchAccountRuntimeSources(sources, []string{"123", "-100123"})
	if len(matches) != 2 || matches[0].SourceID != 1 || matches[1].SourceID != 2 {
		t.Fatalf("unexpected matches: %+v", matches)
	}
}
