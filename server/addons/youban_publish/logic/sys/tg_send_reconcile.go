package sys

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const (
	telegramSendPhaseCleanupProcessing = "cleanup_processing"
	telegramSendPhaseCleanupConfirmed  = "cleanup_confirmed"
	telegramSendPhaseDisplaySending    = "display_sending"
	telegramSendPhaseDisplayConfirmed  = "display_confirmed"
	telegramSendPhaseVerifySending     = "verify_sending"
	telegramSendPhaseVerifyConfirmed   = "verify_confirmed"
	telegramSendPhaseCompletedNoVerify = "completed_without_verify"
	// Telegram does not provide an idempotency key for send requests. An
	// ambiguous response therefore needs an asynchronous reconciliation window
	// before the job is allowed to retry.
	telegramUnknownReconcileDelay = 10 * time.Second
)

func telegramSendPhaseHasCleanup(phase string) bool {
	switch strings.TrimSpace(phase) {
	case telegramSendPhaseCleanupConfirmed,
		telegramSendPhaseDisplaySending, telegramSendPhaseDisplayConfirmed,
		telegramSendPhaseVerifySending, telegramSendPhaseVerifyConfirmed,
		telegramSendPhaseCompletedNoVerify:
		return true
	default:
		return false
	}
}

func telegramSendPhaseIsCleanup(phase string) bool {
	return strings.TrimSpace(phase) == telegramSendPhaseCleanupProcessing
}

func telegramSendPhaseHasDisplay(phase string) bool {
	switch strings.TrimSpace(phase) {
	case telegramSendPhaseDisplayConfirmed, telegramSendPhaseVerifySending,
		telegramSendPhaseVerifyConfirmed, telegramSendPhaseCompletedNoVerify:
		return true
	default:
		return false
	}
}

func telegramCaptionWithJobMarker(caption string, jobId int64, purpose string) string {
	return caption + telegramJobPhaseMarker(jobId, purpose)
}

func telegramJobPhaseMarker(jobId int64, purpose string) string {
	sum := sha256.Sum256([]byte(strconv.FormatInt(jobId, 10) + ":" + strings.TrimSpace(purpose)))
	var marker strings.Builder
	marker.WriteRune('\u2063')
	for _, value := range sum[:8] {
		for bit := 7; bit >= 0; bit-- {
			if value&(1<<bit) == 0 {
				marker.WriteRune('\u200b')
			} else {
				marker.WriteRune('\u200c')
			}
		}
	}
	marker.WriteRune('\u2064')
	return marker.String()
}

// telegramMediaPreparationError identifies failures before a Telegram request.
// Unwrap preserves cancellation and existing retry classifications.
type telegramMediaPreparationError struct{ cause error }

func (e *telegramMediaPreparationError) Error() string { return e.cause.Error() }
func (e *telegramMediaPreparationError) Unwrap() error { return e.cause }

func telegramPreparationError(err error) error {
	if err == nil {
		return nil
	}
	return &telegramMediaPreparationError{cause: err}
}

func isTelegramAmbiguousDeliveryError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errTelegramDeliveryUncertain) {
		return true
	}
	var preparationErr *telegramMediaPreparationError
	if errors.As(err, &preparationErr) {
		return false
	}
	message := strings.ToLower(err.Error())
	// Media preparation/download failures happen before Telegram accepts a
	// request, so delivery is known to have failed and must be retried normally.
	for _, part := range []string{"下载远程媒体失败", "保存媒体缓存临时文件失败", "创建媒体缓存临时文件失败", "写入媒体缓存文件失败"} {
		if strings.Contains(message, strings.ToLower(part)) {
			return false
		}
	}
	for _, part := range []string{
		"context deadline exceeded", "client.timeout exceeded", "closed pipe", "broken pipe", "goaway",
		"cannot rewind body", "connection reset", "unexpected eof", "error decode response body for method",
		"发送进程中断", "等待webhook或协议号对账",
	} {
		if strings.Contains(message, part) {
			return true
		}
	}
	return false
}

func telegramDeliveryUncertainError(err error) error {
	if err == nil {
		return errTelegramDeliveryUncertain
	}
	return fmt.Errorf("%w：%v", errTelegramDeliveryUncertain, err)
}

func (s *sSysPublish) updateTelegramJobSendPhase(ctx context.Context, jobId int64, phase string) error {
	return s.updateTelegramJobSendPhaseFromStatuses(ctx, jobId, phase, telegramJobStatusSending)
}

func (s *sSysPublish) updateTelegramJobSendPhaseFromStatuses(ctx context.Context, jobId int64, phase string, statuses ...string) error {
	if len(statuses) == 0 {
		return gerror.New("TG任务发送阶段来源状态不能为空")
	}
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).WhereIn("status", statuses).
		Data(g.Map{"send_phase": phase, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新TG发送阶段失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return gerror.New("TG任务状态已变化，停止更新发送阶段")
	}
	if projectionErr := s.projectTelegramDeliverySteps(ctx, jobId, phase); projectionErr != nil {
		g.Log().Warningf(ctx, "投影TG发送步骤失败 jobId:%d phase:%s err:%+v", jobId, phase, projectionErr)
	}
	return nil
}

func (s *sSysPublish) markTelegramJobUnknown(ctx context.Context, job telegramJobRecord, cause error) error {
	causeMessage := "未知原因"
	if cause != nil {
		causeMessage = cause.Error()
	}
	message := "Telegram 返回结果不确定，已暂停重发并等待频道消息对账：" + causeMessage
	transitioned, err := s.transitionTelegramJob(ctx, telegramJobTransition{
		JobId:        job.Id,
		From:         []string{telegramJobStatusSending},
		To:           telegramJobStatusConfirming,
		RetryDelay:   telegramUnknownReconcileDelay,
		ErrorMessage: message,
		Additional:   g.Map{"reconcile_count": 0},
	})
	if err != nil {
		return gerror.Wrap(err, "标记TG任务待对账失败")
	}
	if !transitioned {
		return nil
	}
	s.appendTelegramJobLog(ctx, job, "publish", "unknown", message)
	return nil
}

func (s *sSysPublish) reconcileUnknownTelegramJob(ctx context.Context, job telegramJobRecord) error {
	claimed, err := s.claimUnknownTelegramJob(ctx, job.Id)
	if err != nil || !claimed {
		return err
	}
	job, err = s.telegramJobById(ctx, job.Id)
	if err != nil {
		return s.releaseUnknownTelegramJobClaim(ctx, job.Id, err)
	}
	handled, recoverErr := s.recoverConfirmedUnknownTelegramJob(ctx, job)
	if recoverErr != nil {
		if errors.Is(recoverErr, errTelegramAttemptAuditIncomplete) {
			return s.reconcileIncompleteUnknownTelegramAttempt(ctx, job, recoverErr)
		}
		return s.releaseUnknownTelegramJobClaim(ctx, job.Id, recoverErr)
	}
	if handled {
		return nil
	}
	handled, recoverErr = s.recoverUnconfirmedUnknownTelegramJob(ctx, job)
	if recoverErr != nil {
		return s.releaseUnknownTelegramJobClaim(ctx, job.Id, recoverErr)
	}
	if handled {
		return nil
	}
	// A timeout is an ambiguous Telegram result: the request may have been
	// accepted even though the HTTP response was lost. Never turn this state
	// back into failed_retry, because doing so can send the same media twice.
	dispatchStatus := tgDispatchStatusProcessing
	if telegramSendPhaseHasDisplay(job.SendPhase) {
		dispatchStatus = tgDispatchStatusIdle
	}
	message := "Telegram发送结果未确认，已停止自动重发；等待Webhook确认"
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Where("status", "unknown").Data(g.Map{
		"dispatch_status": dispatchStatus,
		"next_retry_at":   nil, "error_message": message, "updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "迁移历史TG待对账任务失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	observeTelegramReconcile(ctx, "awaiting_confirmation")
	s.appendTelegramJobLog(ctx, job, "reconcile", "unknown", message)
	return nil
}

const (
	telegramUnknownAttemptReconcile = "reconcile"
	telegramUnknownAttemptRetry     = "retry"
)

func telegramUnknownAttemptRecoveryAction(attempt telegramDeliveryAttempt, auditCount int) string {
	if attempt.Id <= 0 {
		return telegramUnknownAttemptRetry
	}
	if attempt.Status != telegramAttemptStatusRetryWait {
		return telegramUnknownAttemptReconcile
	}
	if auditCount > 0 || isTelegramAmbiguousDeliveryError(errors.New(attempt.ErrorMessage)) {
		return telegramUnknownAttemptReconcile
	}
	return telegramUnknownAttemptRetry
}

func (s *sSysPublish) unknownTelegramJobAttempt(ctx context.Context, job telegramJobRecord) (telegramDeliveryAttempt, int, error) {
	phase := "display"
	if telegramSendPhaseHasDisplay(job.SendPhase) {
		phase = "verify"
	}
	attempt, err := s.activeTelegramDeliveryAttempt(ctx, job.Id, phase)
	if err != nil || attempt.Id <= 0 {
		return attempt, 0, err
	}
	count, err := g.DB().Model(publishTgAttemptMessageTable).Safe().Ctx(ctx).
		Where("attempt_id", attempt.Id).Count()
	if err != nil {
		return attempt, 0, gerror.Wrap(err, "读取TG Attempt消息审计数量失败")
	}
	return attempt, count, nil
}

func (s *sSysPublish) latestUnknownTelegramJobAttempt(ctx context.Context, job telegramJobRecord) (telegramDeliveryAttempt, error) {
	phase := "display"
	if telegramSendPhaseHasDisplay(job.SendPhase) {
		phase = "verify"
	}
	var attempt telegramDeliveryAttempt
	err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Where("job_id", job.Id).Where("phase", phase).Where("chunk_index", 0).
		OrderDesc("id").Limit(1).Scan(&attempt)
	if telegramOptionalAttemptScanSucceeded(err) {
		return attempt, nil
	}
	return attempt, gerror.Wrap(err, "读取TG最新发送Attempt失败")
}

// recoverUnconfirmedUnknownTelegramJob distinguishes failures known to have
// happened before Telegram accepted a request from ambiguous remote results.
// Only the former may be retried automatically; every ambiguous result must be
// reconciled by the bound account first to prevent duplicate media.
func (s *sSysPublish) recoverUnconfirmedUnknownTelegramJob(ctx context.Context, job telegramJobRecord) (bool, error) {
	attempt, auditCount, err := s.unknownTelegramJobAttempt(ctx, job)
	if err != nil {
		return false, err
	}
	switch telegramUnknownAttemptRecoveryAction(attempt, auditCount) {
	case telegramUnknownAttemptReconcile:
		if err = s.enqueueTelegramAttemptReconcile(ctx, job, attempt); err != nil {
			return true, s.failTelegramMediaFallbackJob(ctx, job,
				gerror.Wrap(err, "TG发送结果不确定且无法提交协议号对账"))
		}
		s.appendTelegramJobLog(ctx, job, "reconcile", "queued", "TG发送结果不确定，已提交协议号对账，禁止直接重发")
		return true, nil

	case telegramUnknownAttemptRetry:
		phase := telegramSendPhaseCleanupConfirmed
		if attempt.Phase == "verify" {
			phase = telegramSendPhaseDisplayConfirmed
		}
		if attempt.Id <= 0 && job.SendPhase == telegramSendPhaseVerifySending {
			phase = telegramSendPhaseDisplayConfirmed
		}
		if attempt.Id > 0 {
			result, updateErr := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
				Where("id", attempt.Id).Where("status", telegramAttemptStatusRetryWait).
				Data(g.Map{"status": telegramAttemptStatusSuperseded, "webhook_deadline": nil, "updated_at": gtime.Now()}).Update()
			if updateErr != nil {
				return true, gerror.Wrap(updateErr, "关闭TG调用前失败Attempt失败")
			}
			affected, _ := result.RowsAffected()
			if affected == 0 {
				return true, nil
			}
		}
		transitioned, transitionErr := s.transitionTelegramJob(ctx, telegramJobTransition{
			JobId: job.Id, From: []string{telegramJobStatusConfirming}, To: telegramJobStatusRetryWait,
			Additional: g.Map{
				"send_phase": phase, "error_message": "", "last_dispatch_error": "",
			},
		})
		if transitionErr != nil {
			return true, transitionErr
		}
		if !transitioned {
			return true, nil
		}
		s.appendTelegramJobLog(ctx, job, "reconcile", "retry", "TG请求前准备失败且无远端消息证据，已安全重新排队")
		return true, s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, 0, false)
	default:
		return false, nil
	}
}

func (s *sSysPublish) reconcileIncompleteUnknownTelegramAttempt(ctx context.Context, job telegramJobRecord, cause error) error {
	attempt, err := s.latestUnknownTelegramJobAttempt(ctx, job)
	if err != nil {
		return s.releaseUnknownTelegramJobClaim(ctx, job.Id, err)
	}
	if attempt.Id <= 0 {
		return s.failTelegramMediaFallbackJob(ctx, job,
			gerror.Wrap(cause, "TG响应已确认但审计不完整且找不到活动Attempt"))
	}
	if err = s.enqueueTelegramAttemptReconcile(ctx, job, attempt); err != nil {
		return s.failTelegramMediaFallbackJob(ctx, job,
			gerror.Wrap(err, "TG响应审计不完整且无法提交协议号对账"))
	}
	s.appendTelegramJobLog(ctx, job, "reconcile", "queued", "TG响应审计不完整，已提交协议号补全对账，禁止重复发送")
	return nil
}

const (
	telegramUnknownRecoveryWait         = "wait"
	telegramUnknownRecoveryResumeVerify = "resume_verify"
	telegramUnknownRecoveryComplete     = "complete"
)

func telegramUnknownRecoveryAction(sendPhase string, verifyMediaCount int, displayConfirmed bool) string {
	if sendPhase == telegramSendPhaseVerifyConfirmed {
		return telegramUnknownRecoveryComplete
	}
	if sendPhase != telegramSendPhaseDisplayConfirmed && sendPhase != telegramSendPhaseCompletedNoVerify {
		return telegramUnknownRecoveryWait
	}
	if verifyMediaCount > 0 {
		if !displayConfirmed {
			return telegramUnknownRecoveryWait
		}
		return telegramUnknownRecoveryResumeVerify
	}
	return telegramUnknownRecoveryComplete
}

func telegramDisplayExpectedCount(mediaCount int) int {
	if mediaCount > 0 {
		return mediaCount
	}
	return 1
}

// telegramDisplayDeliveryConfirmed treats send_phase as a projection only.
// Verification may continue only when the display response has durable,
// complete evidence. This prevents legacy/corrupt display_confirmed jobs from
// emitting orphan verification videos after recovery.
func (s *sSysPublish) telegramDisplayDeliveryConfirmed(ctx context.Context, jobId int64, expectedCount int) (bool, error) {
	if jobId <= 0 || expectedCount <= 0 {
		return false, nil
	}
	messageCount, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Where("job_id", jobId).Where("purpose", "display").Where("status", "sent").WhereNull("deleted_at").Count()
	if err != nil {
		return false, gerror.Wrap(err, "读取TG展示消息投影失败")
	}
	if messageCount >= expectedCount {
		return true, nil
	}
	var attempts []telegramDeliveryAttempt
	if err = g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Where("job_id", jobId).Where("phase", "display").Where("status", telegramAttemptStatusConfirmed).
		Where("response_received", 1).OrderDesc("id").Scan(&attempts); err != nil {
		return false, gerror.Wrap(err, "读取TG展示确认Attempt失败")
	}
	for _, attempt := range attempts {
		if attempt.ExpectedCount < expectedCount || attempt.ConfirmedCount < expectedCount {
			continue
		}
		auditCount, countErr := g.DB().Model(publishTgAttemptMessageTable).Safe().Ctx(ctx).
			Where("attempt_id", attempt.Id).Where("source", "response").Count()
		if countErr != nil {
			return false, gerror.Wrap(countErr, "读取TG展示响应审计失败")
		}
		if auditCount >= expectedCount {
			return true, nil
		}
	}
	return false, nil
}

// recoverConfirmedUnknownTelegramJob repairs local state strictly from a
// confirmed Telegram response. It never sends media to Telegram. Once the
// display projection is restored, verification resumes through the normal
// ordered queue so an old uncertain job cannot be sent twice.
func (s *sSysPublish) recoverConfirmedUnknownTelegramJob(ctx context.Context, job telegramJobRecord) (bool, error) {
	displayMedia, err := s.telegramJobMedia(ctx, job, "display")
	if err != nil {
		return false, err
	}
	displayMedia, err = s.selectTelegramDisplayMediaForTenant(ctx, job, displayMedia)
	if err != nil {
		return false, err
	}
	verifyMedia, err := s.telegramJobMedia(ctx, job, "verify")
	if err != nil {
		return false, err
	}

	if !telegramSendPhaseHasDisplay(job.SendPhase) {
		restored, restoreErr := s.restoreConfirmedTelegramResponse(ctx, job, "display", displayMedia)
		if restoreErr != nil {
			return false, restoreErr
		}
		if !restored {
			return false, nil
		}
		job, err = s.telegramJobById(ctx, job.Id)
		if err != nil {
			return false, err
		}
	}

	if len(verifyMedia) > 0 && job.SendPhase != telegramSendPhaseVerifyConfirmed {
		restored, restoreErr := s.restoreConfirmedTelegramResponse(ctx, job, "verify", verifyMedia)
		if restoreErr != nil {
			return false, restoreErr
		}
		if restored {
			job, err = s.telegramJobById(ctx, job.Id)
			if err != nil {
				return false, err
			}
		}
	}

	displayConfirmed, evidenceErr := s.telegramDisplayDeliveryConfirmed(ctx, job.Id, telegramDisplayExpectedCount(len(displayMedia)))
	if evidenceErr != nil {
		return false, evidenceErr
	}
	action := telegramUnknownRecoveryAction(job.SendPhase, len(verifyMedia), displayConfirmed)
	switch action {
	case telegramUnknownRecoveryResumeVerify:
		transitioned, transitionErr := s.transitionTelegramJob(ctx, telegramJobTransition{
			JobId: job.Id, From: []string{telegramJobStatusConfirming}, To: telegramJobStatusRetryWait,
			Additional: g.Map{
				"send_phase":          telegramSendPhaseDisplayConfirmed,
				"error_message":       "",
				"last_dispatch_error": "",
			},
		})
		if transitionErr != nil {
			return false, transitionErr
		}
		if !transitioned {
			return true, nil
		}
		s.appendTelegramJobLog(ctx, job, "reconcile", "confirmed", "TG展示资料已确认，继续按频道顺序推送验证资料")
		return true, s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, 0, false)

	case telegramUnknownRecoveryComplete:
		phase := job.SendPhase
		if len(verifyMedia) == 0 {
			phase = telegramSendPhaseCompletedNoVerify
			if err = s.updateTelegramJobSendPhaseFromStatuses(ctx, job.Id, phase, telegramJobStatusConfirming); err != nil {
				return false, err
			}
		}
		transitioned, transitionErr := s.transitionTelegramJob(ctx, telegramJobTransition{
			JobId: job.Id, From: []string{telegramJobStatusConfirming}, To: telegramJobStatusSending,
			Additional: g.Map{
				"send_phase":          phase,
				"error_message":       "",
				"last_dispatch_error": "",
			},
		})
		if transitionErr != nil {
			return false, transitionErr
		}
		if !transitioned {
			return true, nil
		}
		job.Status = telegramJobStatusSending
		job.SendPhase = phase
		s.appendTelegramJobLog(ctx, job, "reconcile", "confirmed", "TG远端发送结果已确认，完成本地任务投影")
		return true, s.completeTelegramJob(ctx, job)
	default:
		return false, nil
	}
}

func (s *sSysPublish) claimUnknownTelegramJob(ctx context.Context, jobId int64) (bool, error) {
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).Where("status", "unknown").
		WhereIn("dispatch_status", []string{tgDispatchStatusQueued, tgDispatchStatusProcessing}).
		Data(g.Map{"dispatch_status": tgDispatchStatusProcessing, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return false, gerror.Wrap(err, "领取TG对账任务失败")
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (s *sSysPublish) releaseUnknownTelegramJobClaim(ctx context.Context, jobId int64, cause error) error {
	message := "释放TG对账任务失败"
	if cause != nil {
		message = "TG对账任务读取失败，等待重试：" + cause.Error()
	}
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).Where("status", "unknown").
		Data(g.Map{"dispatch_status": tgDispatchStatusIdle, "next_retry_at": gtime.Now().Add(time.Minute), "error_message": message, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "释放TG对账任务领取失败")
	}
	return nil
}
