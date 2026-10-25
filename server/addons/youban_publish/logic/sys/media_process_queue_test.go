package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
)

func TestMediaProcessLegacyFailureUpgrade(t *testing.T) {
	for _, version := range []int{0, 1} {
		var payload mediaProcessQueuePayload
		body := `{"mediaId":123}`
		if version > 0 {
			body = `{"mediaId":123,"retryPolicyVersion":1}`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatal(err)
		}
		if got := mediaProcessFailureIsTerminal(mediaProcessingFailed, payload); got != (version > 0) {
			t.Fatalf("version=%d terminal=%v", version, got)
		}
		for retry := 0; retry < 3; retry++ {
			want := mediaProcessingRetry
			if retry == 2 {
				want = mediaProcessingFailed
			}
			if got := mediaProcessFailureStatus(context.Background(), context.DeadlineExceeded, retry); got != want {
				t.Fatalf("version=%d retry=%d got=%s", version, retry, got)
			}
		}
	}
}

func TestMediaProcessArchivedRepairQueue(t *testing.T) {
	redis := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(opt)
	defer client.Close()
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	ctx := context.Background()
	enqueue := func() error { return enqueueMediaProcessTask(ctx, client, inspector, 123, time.Second) }
	if err := enqueue(); err != nil {
		t.Fatal(err)
	}
	if err := inspector.ArchiveTask(tgQueueNameMediaProcess, "media-process:123"); err != nil {
		t.Fatal(err)
	}
	// Concurrent manual repair and scheduler recovery must produce one task.
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := enqueue(); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	archived, err := inspector.ListArchivedTasks(tgQueueNameMediaProcess)
	if err != nil {
		t.Fatal(err)
	}
	scheduled, err := inspector.ListScheduledTasks(tgQueueNameMediaProcess)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 || len(scheduled) != 1 {
		t.Fatalf("archived=%d scheduled=%d", len(archived), len(scheduled))
	}
	current, err := inspector.GetTaskInfo(tgQueueNameMediaProcess, "media-process:123:repair:1")
	if err != nil {
		t.Fatal(err)
	}
	var payload mediaProcessQueuePayload
	if err := json.Unmarshal(current.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.RetryPolicyVersion != 1 || current.MaxRetry != 2 {
		t.Fatalf("task=%+v payload=%+v", current, payload)
	}
	if err := inspector.RunTask(tgQueueNameMediaProcess, current.ID); err != nil {
		t.Fatal(err)
	}
	if err := enqueue(); err != nil {
		t.Fatal(err)
	}
	pending, err := inspector.ListPendingTasks(tgQueueNameMediaProcess)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending=%d err=%v", len(pending), err)
	}
	if err := inspector.ArchiveTask(tgQueueNameMediaProcess, current.ID); err != nil {
		t.Fatal(err)
	}
	if err := enqueue(); err != nil {
		t.Fatal(err)
	}
	if _, err := inspector.GetTaskInfo(tgQueueNameMediaProcess, "media-process:123:repair:2"); err != nil {
		t.Fatal(err)
	}
}

func TestMediaProcessFailureStatus(t *testing.T) {
	for attempt := 0; attempt < 5; attempt++ {
		want := mediaProcessingRetry
		if attempt >= 2 {
			want = mediaProcessingFailed
		}
		err := fmt.Errorf("download failed: %w", context.DeadlineExceeded)
		if got := mediaProcessFailureStatus(context.Background(), err, attempt); got != want {
			t.Fatalf("retry=%d status=%s want=%s", attempt, got, want)
		}
	}
	if got := mediaProcessFailureStatus(context.Background(), &mediaFileCacheHTTPStatusError{statusCode: 404}, 0); got != mediaProcessingFailed {
		t.Fatal("404 must not retry")
	}
}

func TestTerminalMediaProcessingStatuses(t *testing.T) {
	statuses := terminalMediaProcessingStatuses()
	want := map[string]bool{
		mediaProcessingReady:  true,
		mediaProcessingFailed: true,
	}
	if len(statuses) != len(want) {
		t.Fatalf("terminal status count = %d, want %d", len(statuses), len(want))
	}
	for _, status := range statuses {
		if !want[status] {
			t.Fatalf("unexpected terminal media status %q", status)
		}
	}
}

func TestMediaProcessRecoveryAndPublishRetryIntervalsStayBounded(t *testing.T) {
	if mediaProcessRecoveryEvery <= 0 || mediaProcessRecoveryEvery > time.Minute {
		t.Fatalf("media recovery interval = %s, want within one minute", mediaProcessRecoveryEvery)
	}
	if telegramMediaReadyRetry < mediaProcessRecoveryEvery || telegramMediaReadyRetry > 2*time.Minute {
		t.Fatalf("media publish retry = %s, want recovery interval through two minutes", telegramMediaReadyRetry)
	}
	if telegramMediaReadyRetry >= telegramPublishTaskTimeout {
		t.Fatalf("media publish retry = %s, must remain below task timeout %s", telegramMediaReadyRetry, telegramPublishTaskTimeout)
	}
}
