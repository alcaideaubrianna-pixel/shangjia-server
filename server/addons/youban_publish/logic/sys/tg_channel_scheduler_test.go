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

func TestTelegramJobPriorityManualProfilePublishIsUrgent(t *testing.T) {
	tests := []struct {
		name     string
		job      telegramJobRecord
		priority int
		urgent   bool
	}{
		{name: "manual profile publish", job: telegramJobRecord{OperationNo: "profile:12:345", Priority: tgJobPriorityDefault}, priority: tgJobPriorityUrgent, urgent: true},
		{name: "bulk full push", job: telegramJobRecord{OperationNo: "full_push:12:345"}, priority: tgJobPriorityBulk, urgent: false},
		{name: "scheduled publish", job: telegramJobRecord{OperationNo: "scheduled:12:345"}, priority: tgJobPriorityDefault, urgent: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := (&sSysPublish{}).telegramJobPriority(test.job); got != test.priority {
				t.Fatalf("unexpected priority: got=%d want=%d", got, test.priority)
			}
			if got := isTelegramUrgentJob(test.job); got != test.urgent {
				t.Fatalf("unexpected urgent flag: got=%t want=%t", got, test.urgent)
			}
		})
	}
}
