package sys

import "testing"

func TestTelegramJobBypassesAutomaticSchedule(t *testing.T) {
	for _, operationNo := range []string{
		"profile:1", "edit:1:request", "batchtext:1", "bot_publish:20:123:1:456",
	} {
		if !telegramJobBypassesAutomaticSchedule(telegramJobRecord{OperationNo: operationNo}) {
			t.Fatalf("manual operation %q must bypass automatic window and interval", operationNo)
		}
	}
	for _, operationNo := range []string{"collect:1", "cycle_batch:1", "full_push:1", "ai-republish:1", "verify-misbinding-repair:1"} {
		if telegramJobBypassesAutomaticSchedule(telegramJobRecord{OperationNo: operationNo}) {
			t.Fatalf("automatic operation %q must keep automatic window and interval", operationNo)
		}
	}
}

func TestInteractivePublishOperationDoesNotIncludeUnpublish(t *testing.T) {
	if isInteractiveProfilePublishOperation("bot_unpublish:20:-100123:88:123456") {
		t.Fatal("bot_unpublish must not enter the publish lifecycle")
	}
	if !isInteractiveProfileUnpublishOperation("bot_unpublish:20:-100123:88:123456") {
		t.Fatal("bot_unpublish must be recognized as an unpublish operation")
	}
}
