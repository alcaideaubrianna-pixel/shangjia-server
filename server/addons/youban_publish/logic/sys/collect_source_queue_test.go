package sys

import (
	"fmt"
	"testing"
)

// TestCollectProcessSourceQueueIsolation 验证 BOT account_id=0 按租户和来源分片，
// 不会因为事件 ID 不同而漂移到不同队列。
func TestCollectProcessSourceQueueIsolation(t *testing.T) {
	first := collectProcessQueuePayload{
		TenantId: 78, AccountId: 0, SourceId: 351, SourceChatId: "-100123",
		EventId: 1, OutboxOwner: "owner-1",
	}
	sameSource := first
	sameSource.EventId = 999
	sameSource.OutboxOwner = "owner-2"
	if got, want := collectProcessQueueName(first), collectProcessQueueName(sameSource); got != want {
		t.Fatalf("同一 BOT 来源必须固定到同一队列: got=%s want=%s", got, want)
	}
	if got := collectProcessQueueName(first); got != collectProcessSourceQueueName(first) {
		t.Fatalf("BOT 来源没有使用来源分片队列: %s", got)
	}
	history := first
	history.History = true
	if got := collectProcessQueueName(history); got == collectProcessQueueName(first) {
		t.Fatalf("BOT 历史任务不能与实时任务共用队列: %s", got)
	}
}

func TestCollectProcessSourceQueuesRegistered(t *testing.T) {
	queues := collectProcessWorkerQueues()
	for shard := 0; shard < collectProcessSourceQueueShards; shard++ {
		realtime := fmt.Sprintf("%s%02d", tgQueueNameCollectProcessSourceRealtimePrefix, shard)
		recovery := fmt.Sprintf("%s%02d", tgQueueNameCollectProcessSourceRecoveryPrefix, shard)
		if queues[realtime] != 2 || queues[recovery] != 1 {
			t.Fatalf("BOT 来源队列未注册或优先级错误 realtime=%s recovery=%s", realtime, recovery)
		}
	}
}
