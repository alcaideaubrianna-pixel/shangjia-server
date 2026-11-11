package sys

import (
	"context"
	"testing"
)

// TestCollectMediaQueuePrioritizesAccount verifies shared Bot usage cannot merge different account media work.
func TestCollectMediaQueuePrioritizesAccount(t *testing.T) {
	first := collectMediaQueueName(context.Background(), collectMediaQueuePayload{TenantId: 87, AccountId: 628, TgAccountId: 54, EventId: 1})
	second := collectMediaQueueName(context.Background(), collectMediaQueuePayload{TenantId: 87, AccountId: 629, TgAccountId: 54, EventId: 2})
	if first == second {
		t.Fatal("different accounts sharing one Bot must use separate media shards")
	}
	botFallback := collectMediaQueueName(context.Background(), collectMediaQueuePayload{TgAccountId: 54, EventId: 3})
	if botFallback != collectMediaRealtimeQueueName(54) {
		t.Fatalf("unowned Bot media should retain Bot fallback shard: got=%s", botFallback)
	}
}
