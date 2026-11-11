package sys

import "testing"

// TestCollectProcessAccountQueueIsolation 验证采集事件、历史任务和恢复任务都按账户稳定路由。
func TestCollectProcessAccountQueueIsolation(t *testing.T) {
	first := collectProcessAccountQueueName(collectProcessQueuePayload{TenantId: 87, AccountId: 628})
	if first == "" {
		t.Fatal("account queue is empty")
	}
	if first != collectProcessAccountQueueName(collectProcessQueuePayload{TenantId: 87, AccountId: 628}) {
		t.Fatal("same account queue is not stable")
	}
	if first == collectProcessAccountQueueName(collectProcessQueuePayload{TenantId: 87, AccountId: 629}) && collectProcessAccountQueueShards > 1 {
		// Hash collision is valid; the assertion below verifies routing does not
		// depend on event, rule, or source message identifiers.
		if collectProcessQueueName(collectProcessQueuePayload{TenantId: 87, AccountId: 628, EventId: 1, SourceId: 10}) != first {
			t.Fatal("event id changed account routing")
		}
	}
	historyQueue := collectProcessQueueName(collectProcessQueuePayload{TenantId: 87, AccountId: 628, History: true})
	if historyQueue == first {
		t.Fatal("history queue must be isolated from realtime queue")
	}
	for _, payload := range []collectProcessQueuePayload{
		{TenantId: 87, AccountId: 628, EventId: 1, SourceId: 10, OutboxOwner: "owner-1"},
		{TenantId: 87, AccountId: 628, EventId: 999, SourceId: 99, Recovery: true, OutboxOwner: "owner-2"},
		{TenantId: 87, AccountId: 628, EventId: 777, SourceId: 77, History: true, OutboxOwner: "outbox-1"},
	} {
		got := collectProcessQueueName(payload)
		if payload.History || payload.Recovery {
			if got != historyQueue {
				t.Fatalf("payload %+v escaped history account shard: got=%s want=%s", payload, got, historyQueue)
			}
		} else if got != first {
			t.Fatalf("payload %+v escaped account shard: got=%s want=%s", payload, got, first)
		}
	}
}
