package sys

import (
	"strings"
	"testing"
	"time"
)

func TestTelegramSchedulerCollectPredecessorCondition(t *testing.T) {
	condition := telegramSchedulerCollectPredecessorCondition()
	checks := []string{
		"NOT EXISTS",
		"pj.collect_source_id = j.collect_source_id",
		"pj.collect_source_chat_id = j.collect_source_chat_id",
		"pj.collect_source_message_id < j.collect_source_message_id",
		"pj.channel_id = j.channel_id",
		"pj.next_retry_at <= NOW()",
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

func TestTelegramSchedulerChannelCacheKey(t *testing.T) {
	if got := telegramSchedulerChannelCacheKey(telegramSchedulerChannel{ChannelId: 24}); got != "youban_publish:tg_scheduler:candidates:channel:24" {
		t.Fatalf("channel cache key = %q", got)
	}
	if got := telegramSchedulerChannelCacheKey(telegramSchedulerChannel{TargetChatId: "-100123"}); got != "youban_publish:tg_scheduler:candidates:chat:-100123" {
		t.Fatalf("chat cache key = %q", got)
	}
}

func TestShouldInvalidateTelegramSchedulerChannelCache(t *testing.T) {
	urgent := telegramJobRecord{OperationNo: "profile:100:1"}
	bulk := telegramJobRecord{OperationNo: "full_push:100:1"}
	if !shouldInvalidateTelegramSchedulerChannelCache(urgent, 0) {
		t.Fatal("immediate urgent job should invalidate channel candidates")
	}
	if shouldInvalidateTelegramSchedulerChannelCache(urgent, time.Second) {
		t.Fatal("delayed urgent job should wait for its due scheduler scan")
	}
	if shouldInvalidateTelegramSchedulerChannelCache(bulk, 0) {
		t.Fatal("bulk job should not invalidate the short-lived channel cache")
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
