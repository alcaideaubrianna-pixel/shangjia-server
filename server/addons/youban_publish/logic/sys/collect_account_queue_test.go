package sys

import "testing"

// TestCollectProcessAccountQueueIsolation 验证采集事件、历史任务和恢复任务都按账户稳定路由。
func TestCollectProcessAccountQueueIsolation(t *testing.T) {
	first := collectProcessQueueName(collectProcessQueuePayload{TenantId: 87, AccountId: 628, SourceId: 10, EventId: 1})
	if first == "" {
		t.Fatal("source queue is empty")
	}
	if first != collectProcessQueueName(collectProcessQueuePayload{TenantId: 87, AccountId: 628, SourceId: 10, EventId: 2}) {
		t.Fatal("same source queue is not stable")
	}
	otherSource := collectProcessQueueName(collectProcessQueuePayload{TenantId: 87, AccountId: 628, SourceId: 11, EventId: 1})
	if first == otherSource {
		t.Fatal("different sources must not share a physical queue")
	}
	history := collectProcessQueueName(collectProcessQueuePayload{TenantId: 87, AccountId: 628, SourceId: 10, History: true, Recovery: true})
	if history == first {
		t.Fatal("history source queue must be isolated from realtime source queue")
	}
	if got := collectProcessQueueName(collectProcessQueuePayload{TenantId: 87, AccountId: 629, SourceId: 10, EventId: 1}); got == first {
		t.Fatal("different accounts must not share a source queue")
	}
}
