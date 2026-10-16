package sys

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestTelegramPendingRecoverySelectsChannelHeadsBeforeLimit(t *testing.T) {
	query := telegramPendingRecoveryChannelHeadSQL("j", "earlier")
	for _, fragment := range []string{
		"NOT EXISTS", "earlier.tenant_id=j.tenant_id", "j.channel_id>0",
		"earlier.channel_id=j.channel_id", "REPLACE(COALESCE(earlier.target_chat_id,''),'-100','')",
		"earlier.status IN ('pending','failed_retry','confirming','sending')",
		"earlier.status='failed' AND earlier.send_phase='display_confirmed'",
		"channel_display.job_id=earlier.id", "channel_display.purpose='display'",
		"earlier.created_at<j.created_at", "earlier.id<j.id",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("channel-head recovery query missing %q: %s", fragment, query)
		}
	}
}

func TestTelegramPendingRecoveryPostgresSelectsHeadsBeforeDueFilters(t *testing.T) {
	query := telegramPendingRecoveryPostgresSQL()
	if strings.Count(query, "(") != strings.Count(query, ")") {
		t.Fatalf("PostgreSQL recovery query has unbalanced parentheses: %s", query)
	}
	for _, fragment := range []string{
		"WITH channel_heads AS", "DISTINCT ON", "j.status IN ('pending','failed_retry','confirming','sending')",
		"j.status='failed' AND j.send_phase='display_confirmed'",
		"channel_display.job_id=j.id", "channel_display.purpose='display'",
		"ORDER BY j.tenant_id", "j.created_at, j.id", "h.status IN ('pending','failed_retry','unknown')",
		"COALESCE(h.dispatch_status,'') IN ('','idle')", "h.next_retry_at IS NULL OR h.next_retry_at<=?",
		"h.updated_at<=?", "ORDER BY h.created_at,h.id", "LIMIT ?",
	} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("PostgreSQL channel-head recovery query missing %q: %s", fragment, query)
		}
	}
	if got := strings.Count(query, "?"); got != 4 {
		t.Fatalf("PostgreSQL channel-head recovery placeholders=%d, want 4: %s", got, query)
	}
}

func TestTelegramPendingRecoveryProductionSnapshot(t *testing.T) {
	if os.Getenv("YOUBAN_PUBLISH_REPLAY") != "1" {
		t.Skip("set YOUBAN_PUBLISH_REPLAY=1 to run against an isolated production snapshot")
	}
	ctx := context.Background()
	now := gtime.Now()
	var jobs []telegramJobRecord
	err := g.DB().GetScan(ctx, &jobs, telegramPendingRecoveryPostgresSQL(),
		telegramRecoveryTimeText(now.Add(-messagePushPlanJobExpireAfter)),
		telegramRecoveryTimeText(now), telegramRecoveryTimeText(now), 2000)
	if err != nil {
		t.Fatalf("query production snapshot channel heads: %v", err)
	}
	if len(jobs) == 0 {
		t.Fatal("production snapshot returned no recoverable channel heads")
	}
	channels := make(map[string]struct{}, len(jobs))
	for _, job := range jobs {
		key := telegramPendingJobChannelKey(job)
		if _, exists := channels[key]; exists {
			t.Fatalf("production snapshot returned multiple recovery jobs for channel %s", key)
		}
		channels[key] = struct{}{}
		hasEarlier, checkErr := new(sSysPublish).telegramChannelHasEarlierActiveJob(ctx, job)
		if checkErr != nil {
			t.Fatalf("check earlier active job %d: %v", job.Id, checkErr)
		}
		if hasEarlier {
			t.Fatalf("recovery selected non-head job %d for channel %s", job.Id, key)
		}
	}
	t.Logf("validated %d real recoverable channel heads", len(jobs))
}

func TestTelegramDeliveryOrderingProductionSnapshot(t *testing.T) {
	if os.Getenv("YOUBAN_PUBLISH_REPLAY") != "1" {
		t.Skip("set YOUBAN_PUBLISH_REPLAY=1 to run against an isolated production snapshot")
	}
	ctx := context.Background()
	assertSnapshotCount(t, ctx, "normal jobs with orphan verification", `
		SELECT COUNT(1) FROM (
			SELECT j.id
			FROM hg_youban_publish_tg_job j
			JOIN hg_youban_publish_tg_message m ON m.job_id=j.id AND m.deleted_at IS NULL
			WHERE j.operation_no NOT LIKE 'verify-repair:%'
			GROUP BY j.id
			HAVING COUNT(*) FILTER (WHERE m.purpose='verify') > 0
				AND COUNT(*) FILTER (WHERE m.purpose='display') = 0
		) orphan`, 0)
	assertSnapshotCount(t, ctx, "jobs with verification before display completion", `
		SELECT COUNT(1) FROM (
			SELECT job_id
			FROM hg_youban_publish_tg_message
			WHERE deleted_at IS NULL
			GROUP BY job_id
			HAVING MIN(tg_message_id) FILTER (WHERE purpose='verify') <=
				MAX(tg_message_id) FILTER (WHERE purpose='display')
		) inverted`, 0)
	assertSnapshotCount(t, ctx, "normal verification attempts without confirmed display", `
		SELECT COUNT(DISTINCT verify.job_id)
		FROM hg_youban_publish_tg_attempt verify
		JOIN hg_youban_publish_tg_job job ON job.id=verify.job_id
		WHERE verify.phase='verify'
			AND job.operation_no NOT LIKE 'verify-repair:%'
			AND NOT EXISTS (
				SELECT 1 FROM hg_youban_publish_tg_attempt display
				WHERE display.job_id=verify.job_id
					AND display.phase='display' AND display.status='confirmed'
			)`, 0)
	assertSnapshotCount(t, ctx, "channels with more than one active delivery slot", `
		SELECT COUNT(1) FROM (
			SELECT tenant_id,
				CASE WHEN channel_id>0 THEN channel_id::text
				ELSE 'chat:' || REPLACE(COALESCE(target_chat_id,''),'-100','') END channel_key
			FROM hg_youban_publish_tg_job
			WHERE status IN ('sending','confirming')
				OR (status IN ('pending','failed_retry','unknown') AND dispatch_status IN ('queued','processing'))
			GROUP BY tenant_id,channel_key
			HAVING COUNT(*) > 1
		) concurrent`, 0)
}

func assertSnapshotCount(t *testing.T, ctx context.Context, name, query string, want int) {
	t.Helper()
	value, err := g.DB().GetValue(ctx, query)
	if err != nil {
		t.Fatalf("audit %s: %v", name, err)
	}
	if got := value.Int(); got != want {
		t.Fatalf("%s=%d, want %d", name, got, want)
	}
}

func telegramPendingJobChannelKey(job telegramJobRecord) string {
	return fmt.Sprintf("%d:%d:%s", job.TenantId, job.ChannelId, normalizeTelegramChannelChatID(job.TargetChatId))
}

func TestTelegramSendingRecoveryStartsAfterTaskTimeout(t *testing.T) {
	if telegramSendingJobRecoverAfter <= telegramPublishTaskTimeout {
		t.Fatalf("sending recovery %s must be later than task timeout %s", telegramSendingJobRecoverAfter, telegramPublishTaskTimeout)
	}
}

func TestTelegramFailedDisplayRecoveryMessage(t *testing.T) {
	if got := telegramFailedDisplayRecoveryMessage(0); !strings.Contains(got, "无验证媒体") {
		t.Fatalf("display-only recovery message = %q", got)
	}
	if got := telegramFailedDisplayRecoveryMessage(2); !strings.Contains(got, "验证视频") {
		t.Fatalf("verified recovery message = %q", got)
	}
}

func TestTelegramSendingAttemptRecoveryFollowsMediaDeadline(t *testing.T) {
	if telegramSendingAttemptRecoverAfter >= telegramPublishTaskTimeout {
		t.Fatalf("attempt recovery %s must not wait for queue task timeout %s", telegramSendingAttemptRecoverAfter, telegramPublishTaskTimeout)
	}
	if 2*telegramAttemptHeartbeatInterval >= telegramSendingAttemptRecoverAfter {
		t.Fatalf("attempt heartbeat %s must be shorter than recovery threshold %s", telegramAttemptHeartbeatInterval, telegramSendingAttemptRecoverAfter)
	}
}

func TestTelegramPreAttemptRecoveryIsPrompt(t *testing.T) {
	if telegramPreAttemptRecoverAfter > 2*time.Minute {
		t.Fatalf("pre-attempt recovery is too slow: %s", telegramPreAttemptRecoverAfter)
	}
	if telegramPreAttemptRecoverAfter >= telegramSendingJobRecoverAfter {
		t.Fatalf("pre-attempt recovery %s must be earlier than ambiguous sending recovery %s", telegramPreAttemptRecoverAfter, telegramSendingJobRecoverAfter)
	}
}

func TestTelegramPreAttemptPhasesExcludeExternalSend(t *testing.T) {
	phases := telegramPreAttemptPhases()
	want := map[string]bool{
		"": true, telegramSendPhaseCleanupProcessing: true, telegramSendPhaseCleanupConfirmed: true,
	}
	for _, phase := range phases {
		if !want[phase] {
			t.Fatalf("unsafe phase included in pre-attempt recovery: %q", phase)
		}
		delete(want, phase)
	}
	if len(want) != 0 {
		t.Fatalf("missing pre-attempt recovery phases: %+v", want)
	}
	for _, unsafe := range []string{
		telegramSendPhaseDisplaySending, telegramSendPhaseDisplayConfirmed,
		telegramSendPhaseVerifySending, telegramSendPhaseVerifyConfirmed,
	} {
		for _, phase := range phases {
			if phase == unsafe {
				t.Fatalf("external-send phase %q must require evidence reconciliation", phase)
			}
		}
	}
}

func TestStaleSendingAttemptRecoveryCoversResetJobs(t *testing.T) {
	for _, status := range []string{
		telegramJobStatusPending,
		telegramJobStatusSending,
		telegramJobStatusRetryWait,
		telegramJobStatusConfirming,
	} {
		if !telegramAttemptRecoverableJobStatus(status) {
			t.Fatalf("stale sending attempt under %s job must be recoverable", status)
		}
		if status != telegramJobStatusConfirming {
			if err := validateTelegramJobTransition([]string{status}, telegramJobStatusConfirming); err != nil {
				t.Fatalf("%s job cannot enter confirmation safely: %v", status, err)
			}
		}
	}
	for _, status := range []string{telegramJobStatusSent, telegramJobStatusFailed, telegramJobStatusSuperseded} {
		if telegramAttemptRecoverableJobStatus(status) {
			t.Fatalf("terminal %s job must not be recovered", status)
		}
	}
}

func TestStaleSendingPermanentAttemptDoesNotEnterUnknown(t *testing.T) {
	attempt := telegramDeliveryAttempt{
		Id: 1, Status: telegramAttemptStatusRetryWait,
		ErrorMessage: "bad request, Bad Request: chat not found",
	}
	if !telegramAttemptHasDefinitivePermanentFailure(attempt) {
		t.Fatal("a concrete chat-not-found response must terminate instead of entering unknown reconciliation")
	}
	attempt.ErrorMessage = "context deadline exceeded"
	if telegramAttemptHasDefinitivePermanentFailure(attempt) {
		t.Fatal("an ambiguous timeout must remain eligible for reconciliation")
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
	if !strings.Contains(condition, "status='authorized'") || !strings.Contains(condition, "session_key") {
		t.Fatalf("active channel condition must exclude unavailable protocol accounts: %s", condition)
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
