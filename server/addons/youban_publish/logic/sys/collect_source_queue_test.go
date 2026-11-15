package sys

import "testing"

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
	payload := collectProcessQueuePayload{TenantId: 56, AccountId: 561, SourceId: 1370, EventId: 1}
	realtime := collectProcessQueueName(payload)
	recovery := collectProcessQueueName(collectProcessQueuePayload{TenantId: 56, AccountId: 561, SourceId: 1370, EventId: 1, Recovery: true, History: true})
	if realtime == recovery || realtime == "" || recovery == "" {
		t.Fatalf("source realtime/recovery queues are not isolated: realtime=%s recovery=%s", realtime, recovery)
	}
}
