package sys

import (
	"strings"
	"testing"
	"time"
)

func TestTelegramSendingRecoveryStartsAfterTaskTimeout(t *testing.T) {
	if telegramSendingJobRecoverAfter <= telegramPublishTaskTimeout {
		t.Fatalf("sending recovery %s must be later than task timeout %s", telegramSendingJobRecoverAfter, telegramPublishTaskTimeout)
	}
}

func TestTelegramMediaFallbackRecoveryIsPromptAndUnambiguous(t *testing.T) {
	if telegramMediaFallbackRecoverAfter > 2*time.Minute {
		t.Fatalf("fallback recovery is too slow: %s", telegramMediaFallbackRecoverAfter)
	}
	for _, purpose := range []string{"display", "verify"} {
		reason := telegramMediaFallbackReason(purpose)
		if !telegramMediaFallbackJob(reason) {
			t.Fatalf("fallback marker was not recognized: %s", reason)
		}
		if got := telegramMediaFallbackPurpose(reason); got != purpose {
			t.Fatalf("fallback purpose = %q, want %q", got, purpose)
		}
	}
	if telegramMediaFallbackJob("network timeout") {
		t.Fatal("ordinary send errors must not enter fallback recovery")
	}
}

func TestTelegramMediaFallbackRecoveryBackoffIsBounded(t *testing.T) {
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 0, want: 2 * time.Minute},
		{attempt: 1, want: 4 * time.Minute},
		{attempt: 2, want: 8 * time.Minute},
		{attempt: 3, want: 16 * time.Minute},
		{attempt: 4, want: 30 * time.Minute},
		{attempt: 20, want: 30 * time.Minute},
	}
	for _, test := range tests {
		if got := telegramMediaFallbackRecoveryDelay(test.attempt); got != test.want {
			t.Fatalf("attempt %d delay = %s, want %s", test.attempt, got, test.want)
		}
	}
}

func TestTelegramChannelBusyDelayBackoff(t *testing.T) {
	initial := telegramChannelBusyDelayDuration(3, 10, 0)
	busy := telegramChannelBusyDelayDuration(3, 10, 20)
	max := telegramChannelBusyDelayDuration(3, 10, 1000)
	if busy <= initial {
		t.Fatalf("busy delay must grow: initial=%s busy=%s", initial, busy)
	}
	if max > 30*time.Second {
		t.Fatalf("busy delay must be capped: %s", max)
	}
}

func TestTelegramActiveChannelConditionSupportsMessagePushTargets(t *testing.T) {
	condition := telegramActiveChannelCondition()

	for _, operation := range []string{"message_push:%", "message_push_plan:%"} {
		if !strings.Contains(condition, "operation_no LIKE '"+operation+"'") {
			t.Fatalf("active channel condition does not cover %s", operation)
		}
	}
	if !strings.Contains(condition, publishTgChannelTable) {
		t.Fatalf("active channel condition does not validate TG channel cache")
	}
	if !strings.Contains(condition, publishChannelTable) {
		t.Fatalf("active channel condition does not validate publish channels")
	}
}

func TestMessagePushPlanRecoveryConditionExcludesExpiredPlanJobs(t *testing.T) {
	condition := messagePushPlanRecoveryConditionSQL("j")
	if !strings.Contains(condition, "j.operation_no LIKE 'message_push_plan:%'") {
		t.Fatalf("condition does not identify message push plan jobs: %s", condition)
	}
	if !strings.Contains(condition, "j.created_at < ?") {
		t.Fatalf("condition does not enforce plan expiration deadline: %s", condition)
	}
	if messagePushPlanJobExpireAfter != 30*time.Minute {
		t.Fatalf("unexpected message push plan expiration: %s", messagePushPlanJobExpireAfter)
	}
}

func TestFairPendingTelegramJobsSelectsOnePerChannel(t *testing.T) {
	candidates := []telegramJobRecord{
		{Id: 1, TenantId: 1, ChannelId: 10},
		{Id: 2, TenantId: 1, ChannelId: 10},
		{Id: 3, TenantId: 1, ChannelId: 20},
	}
	jobs := fairPendingTelegramJobs(candidates, 10)
	if len(jobs) != 2 || jobs[0].Id != 1 || jobs[1].Id != 3 {
		t.Fatalf("unexpected fair candidates: %+v", jobs)
	}
}
