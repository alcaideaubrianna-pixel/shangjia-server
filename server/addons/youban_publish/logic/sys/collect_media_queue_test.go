package sys

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/hibiken/asynq"
)

func TestCollectMediaQueueShardKey(t *testing.T) {
	tests := []struct {
		name    string
		payload collectMediaQueuePayload
		want    int64
	}{
		{name: "account source", payload: collectMediaQueuePayload{TgAccountId: 74, SourceId: 364}, want: 74},
		{name: "bot source", payload: collectMediaQueuePayload{SourceId: 351}, want: 351},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := collectMediaQueueShardKey(test.payload); got != test.want {
				t.Fatalf("collectMediaQueueShardKey()=%d, want %d", got, test.want)
			}
		})
	}
}

func TestNormalizeCollectMediaOwnerConcurrency(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{input: 0, want: 1},
		{input: 2, want: 2},
		{input: 99, want: collectMediaMaxOwnerConcurrency},
	}
	for _, test := range tests {
		if got := normalizeCollectMediaOwnerConcurrency(test.input); got != test.want {
			t.Fatalf("normalizeCollectMediaOwnerConcurrency(%d)=%d, want %d", test.input, got, test.want)
		}
	}
}

func TestCollectMediaTaskTimeoutIsBounded(t *testing.T) {
	if collectMediaTaskTimeout > 15*time.Minute {
		t.Fatalf("media task timeout=%s is too long for interactive collection", collectMediaTaskTimeout)
	}
}

func TestDeferredCollectMediaEnqueueIsIdempotent(t *testing.T) {
	redis := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(opt)
	defer client.Close()
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	ctx := context.Background()
	payload := collectMediaQueuePayload{EventId: 42, TgAccountId: 7, SourceId: 9, Reason: "recovery"}
	queueName := collectMediaQueueName(ctx, payload)
	task := asynq.NewTask(tgTaskTypeCollectMedia, []byte(`{"eventId":42}`))
	options := []asynq.Option{asynq.Queue(queueName)}
	baseID := fmt.Sprintf("collect-media:%d:%s:%d", payload.EventId, payload.Reason, payload.DeferredAttempts)

	enqueued, err := enqueueCollectMediaCacheTaskWithID(ctx, client, inspector, task, queueName, options, baseID, 42)
	if err != nil || !enqueued {
		t.Fatalf("first enqueue=%t err=%v", enqueued, err)
	}
	enqueued, err = enqueueCollectMediaCacheTaskWithID(ctx, client, inspector, task, queueName, options, baseID, 42)
	if err != nil || enqueued {
		t.Fatalf("duplicate enqueue=%t err=%v, want skipped", enqueued, err)
	}
	deferredID := fmt.Sprintf("collect-media:%d:%s:%d", payload.EventId, payload.Reason, payload.DeferredAttempts+1)
	enqueued, err = enqueueCollectMediaCacheTaskWithID(ctx, client, inspector, task, queueName, options, deferredID, 42)
	if err != nil || !enqueued {
		t.Fatalf("next deferred attempt=%t err=%v, want queued", enqueued, err)
	}
	pending, err := inspector.ListPendingTasks(queueName)
	if err != nil || len(pending) != 2 {
		t.Fatalf("pending tasks=%d err=%v, want one task per recovery generation", len(pending), err)
	}

	if err := inspector.ArchiveTask(queueName, baseID); err != nil {
		t.Fatal(err)
	}
	enqueued, err = enqueueCollectMediaCacheTaskWithID(ctx, client, inspector, task, queueName, options, baseID, 42)
	if err != nil || !enqueued {
		t.Fatalf("repair enqueue=%t err=%v after archived generation", enqueued, err)
	}
	repaired, err := inspector.GetTaskInfo(queueName, baseID+":repair:1")
	if err != nil || repaired == nil {
		t.Fatalf("recovery generation missing: info=%v err=%v", repaired, err)
	}
}
