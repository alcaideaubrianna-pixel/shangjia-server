package sys

import (
	"strings"
	"testing"
)

func TestTelegramSchedulerCollectPredecessorCondition(t *testing.T) {
	condition := telegramSchedulerCollectPredecessorCondition()
	checks := []string{
		"NOT EXISTS",
		"pj.collect_source_id = j.collect_source_id",
		"pj.collect_source_chat_id = j.collect_source_chat_id",
		"pj.collect_source_message_id < j.collect_source_message_id",
		"pj.channel_id = j.channel_id",
	}
	for _, check := range checks {
		if !strings.Contains(condition, check) {
			t.Fatalf("scheduler predecessor condition is missing %q", check)
		}
	}
}
