package sys

import (
	"context"
	"strings"
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

func TestTelegramJobPriorityClassifiesPublishOperations(t *testing.T) {
	cases := []struct {
		name        string
		operationNo string
		priority    int
		want        int
	}{
		{name: "manual profile", operationNo: "profile:123", want: tgJobPriorityUrgent},
		{name: "manual batch", operationNo: "batchtext:1:batch-1:profile:123", want: tgJobPriorityUrgent},
		{name: "message push", operationNo: "message_push:1:2:3", want: tgJobPriorityUrgent},
		{name: "full push", operationNo: "full_push:1:2", want: tgJobPriorityFullPush},
		{name: "cycle push", operationNo: "cycle_batch:1:2:3", want: tgJobPriorityBulk},
		{name: "explicit default", operationNo: "collect:1", priority: tgJobPriorityDefault, want: tgJobPriorityDefault},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			job := telegramJobRecord{OperationNo: testCase.operationNo, Priority: testCase.priority}
			if got := telegramJobPriorityValue(job); got != testCase.want {
				t.Fatalf("expected priority %d, got %d", testCase.want, got)
			}
		})
	}
}

func TestTelegramChannelAllowsOnlyOneActiveJob(t *testing.T) {
	if telegramChannelActiveJobLimit != 1 {
		t.Fatalf("channel active job limit=%d, want 1", telegramChannelActiveJobLimit)
	}
	if tgJobPriorityFullPush >= tgJobPriorityBulk {
		t.Fatalf("full push priority=%d must be higher than cycle bulk=%d", tgJobPriorityFullPush, tgJobPriorityBulk)
	}
	if queue := telegramQueueNameByPriority(tgJobPriorityFullPush); queue != tgQueueNameDefault {
		t.Fatalf("full push queue=%s, want %s", queue, tgQueueNameDefault)
	}
	if queue := telegramQueueNameByPriority(tgJobPriorityBulk); queue != tgQueueNameBulk {
		t.Fatalf("cycle queue=%s, want %s", queue, tgQueueNameBulk)
	}
	expression := telegramJobEffectivePrioritySQL("j")
	if !strings.Contains(expression, "j.operation_no LIKE 'full_push:%'") || !strings.Contains(expression, "THEN 70") {
		t.Fatalf("effective priority SQL does not prioritize existing full push jobs: %s", expression)
	}
}
