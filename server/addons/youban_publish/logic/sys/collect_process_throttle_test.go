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
