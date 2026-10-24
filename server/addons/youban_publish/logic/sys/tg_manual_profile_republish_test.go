package sys

import "testing"

func TestLateVerificationCannotTriggerReplacementPublish(t *testing.T) {
	if isManualProfilePublishOperation("verify-repair:4598733:profile:652900") {
		t.Fatal("legacy verification-only operation must remain retired")
	}
	if !isManualProfilePublishOperation("verify-misbinding-repair:4598733:profile:652900") {
		t.Fatal("historical verification repair must replace the complete publication")
	}
	if !isManualProfilePublishOperation("ai-republish:652900:1791600000000000000") {
		t.Fatal("explicit AI operations republish must remain replacement-style")
	}
	if !isManualProfilePublishOperation("bot_publish:20:123456:652900:1791600000000000000") {
		t.Fatal("BOT publish must be recognized as manual publish")
	}
}

func TestTelegramJobBypassesAutomaticSchedule(t *testing.T) {
	for _, operationNo := range []string{
		"profile:1", "batchtext:1", "ai-republish:1", "bot_publish:20:123:1:456",
	} {
		if !telegramJobBypassesAutomaticSchedule(telegramJobRecord{OperationNo: operationNo}) {
			t.Fatalf("manual operation %q must bypass automatic window and interval", operationNo)
		}
	}
	for _, operationNo := range []string{"collect:1", "cycle_batch:1", "full_push:1"} {
		if telegramJobBypassesAutomaticSchedule(telegramJobRecord{OperationNo: operationNo}) {
			t.Fatalf("automatic operation %q must keep automatic window and interval", operationNo)
		}
	}
}
