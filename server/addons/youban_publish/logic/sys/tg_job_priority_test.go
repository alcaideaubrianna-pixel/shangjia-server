package sys

import (
	"context"
	"testing"
)

func TestTelegramQueueNameByPriorityAndChannel(t *testing.T) {
	ctx := context.Background()

	if first, second := telegramQueueNameByPriorityAndChannel(ctx, tgJobPriorityBulk, 1), telegramQueueNameByPriorityAndChannel(ctx, tgJobPriorityBulk, 17); first != second {
		t.Fatalf("same shard channels should use the same queue: %s != %s", first, second)
	}
	if first, second := telegramQueueNameByPriorityAndChannel(ctx, tgJobPriorityBulk, 1), telegramQueueNameByPriorityAndChannel(ctx, tgJobPriorityBulk, 2); first == second {
		t.Fatalf("different shard channels should not use the same queue: %s", first)
	}
	if got := telegramQueueNameByPriorityAndChannel(ctx, tgJobPriorityBulk, 0); got != tgQueueNameBulk {
		t.Fatalf("channel-less job should use the legacy queue: %s", got)
	}
}

func TestTelegramPublishQueueWeightsKeepLegacyQueues(t *testing.T) {
	foreground := telegramPublishForegroundQueueWeights(context.Background())
	bulk := telegramPublishBulkQueueWeights(context.Background())
	for _, queue := range []string{tgQueueNameUrgent, tgQueueNameDefault} {
		if foreground[queue] <= 0 {
			t.Fatalf("foreground queue %s is not registered", queue)
		}
	}
	if bulk[tgQueueNameBulk] <= 0 {
		t.Fatalf("legacy bulk queue is not registered")
	}
	if bulk[telegramQueueNameByPriorityAndChannel(context.Background(), tgJobPriorityBulk, 1)] <= 0 {
		t.Fatal("bulk shard queue is not registered")
	}
}
