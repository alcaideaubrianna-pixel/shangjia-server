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
}
