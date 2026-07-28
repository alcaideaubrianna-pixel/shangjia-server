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

func TestTelegramSchedulerCandidateLimitPreventsChannelStarvation(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "default batch", limit: 50, want: 5000},
		{name: "small batch minimum", limit: 1, want: 1000},
		{name: "large batch cap", limit: 200, want: 10000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := telegramSchedulerCandidateLimit(test.limit); got != test.want {
				t.Fatalf("candidate limit = %d, want %d", got, test.want)
			}
		})
	}
}

func TestIsManualProfilePublishOperation(t *testing.T) {
	tests := map[string]struct {
		operationNo string
		want        bool
	}{
		"manual profile":   {operationNo: "profile:12:345", want: true},
		"case insensitive": {operationNo: " PROFILE:12:345 ", want: true},
		"cycle":            {operationNo: "cycle_batch:12:345", want: false},
		"full push":        {operationNo: "full_push:12:345", want: false},
		"down":             {operationNo: "down:12:345", want: false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := isManualProfilePublishOperation(test.operationNo); got != test.want {
				t.Fatalf("unexpected manual publish flag: got=%t want=%t", got, test.want)
			}
		})
	}
}
