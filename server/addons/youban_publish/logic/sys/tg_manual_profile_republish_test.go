package sys

import "testing"

func TestLateVerificationCannotTriggerReplacementPublish(t *testing.T) {
	if isManualProfilePublishOperation("verify-repair:4598733:profile:652900") {
		t.Fatal("late verification must not replace an already published profile")
	}
	if !isManualProfilePublishOperation("ai-republish:652900:1791600000000000000") {
		t.Fatal("explicit AI operations republish must remain replacement-style")
	}
}
