package sys

import "testing"

func TestCollectProcessThrottleKeyIsSourceScoped(t *testing.T) {
	first := collectProcessThrottleKey(collectProcessQueuePayload{TenantId: 2, AccountId: 3, SourceId: 4, EventId: 100})
	second := collectProcessThrottleKey(collectProcessQueuePayload{TenantId: 2, AccountId: 3, SourceId: 4, EventId: 200})
	if first != second || first != "youban_publish:collect:process:throttle:2:3:4" {
		t.Fatalf("unexpected throttle keys: %s %s", first, second)
	}
}

func TestCollectProcessKeysPartitionBySourceChat(t *testing.T) {
	first := collectProcessQueuePayload{TenantId: 2, AccountId: 3, SourceId: 4, SourceChatId: "-1003981090528"}
	sameChat := collectProcessQueuePayload{TenantId: 2, AccountId: 3, SourceId: 4, SourceChatId: "3981090528"}
	otherChat := collectProcessQueuePayload{TenantId: 2, AccountId: 3, SourceId: 4, SourceChatId: "3981090529"}
	if collectProcessScheduleKey(first) != collectProcessScheduleKey(sameChat) {
		t.Fatal("equivalent Telegram chat IDs must use the same schedule partition")
	}
	if collectProcessThrottleKey(first) != collectProcessThrottleKey(sameChat) {
		t.Fatal("equivalent Telegram chat IDs must use the same throttle partition")
	}
	if collectProcessScheduleKey(first) == collectProcessScheduleKey(otherChat) {
		t.Fatal("different chats under one source must use different schedule partitions")
	}
	if collectProcessThrottleKey(first) == collectProcessThrottleKey(otherChat) {
		t.Fatal("different chats under one source must use different throttle partitions")
	}
}

func TestCollectProcessRuleQueueKeyIsTenantScoped(t *testing.T) {
	first := collectProcessRuleQueueKey(collectProcessQueuePayload{TenantId: 2, RuleId: 9, SourceId: 4})
	same := collectProcessRuleQueueKey(collectProcessQueuePayload{TenantId: 2, RuleId: 9, SourceId: 8})
	otherTenant := collectProcessRuleQueueKey(collectProcessQueuePayload{TenantId: 3, RuleId: 9, SourceId: 4})
	if first != same {
		t.Fatalf("same tenant and rule must share logical queue: %s %s", first, same)
	}
	if first == otherTenant {
		t.Fatalf("different tenants must not share logical queue: %s", first)
	}
}

func TestCollectProcessRuleQueueKeyFallsBackToSourcePartition(t *testing.T) {
	first := collectProcessRuleQueueKey(collectProcessQueuePayload{TenantId: 2, SourceId: 4, SourceChatId: "-1003981090528"})
	sameChat := collectProcessRuleQueueKey(collectProcessQueuePayload{TenantId: 2, SourceId: 4, SourceChatId: "3981090528"})
	otherChat := collectProcessRuleQueueKey(collectProcessQueuePayload{TenantId: 2, SourceId: 4, SourceChatId: "3981090529"})
	if first != sameChat {
		t.Fatalf("equivalent Telegram chat IDs must share source fallback queue: %s %s", first, sameChat)
	}
	if first == otherChat {
		t.Fatalf("different source chats must not share source fallback queue: %s", first)
	}
}

func TestCollectProcessDispatchBatchSizeIsBounded(t *testing.T) {
	if got := normalizeCollectProcessDispatchBatchSize(0); got != collectProcessDispatchBatchDefault {
		t.Fatalf("unexpected default dispatch batch size: %d", got)
	}
	if got := normalizeCollectProcessDispatchBatchSize(5000); got != collectProcessDispatchBatchMaximum {
		t.Fatalf("dispatch batch size must be bounded: %d", got)
	}
}
