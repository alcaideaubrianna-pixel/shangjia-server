package sys

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gotd/td/telegram"
)

func TestCollectSourceQueueNameIsStable(t *testing.T) {
	first := collectSourceQueueName(tgQueueNameCollect, 17)
	second := collectSourceQueueName(tgQueueNameCollect, 17)
	if first != second {
		t.Fatalf("collect source queue is not stable: %q != %q", first, second)
	}
	if first != "youban_publish_collect_source_17" {
		t.Fatalf("unexpected collect source queue: %q", first)
	}
}

func TestCollectQueueShardSeparatesSources(t *testing.T) {
	shards := make(map[string]struct{})
	for _, sourceId := range []int64{9, 10, 11, 14} {
		shards[collectSourceQueueName(tgQueueNameCollect, sourceId)] = struct{}{}
	}
	if len(shards) != 4 {
		t.Fatalf("expected each active source to use an isolated shard, got %d", len(shards))
	}
}

func TestExpectedTelegramObserveShutdownError(t *testing.T) {
	if !isExpectedTelegramObserveShutdownError(context.Canceled) {
		t.Fatal("context cancellation should be treated as an expected shutdown")
	}
	if !isExpectedTelegramObserveShutdownError(errors.New("sql: transaction has already been committed or rolled back")) {
		t.Fatal("completed transaction rollback should be treated as an expected shutdown")
	}
}

func TestAccountCollectOperationReturnsWhenTaskIsCanceled(t *testing.T) {
	worker := &accountCollectWorker{}
	taskCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- worker.runOperation(context.Background(), nil, accountCollectOperationTask{
			ctx: taskCtx,
			run: func(ctx context.Context, _ *telegram.Client) error {
				<-ctx.Done()
				return ctx.Err()
			},
		})
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("canceled account collect operation did not return")
	}
}
