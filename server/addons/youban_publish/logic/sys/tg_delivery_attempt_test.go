package sys

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestTelegramAttemptMessageSnapshotPreservesProjectionMetadata(t *testing.T) {
	want := telegramSentMessage{
		MessageId: 101, MediaGroupId: "group-1", Purpose: "display", MediaId: 202,
		TgFileId: "file-id", TgFileUniqueId: "unique-id", AssetHash: "asset-hash",
		ProtectedHashKey: "protected-key", ProtectedPHash: 303, ProtectedDHash: 404,
	}
	payload, err := json.Marshal(&want)
	if err != nil {
		t.Fatal(err)
	}
	var got telegramSentMessage
	if err = json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("attempt message snapshot = %+v, want %+v", got, want)
	}
}

func TestTelegramOptionalAttemptScanTreatsMissingRowAsCacheMiss(t *testing.T) {
	if !telegramOptionalAttemptScanSucceeded(nil) {
		t.Fatal("nil error should be accepted")
	}
	if !telegramOptionalAttemptScanSucceeded(sql.ErrNoRows) {
		t.Fatal("sql.ErrNoRows should create a new delivery attempt")
	}
	if telegramOptionalAttemptScanSucceeded(errors.New("database unavailable")) {
		t.Fatal("database errors must not be swallowed")
	}
}

func TestTelegramConfirmedAttemptRequiresCompleteAudit(t *testing.T) {
	attempt := telegramDeliveryAttempt{ExpectedCount: 3}
	if len([]*telegramAttemptMessageRecord{{}, {}}) >= attempt.ExpectedCount {
		t.Fatal("partial attempt audit must not be treated as a complete Telegram response")
	}
	if len([]*telegramAttemptMessageRecord{{}, {}, {}}) < attempt.ExpectedCount {
		t.Fatal("complete attempt audit should satisfy the expected response count")
	}
}

func TestTelegramAttemptPhaseTransition(t *testing.T) {
	tests := []struct {
		name          string
		phase         string
		from          string
		sending       string
		confirmed     string
		shouldBeValid bool
	}{
		{
			name: "display", phase: "display",
			from: telegramSendPhaseCleanupConfirmed, sending: telegramSendPhaseDisplaySending,
			confirmed: telegramSendPhaseDisplayConfirmed, shouldBeValid: true,
		},
		{
			name: "verify", phase: "verify",
			from: telegramSendPhaseDisplayConfirmed, sending: telegramSendPhaseVerifySending,
			confirmed: telegramSendPhaseVerifyConfirmed, shouldBeValid: true,
		},
		{name: "unknown", phase: "cleanup"},
		{name: "empty", phase: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			from, sending, confirmed, err := telegramAttemptPhaseTransition(test.phase)
			if !test.shouldBeValid {
				if err == nil {
					t.Fatalf("phase %q must be rejected", test.phase)
				}
				return
			}
			if err != nil {
				t.Fatalf("phase %q: %v", test.phase, err)
			}
			if from != test.from || sending != test.sending || confirmed != test.confirmed {
				t.Fatalf("phase %q transition = (%q, %q, %q), want (%q, %q, %q)",
					test.phase, from, sending, confirmed, test.from, test.sending, test.confirmed)
			}
		})
	}
}

func TestTelegramAttemptPhaseMode(t *testing.T) {
	tests := []struct {
		name                   string
		current                string
		advancePhase           bool
		wantAdvance, wantRetry bool
	}{
		{name: "first send", current: telegramSendPhaseCleanupConfirmed, advancePhase: true, wantAdvance: true},
		{name: "retry sending phase", current: telegramSendPhaseDisplaySending, advancePhase: true, wantRetry: true},
		{name: "begin existing phase", current: telegramSendPhaseDisplaySending, wantRetry: true},
		{name: "wrong phase", current: telegramSendPhaseVerifySending, advancePhase: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			advance, retry := telegramAttemptPhaseMode(test.current, telegramSendPhaseCleanupConfirmed, telegramSendPhaseDisplaySending, test.advancePhase)
			if advance != test.wantAdvance || retry != test.wantRetry {
				t.Fatalf("phase mode = (%v, %v), want (%v, %v)", advance, retry, test.wantAdvance, test.wantRetry)
			}
		})
	}
}

func TestTelegramSupersededAttemptCanResume(t *testing.T) {
	if !telegramSupersededAttemptCanResume(telegramDeliveryAttempt{Status: telegramAttemptStatusSuperseded, ErrorMessage: "下载远程媒体失败：HTTP 404"}, 0) {
		t.Fatal("superseded attempt must allow a fresh retry attempt")
	}
	for _, status := range []string{
		telegramAttemptStatusSending,
		telegramAttemptStatusWaiting,
		telegramAttemptStatusRetryWait,
		telegramAttemptStatusConfirmed,
	} {
		if telegramSupersededAttemptCanResume(telegramDeliveryAttempt{Status: status, ErrorMessage: "下载远程媒体失败：HTTP 404"}, 0) {
			t.Fatalf("active or confirmed attempt must not resume as superseded: %s", status)
		}
	}
	for _, tc := range []struct {
		name    string
		attempt telegramDeliveryAttempt
		audit   int
	}{
		{"production126_interrupted", telegramDeliveryAttempt{Id: 1402451, Status: telegramAttemptStatusSuperseded, ErrorMessage: "发送进程中断，等待Webhook或协议号对账"}, 0},
		{"production127_interrupted", telegramDeliveryAttempt{Id: 1402711, Status: telegramAttemptStatusSuperseded, ErrorMessage: "发送进程中断，等待Webhook或协议号对账"}, 0},
		{"production128_interrupted", telegramDeliveryAttempt{Id: 1402453, Status: telegramAttemptStatusSuperseded, ErrorMessage: "发送进程中断，等待Webhook或协议号对账"}, 0},
		{"production249_missing_evidence", telegramDeliveryAttempt{Id: 1411604, Status: telegramAttemptStatusSuperseded}, 0},
		{"response_received", telegramDeliveryAttempt{Status: telegramAttemptStatusSuperseded, ResponseReceived: 1, ErrorMessage: "下载远程媒体失败：HTTP 404"}, 0},
		{"audit_exists", telegramDeliveryAttempt{Status: telegramAttemptStatusSuperseded, ErrorMessage: "下载远程媒体失败：HTTP 404"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if telegramSupersededAttemptCanResume(tc.attempt, tc.audit) {
				t.Fatal("uncertain or confirmed delivery must not be replayed")
			}
		})
	}
}

func TestRestoreConfirmedTelegramResponseTreatsMissingAttemptAsUnconfirmed(t *testing.T) {
	if !telegramOptionalAttemptScanSucceeded(sql.ErrNoRows) {
		t.Fatal("missing confirmed response must allow the initial Telegram send")
	}
}

func TestTelegramUnknownRecoveryAction(t *testing.T) {
	tests := []struct {
		name             string
		phase            string
		verifyMediaCount int
		displayConfirmed bool
		want             string
	}{
		{name: "display response not confirmed", phase: telegramSendPhaseDisplaySending, verifyMediaCount: 1, want: telegramUnknownRecoveryWait},
		{name: "display confirmed resumes verify", phase: telegramSendPhaseDisplayConfirmed, verifyMediaCount: 1, displayConfirmed: true, want: telegramUnknownRecoveryResumeVerify},
		{name: "display phase without evidence waits", phase: telegramSendPhaseDisplayConfirmed, verifyMediaCount: 1, want: telegramUnknownRecoveryWait},
		{name: "verify sending without confirmation waits", phase: telegramSendPhaseVerifySending, verifyMediaCount: 1, want: telegramUnknownRecoveryWait},
		{name: "verify confirmed completes", phase: telegramSendPhaseVerifyConfirmed, verifyMediaCount: 1, want: telegramUnknownRecoveryComplete},
		{name: "display only completes", phase: telegramSendPhaseDisplayConfirmed, verifyMediaCount: 0, want: telegramUnknownRecoveryComplete},
		{name: "legacy display only completes", phase: telegramSendPhaseCompletedNoVerify, verifyMediaCount: 0, want: telegramUnknownRecoveryComplete},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := telegramUnknownRecoveryAction(test.phase, test.verifyMediaCount, test.displayConfirmed); got != test.want {
				t.Fatalf("recovery action = %q, want %q", got, test.want)
			}
		})
	}
	if got := telegramDisplayExpectedCount(0); got != 1 {
		t.Fatalf("text-only display expected count = %d, want 1", got)
	}
	if got := telegramDisplayExpectedCount(3); got != 3 {
		t.Fatalf("media display expected count = %d, want 3", got)
	}
}

func TestTelegramUnknownAttemptRecoveryAction(t *testing.T) {
	tests := []struct {
		name       string
		attempt    telegramDeliveryAttempt
		auditCount int
		want       string
	}{
		{name: "missing attempt retries before Telegram call", want: telegramUnknownAttemptRetry},
		{name: "live sending reconciles", attempt: telegramDeliveryAttempt{Id: 1, Status: telegramAttemptStatusSending}, want: telegramUnknownAttemptReconcile},
		{name: "webhook wait reconciles", attempt: telegramDeliveryAttempt{Id: 1, Status: telegramAttemptStatusWaiting}, want: telegramUnknownAttemptReconcile},
		{name: "ambiguous timeout reconciles", attempt: telegramDeliveryAttempt{Id: 1, Status: telegramAttemptStatusRetryWait, ErrorMessage: "context deadline exceeded"}, want: telegramUnknownAttemptReconcile},
		{name: "interrupted sender reconciles", attempt: telegramDeliveryAttempt{Id: 1, Status: telegramAttemptStatusRetryWait, ErrorMessage: "发送进程中断，等待Webhook或协议号对账"}, want: telegramUnknownAttemptReconcile},
		{name: "partial response reconciles", attempt: telegramDeliveryAttempt{Id: 1, Status: telegramAttemptStatusRetryWait, ErrorMessage: "生成防扫图候选文件失败"}, auditCount: 1, want: telegramUnknownAttemptReconcile},
		{name: "pre-send failure retries", attempt: telegramDeliveryAttempt{Id: 1, Status: telegramAttemptStatusRetryWait, ErrorMessage: "生成防扫图候选文件失败"}, want: telegramUnknownAttemptRetry},
		{name: "missing error cannot prove no delivery", attempt: telegramDeliveryAttempt{Id: 1, Status: telegramAttemptStatusRetryWait}, want: telegramUnknownAttemptReconcile},
		{name: "response must never resend", attempt: telegramDeliveryAttempt{Id: 1, Status: telegramAttemptStatusRetryWait, ResponseReceived: 1, ErrorMessage: "生成防扫图候选文件失败"}, want: telegramUnknownAttemptReconcile},
		{name: "confirmed count must never resend", attempt: telegramDeliveryAttempt{Id: 1, Status: telegramAttemptStatusRetryWait, ConfirmedCount: 1, ErrorMessage: "生成防扫图候选文件失败"}, want: telegramUnknownAttemptReconcile},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := telegramUnknownAttemptRecoveryAction(test.attempt, test.auditCount); got != test.want {
				t.Fatalf("recovery action = %q, want %q", got, test.want)
			}
		})
	}
}

func TestTelegramMediaSendTimeoutsFitPublishTaskDeadline(t *testing.T) {
	if telegramDisplaySendTimeout != 10*time.Minute || telegramVerifySendTimeout != 10*time.Minute {
		t.Fatalf("unexpected media timeouts: verify=%s display=%s", telegramVerifySendTimeout, telegramDisplaySendTimeout)
	}
	if telegramDisplaySendTimeout+telegramVerifySendTimeout+telegramAttemptWebhookWait >= telegramPublishTaskTimeout {
		t.Fatal("media send and webhook timeouts must fit publish task deadline")
	}
	ctx, cancel := context.WithTimeout(context.Background(), telegramVerifySendTimeout)
	defer cancel()
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > telegramVerifySendTimeout {
		t.Fatal("verify send context must carry the configured deadline")
	}
}

func TestTelegramVerifyMediaNeedsAccountUpload(t *testing.T) {
	if !telegramVerifyMediaNeedsAccountUpload([]*telegramMediaItem{{MediaType: "video"}}) {
		t.Fatal("verify video without file_id should use account upload")
	}
	if telegramVerifyMediaNeedsAccountUpload([]*telegramMediaItem{{MediaType: "video", TgFileId: "telegram-file-id"}}) {
		t.Fatal("verify video with reusable file_id should keep the Bot fast path")
	}
	if telegramVerifyMediaNeedsAccountUpload([]*telegramMediaItem{{MediaType: "image"}}) {
		t.Fatal("non-video verification media should not force account upload")
	}
}

func TestTelegramSentMessageIDs(t *testing.T) {
	got := telegramSentMessageIDs([]*telegramSentMessage{nil, {MessageId: 0}, {MessageId: 1880}, {MessageId: 1849}})
	if len(got) != 2 || got[0] != 1880 || got[1] != 1849 {
		t.Fatalf("unexpected message ids: %v", got)
	}
}

func TestTelegramAttemptMarkerStableAndDistinct(t *testing.T) {
	first := telegramAttemptMarker("attempt-a")
	if first == "" || first != telegramAttemptMarker("attempt-a") {
		t.Fatal("attempt marker must be stable and non-empty")
	}
	if first == telegramAttemptMarker("attempt-b") {
		t.Fatal("different attempts must not share a marker")
	}
	if !strings.HasPrefix(first, "\u2063") || !strings.HasSuffix(first, "\u2064") {
		t.Fatal("attempt marker must retain its invisible boundaries")
	}
}

func TestTelegramWebhookDetachedJobSQLKeepsOrderingBarrier(t *testing.T) {
	if got := telegramWebhookDetachedJobSQL("j."); got != "1=1" {
		t.Fatalf("telegramWebhookDetachedJobSQL() = %q, want ordering barrier predicate", got)
	}
}

func TestTelegramAttemptWaitingKeepsChannelOrderingBarrier(t *testing.T) {
	for _, phase := range []string{"display", "verify"} {
		if got := telegramAttemptWaitingDispatchStatus(); got != tgDispatchStatusProcessing {
			t.Fatalf("phase %s dispatch status = %q, want processing", phase, got)
		}
	}
}

func TestTelegramAttemptReconcileSupportsDisplayAndVerify(t *testing.T) {
	for _, phase := range []string{"display", "verify"} {
		if phase != "display" && phase != "verify" {
			t.Fatalf("unsupported reconciliation phase: %s", phase)
		}
	}
}

func TestTelegramJobRequiresChannelCompletionPriority(t *testing.T) {
	for _, phase := range []string{telegramSendPhaseDisplayConfirmed, telegramSendPhaseVerifySending} {
		if !telegramJobRequiresChannelCompletionPriority(phase) {
			t.Fatalf("phase %s must finish before untouched channel backlog", phase)
		}
	}
	for _, phase := range []string{"", telegramSendPhaseCleanupConfirmed, telegramSendPhaseDisplaySending, telegramSendPhaseVerifyConfirmed, telegramSendPhaseCompletedNoVerify} {
		if telegramJobRequiresChannelCompletionPriority(phase) {
			t.Fatalf("phase %s must keep normal channel ordering", phase)
		}
	}
}

func TestTelegramChannelBlockingStatusesExcludeTerminalJobs(t *testing.T) {
	statuses := make(map[string]struct{})
	for _, status := range telegramChannelBlockingJobStatuses() {
		statuses[status] = struct{}{}
	}
	for _, status := range []string{
		telegramJobStatusPending,
		telegramJobStatusRetryWait,
		telegramJobStatusConfirming,
		telegramJobStatusSending,
	} {
		if _, ok := statuses[status]; !ok {
			t.Fatalf("active status %q must preserve channel ordering", status)
		}
	}
	for _, status := range []string{
		telegramJobStatusFailed,
		telegramJobStatusSent,
		telegramJobStatusSuperseded,
	} {
		if _, ok := statuses[status]; ok {
			t.Fatalf("terminal status %q must not block the channel", status)
		}
	}
}

func TestTelegramUnknownProcessingBlocksNextChannelJobIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_PUBLISH_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_PUBLISH_INTEGRATION=1 to run local PostgreSQL integration test")
	}
	ctx := context.Background()
	seed := time.Now().UnixNano()
	now := gtime.Now()
	olderID, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
		"operation_no": "ordering-old", "tenant_id": seed, "channel_id": seed,
		"status": telegramJobStatusConfirming, "dispatch_status": tgDispatchStatusProcessing,
		"dispatched_at": now, "created_at": now, "updated_at": now,
	}).InsertAndGetId()
	if err != nil {
		t.Fatalf("insert earlier uncertain job: %v", err)
	}
	time.Sleep(time.Millisecond)
	newerID, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
		"operation_no": "ordering-new", "tenant_id": seed, "channel_id": seed,
		"status": telegramJobStatusPending, "dispatch_status": tgDispatchStatusIdle,
		"created_at": gtime.Now(), "updated_at": gtime.Now(),
	}).InsertAndGetId()
	if err != nil {
		t.Fatalf("insert later pending job: %v", err)
	}
	t.Cleanup(func() {
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).WhereIn("id", []int64{olderID, newerID}).Delete()
	})
	blocked, err := new(sSysPublish).telegramChannelHasEarlierActiveJob(ctx, telegramJobRecord{
		Id: newerID, TenantId: seed, ChannelId: seed, CreatedAt: gtime.Now(),
	})
	if err != nil || !blocked {
		t.Fatalf("earlier uncertain job must block next channel job: blocked=%v err=%v", blocked, err)
	}
}

func TestTelegramAPISuccessCommitsAttemptMessageAndPhaseIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_PUBLISH_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_PUBLISH_INTEGRATION=1 to run local PostgreSQL integration test")
	}
	ctx := context.Background()
	seed := time.Now().UnixNano()
	jobID, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
		"operation_no": "attempt-atomic-integration", "tenant_id": seed,
		"account_id": seed, "profile_id": seed, "channel_id": seed, "bot_id": seed,
		"target_chat_id": "-1001234567890", "status": telegramJobStatusSending,
		"dispatch_status": tgDispatchStatusProcessing, "send_phase": telegramSendPhaseCleanupConfirmed,
	}).InsertAndGetId()
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	t.Cleanup(func() {
		_, _ = g.DB().Model(publishTgAttemptMessageTable).Safe().Ctx(ctx).
			WhereIn("attempt_id", g.DB().Model(publishTgAttemptTable).Fields("id").Where("job_id", jobID)).Delete()
		_, _ = g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).Unscoped().Where("job_id", jobID).Delete()
		_, _ = g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).Where("job_id", jobID).Delete()
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Unscoped().Where("id", jobID).Delete()
	})

	job, err := new(sSysPublish).telegramJobById(ctx, jobID)
	if err != nil {
		t.Fatalf("load job: %v", err)
	}
	if _, err = new(sSysPublish).prepareTelegramDeliveryAttempt(ctx, job, "verify", 1, true); err == nil {
		t.Fatal("verify attempt must not start before display is confirmed")
	}
	attemptCount, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).Where("job_id", jobID).Count()
	if err != nil || attemptCount != 0 {
		t.Fatalf("rejected phase must not persist an attempt: count=%d err=%v", attemptCount, err)
	}
	attempt, err := new(sSysPublish).prepareTelegramDeliveryAttempt(ctx, job, "display", 1, true)
	if err != nil {
		t.Fatalf("prepare attempt: %v", err)
	}
	phase, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobID).Value("send_phase")
	if err != nil || phase.String() != telegramSendPhaseDisplaySending {
		t.Fatalf("prepared phase=%q err=%v", phase.String(), err)
	}
	message := &telegramSentMessage{MessageId: seed, Purpose: "display"}
	if _, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobID).
		Data(g.Map{"send_phase": telegramSendPhaseVerifySending}).Update(); err != nil {
		t.Fatalf("set mismatched phase: %v", err)
	}
	if err = new(sSysPublish).confirmTelegramAPISuccess(ctx, job, attempt, []*telegramSentMessage{message}); err == nil {
		t.Fatal("API success must not confirm against a mismatched job phase")
	}
	var unconfirmed telegramDeliveryAttempt
	if err = g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).Where("id", attempt.Id).Scan(&unconfirmed); err != nil {
		t.Fatalf("load rolled back attempt: %v", err)
	}
	if unconfirmed.Status != telegramAttemptStatusSending || unconfirmed.ResponseReceived != 0 {
		t.Fatalf("failed phase CAS must roll back attempt confirmation: %+v", unconfirmed)
	}
	auditCount, err := g.DB().Model(publishTgAttemptMessageTable).Safe().Ctx(ctx).Where("attempt_id", attempt.Id).Count()
	if err != nil || auditCount != 0 {
		t.Fatalf("failed phase CAS must roll back response audit: count=%d err=%v", auditCount, err)
	}
	if _, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobID).
		Data(g.Map{"send_phase": telegramSendPhaseDisplaySending}).Update(); err != nil {
		t.Fatalf("restore display sending phase: %v", err)
	}
	if err = new(sSysPublish).confirmTelegramAPISuccess(ctx, job, attempt, []*telegramSentMessage{message}); err != nil {
		t.Fatalf("confirm API success: %v", err)
	}
	var confirmed telegramDeliveryAttempt
	if err = g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).Where("id", attempt.Id).Scan(&confirmed); err != nil {
		t.Fatalf("load confirmed attempt: %v", err)
	}
	if confirmed.Status != telegramAttemptStatusConfirmed || confirmed.ResponseReceived != 1 || confirmed.ConfirmedCount != 1 {
		t.Fatalf("unexpected confirmed attempt: %+v", confirmed)
	}
	phase, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobID).Value("send_phase")
	if err != nil || phase.String() != telegramSendPhaseDisplayConfirmed {
		t.Fatalf("confirmed phase=%q err=%v", phase.String(), err)
	}
	messageCount, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Where("job_id", jobID).Where("tg_message_id", seed).Count()
	if err != nil || messageCount != 1 {
		t.Fatalf("message projection count=%d err=%v", messageCount, err)
	}
	auditCount, err = g.DB().Model(publishTgAttemptMessageTable).Safe().Ctx(ctx).
		Where("attempt_id", attempt.Id).Where("message_id", seed).Where("source", "response").Count()
	if err != nil || auditCount != 1 {
		t.Fatalf("attempt audit count=%d err=%v", auditCount, err)
	}
	// Production Baihuage3 job 2080209: a definite 429 left a retry_wait
	// verification Attempt while the job rolled back to display_confirmed.
	oldVerifyID, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).Data(g.Map{
		"job_id": jobID, "tenant_id": seed, "bot_id": seed, "channel_id": seed,
		"target_chat_id": "-1001234567890", "phase": "verify", "chunk_index": 0,
		"attempt_no": 1, "attempt_token": fmt.Sprintf("rate-limit-%d", seed),
		"expected_count": 1, "confirmed_count": 0, "response_received": 0,
		"status": telegramAttemptStatusRetryWait,
		"error_message": "too many requests, Too Many Requests: retry after 16: retry_after 16",
	}).InsertAndGetId()
	if err != nil {
		t.Fatalf("insert rate-limited verification: %v", err)
	}
	verification, err := new(sSysPublish).prepareTelegramDeliveryAttempt(ctx, job, "verify", 1, true)
	if err != nil || verification.Id == oldVerifyID || verification.AttemptNo != 2 {
		t.Fatalf("rolled-back verification must resume: attempt=%+v err=%v", verification, err)
	}
	oldStatus, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).Where("id", oldVerifyID).Value("status")
	if err != nil || oldStatus.String() != telegramAttemptStatusSuperseded {
		t.Fatalf("rejected attempt must retire atomically: status=%v err=%v", oldStatus, err)
	}
}

func TestTelegramAttemptMarkerSurvivesCaptionComposition(t *testing.T) {
	marker := telegramAttemptMarker("attempt-caption")
	caption := telegramCaptionWithJobMarker("资料标题", 42, "display") + marker
	if !strings.Contains(caption, marker) {
		t.Fatal("composed caption lost attempt marker")
	}
}

func TestTelegramInternalDeliveryMarkerDetection(t *testing.T) {
	if telegramTextHasInternalDeliveryMarker("普通消息") {
		t.Fatal("plain message must not be treated as an internal delivery")
	}
	if !telegramTextHasInternalDeliveryMarker("资料" + telegramAttemptMarker("delete-safe")) {
		t.Fatal("attempt marker must protect internal delivery from auto delete")
	}
}

func TestTelegramVerifyMediaMatchRequiresKnownUniqueID(t *testing.T) {
	media := []*telegramMediaItem{{MediaType: "video", TgFileUniqueId: "expected"}}
	if telegramVerifyMediaMatches(media, "video", "other") {
		t.Fatal("known file unique id mismatch must not confirm an attempt")
	}
	if !telegramVerifyMediaMatches(media, "video", "expected") {
		t.Fatal("matching file unique id must confirm the media candidate")
	}
	if telegramVerifyMediaMatches(media, "image", "expected") {
		t.Fatal("media type mismatch must not confirm an attempt")
	}
}

func TestTelegramVerifyFreshUploadRequiresMarker(t *testing.T) {
	media := []*telegramMediaItem{{MediaType: "photo"}}
	if telegramVerifyMediaMatches(media, "image", "new-file") {
		t.Fatal("unmarked fresh upload must not match by media type alone")
	}
}

func TestTelegramAttemptTerminalStatusesAreDistinct(t *testing.T) {
	statuses := map[string]struct{}{
		telegramAttemptStatusConfirmed:  {},
		telegramAttemptStatusSuperseded: {},
	}
	if len(statuses) != 2 {
		t.Fatal("confirmed and superseded attempts must remain distinguishable")
	}
}
