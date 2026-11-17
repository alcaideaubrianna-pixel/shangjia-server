package sys

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"

	"hotgo/addons/telegram_collector/consts"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestSourceReplayBypassesUrgentBacklogWithoutRemovingOriginal(t *testing.T) {
	redis := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })
	inspector := asynq.NewInspector(opt)
	t.Cleanup(func() { _ = inspector.Close() })
	collector := &sCollector{queueClient: client, queueInspector: inspector}
	ctx := context.Background()
	const eventID int64 = 12682221

	// Preserve a UUID-style task from the legacy producer during rolling upgrades.
	if _, err := client.Enqueue(asynq.NewTask(eventTaskType, collectorTaskPayload(eventID)),
		asynq.Queue(consts.QueueCollectUrgent), asynq.MaxRetry(eventMaxAttempts-1), asynq.Timeout(eventLeaseDuration)); err != nil {
		t.Fatal(err)
	}
	if err := collector.enqueueEventTask(ctx, eventID, sysin.EventPriorityUrgent); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := collector.enqueueSourceReplayEventTask(ctx, eventID); err != nil {
			t.Fatal(err)
		}
	}
	for _, queue := range []string{consts.QueueCollectUrgent, consts.QueueCollectUrgentLive, consts.QueueCollectHistory} {
		tasks, err := inspector.ListPendingTasks(queue)
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) != 1 {
			t.Fatalf("queue=%s pending=%d, want one deduplicated task", queue, len(tasks))
		}
		task := tasks[0]
		if task.Type != eventTaskType || string(task.Payload) != "12682221" {
			t.Fatalf("repair changed handler or event identity: type=%s payload=%s", task.Type, task.Payload)
		}
		if task.MaxRetry != eventMaxAttempts-1 || task.Timeout != eventLeaseDuration {
			t.Fatalf("repair changed retry or execution budget: retries=%d timeout=%s", task.MaxRetry, task.Timeout)
		}
	}
	history, err := inspector.ListPendingTasks(consts.QueueCollectRealtime)
	if err != nil && !errors.Is(err, asynq.ErrQueueNotFound) {
		t.Fatal(err)
	}
	if len(history) != 0 {
		t.Fatalf("repair unexpectedly entered realtime account queue: %d", len(history))
	}
}
