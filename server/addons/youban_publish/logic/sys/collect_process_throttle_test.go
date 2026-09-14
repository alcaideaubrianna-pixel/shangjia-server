package sys

import "testing"

func TestCollectProcessThrottleKeyIsSourceScoped(t *testing.T) {
	first := collectProcessThrottleKey(collectProcessQueuePayload{TenantId: 2, AccountId: 3, SourceId: 4, EventId: 100})
	second := collectProcessThrottleKey(collectProcessQueuePayload{TenantId: 2, AccountId: 3, SourceId: 4, EventId: 200})
	if first != second || first != "youban_publish:collect:process:throttle:2:3:4" {
		t.Fatalf("unexpected throttle keys: %s %s", first, second)
	}
}
