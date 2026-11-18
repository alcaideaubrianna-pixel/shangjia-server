package sys

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/utility/dbtime"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const telegramFallbackAccountTaskTable = "hg_tg_collector_account_task"

type telegramFallbackTaskContextKey struct{}

type telegramFallbackLaneBusyError struct{}

func (telegramFallbackLaneBusyError) Error() string {
	return "频道发送槽忙，协议号交接等待重试"
}
func (telegramFallbackLaneBusyError) AccountTaskRetryDelay() time.Duration {
	return telegramStreamExecutionTTL
}

func telegramFallbackTaskKey(job telegramJobRecord, purpose string) string {
	digest := sha256.Sum256([]byte(job.OperationNo))
	return fmt.Sprintf("message-media-fallback:%d:%s:%x:%d", job.Id, purpose, digest[:12], job.ReconcileCount)
}

func telegramFallbackTaskIdentity(job telegramJobRecord, purpose string, task *collectorin.AccountTask) bool {
	return task != nil && task.ID > 0 && task.TenantID == job.TenantId &&
		task.TaskType == collectorin.AccountTaskTypeMessageMediaFallback &&
		task.TaskKey == telegramFallbackTaskKey(job, purpose)
}

func telegramFallbackOwnsPhase(job telegramJobRecord, purpose string) bool {
	return (purpose == "display" || purpose == "verify") &&
		job.LastDispatchError == telegramMediaFallbackReason(purpose) &&
		(job.Status == telegramJobStatusRetryWait || job.Status == telegramJobStatusSending ||
			job.Status == telegramJobStatusConfirming)
}

func logTelegramFallbackHandoff(ctx context.Context, event string, job telegramJobRecord, purpose string, taskID int64, reason string, from, to JobStatus) {
	outcome, errorClass := "unknown", "none"
	metricEvent := event
	if event == "requested" {
		metricEvent = "queued"
	}
	if event == "completed" || event == "recovered" {
		if strings.Contains(reason, "missing_or_terminal_account_task") {
			outcome, errorClass = "unknown", "unknown"
		} else if to == telegramJobStatusFailed {
			outcome, errorClass = "failed", "other"
		} else {
			outcome = "success"
		}
	}
	observePublishTransportFallback(ctx, metricEvent, purpose, "account", "not_applicable", outcome, errorClass)
	g.Log().Info(ctx, "TG协议号阶段交接", g.Map{
		"event": "telegram_handoff_" + event, "jobId": job.Id, "profileId": job.ProfileId,
		"channelId": job.ChannelId, "targetChat": job.TargetChatId, "purpose": purpose,
		"transport": "protocol", "taskId": taskID, "operation": job.OperationNo,
		"reason": reason, "from": from, "to": to, "generation": job.ReconcileCount,
	})
}

func logTelegramProtocolSend(ctx context.Context, job telegramJobRecord, attempt telegramDeliveryAttempt, purpose string, mediaCount int, messages []*telegramSentMessage, node string, duration time.Duration, err error) {
	taskID := int64(0)
	if task, _ := ctx.Value(telegramFallbackTaskContextKey{}).(*collectorin.AccountTask); task != nil {
		taskID = task.ID
	}
	verdict := "started"
	if node == "result" {
		confirmed := mediaCount > 0 && len(telegramSentMessageIDs(messages)) == mediaCount
		publishTransportMetrics().accountPhase(ctx, purpose, duration, err, confirmed)
		verdict = "success"
		if !confirmed {
			verdict = "unknown"
		}
		if err != nil {
			verdict = "failed"
			if !telegramFallbackBotFailureKnown(err) {
				verdict = "unknown"
			}
		}
	}
	g.Log().Info(ctx, "TG协议号实际发送", g.Map{
		"event": "telegram_protocol_send", "node": node, "verdict": verdict,
		"jobId": job.Id, "profileId": job.ProfileId, "channelId": job.ChannelId,
		"targetChat": job.TargetChatId, "operation": job.OperationNo,
		"taskId": taskID, "attemptId": attempt.Id, "purpose": purpose,
		"transport": "protocol", "mediaCount": mediaCount,
		"messageIds": telegramSentMessageIDs(messages), "durationMs": duration.Milliseconds(),
		"error": telegramPublishLogError(err),
	})
}

// requestTelegramMediaFallback is the producer and recovery entry point. The
// caller must have classified the Bot result: ambiguous Attempts stay waiting
// for facts and are never converted into permission to send again.
func (s *sSysPublish) requestTelegramMediaFallback(ctx context.Context, job telegramJobRecord, accountID int64, purpose string) error {
	ctx, cancel := telegramFinalizeContext(ctx, "account_fallback.request")
	defer cancel()
	if job.Id <= 0 || accountID <= 0 || (purpose != "display" && purpose != "verify") {
		return gerror.New("协议号媒体降级任务参数不完整")
	}
	observePublishTransportFallback(ctx, "requested", purpose, "account", "not_applicable", "unknown", "none")
	available, err := telegramProtocolAccountCanPublish(ctx, job.TenantId, accountID)
	if err != nil {
		return err
	}
	if !available {
		return errTelegramMediaFallbackUnavailable
	}
	err = telegramFallbackTransaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockProfilePublishFenceTx(ctx, tx, job); err != nil {
			return err
		}
		var current telegramJobRecord
		if err := tx.Model(publishTgJobTable).Ctx(ctx).Where("id", job.Id).LockUpdate().Scan(&current); err != nil {
			return err
		}
		if current.Id <= 0 || current.OperationNo != job.OperationNo || current.TenantId != job.TenantId {
			return errTelegramJobSuperseded
		}
		if err := checkTelegramFallbackProfileOperationTx(ctx, tx, current); err != nil {
			return err
		}
		if telegramFallbackOwnsPhase(current, purpose) {
			job = current
			return nil
		}
		// The producer must first abandon a definitely rejected Bot Attempt.
		// Retire only that proven failure; an unresolved Bot token must never
		// be reused by Account delivery.
		if err := retireTelegramFallbackRejectedAttemptTx(ctx, tx, current, purpose); err != nil {
			return err
		}
		handoff := telegramMediaFallbackTransition(current, purpose)
		handoff.Additional["reconcile_count"] = current.ReconcileCount + 1
		if purpose == "display" {
			handoff.SendPhase = jobPhase(SendPhaseCleanupConfirmed)
		}
		changed, err := s.transitionTelegramJobTx(ctx, tx, handoff)
		if err != nil {
			return err
		}
		if !changed {
			return errTelegramJobSuperseded
		}
		job = current
		job.ReconcileCount++
		return nil
	})
	if err != nil {
		logTelegramFallbackHandoff(ctx, "rejected", job, purpose, 0, "handoff_cas_rejected", job.Status, job.Status)
		return err
	}
	taskID, err := collectorservice.AccountTasks().Submit(ctx, &collectorin.AccountTaskSubmit{
		TenantID: job.TenantId, AccountID: accountID,
		TaskType: collectorin.AccountTaskTypeMessageMediaFallback,
		TaskKey:  telegramFallbackTaskKey(job, purpose), Priority: collectorin.EventPriorityUrgent, MaxAttempts: 5,
	})
	if err != nil {
		observePublishTransportFallback(ctx, "submit_delayed", purpose, "account", "not_applicable", "unknown", "other")
		return gerror.Wrap(errTelegramMediaFallbackSubmitDelayed, "协议号交接已持久化，等待恢复器补投任务")
	}
	logTelegramFallbackHandoff(ctx, "requested", job, purpose, taskID, "durable_task_submitted", job.Status, telegramJobStatusRetryWait)
	return nil
}

func telegramFallbackAttemptCanRetire(attempt telegramDeliveryAttempt, auditCount int) bool {
	return attempt.Status == telegramAttemptStatusRetryWait && attempt.ResponseReceived == 0 &&
		attempt.ConfirmedCount == 0 && auditCount == 0 && attempt.WebhookDeadline == nil &&
		strings.TrimSpace(attempt.ErrorMessage) != "" &&
		!strings.Contains(strings.ToLower(attempt.ErrorMessage), "context canceled") &&
		!isTelegramAPITimeout(gerror.New(attempt.ErrorMessage)) &&
		!isTelegramAmbiguousDeliveryError(gerror.New(attempt.ErrorMessage))
}

func telegramFallbackBotFailureKnown(err error) bool {
	if err == nil {
		return false
	}
	// The typed preparation error is authoritative even if its underlying
	// download failed with a deadline: no Telegram send was started.
	var preparation *telegramMediaPreparationError
	if errors.As(err, &preparation) {
		return true
	}
	return !errors.Is(err, context.Canceled) && !isTelegramAPITimeout(err) &&
		!isTelegramAmbiguousDeliveryError(err)
}

// recordTelegramFallbackBotRejection closes the producer contract itself.
// A successful return proves this exact Bot token can be retired; callers do
// not rely on the best-effort abandon helper that normally runs after submit.
func (s *sSysPublish) recordTelegramFallbackBotRejection(ctx context.Context, job telegramJobRecord, attemptID int64, purpose string, cause error) error {
	if attemptID <= 0 || !telegramFallbackBotFailureKnown(cause) {
		return errTelegramAttemptNeedsReconcile
	}
	ctx, cancel := telegramFinalizeContext(ctx, "account_fallback.bot_rejection")
	defer cancel()
	return telegramFallbackTransaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockProfilePublishFenceTx(ctx, tx, job); err != nil {
			return err
		}
		var current telegramJobRecord
		if err := tx.Model(publishTgJobTable).Ctx(ctx).Where("id", job.Id).LockUpdate().Scan(&current); err != nil {
			return err
		}
		_, expectedPhase, _, phaseErr := telegramAttemptPhaseTransition(purpose)
		if phaseErr != nil {
			return phaseErr
		}
		if current.Id <= 0 || current.OperationNo != job.OperationNo || current.TenantId != job.TenantId ||
			current.Status != telegramJobStatusSending || current.SendPhase.String() != expectedPhase ||
			telegramMediaFallbackJob(current.LastDispatchError) {
			return errTelegramJobSuperseded
		}
		return recordTelegramFallbackRejectionTx(ctx, tx, current, attemptID, purpose, cause, "bot")
	})
}

func recordTelegramFallbackRejectionTx(ctx context.Context, tx gdb.TX, job telegramJobRecord, attemptID int64, purpose string, cause error, transport string) error {
	var attempt telegramDeliveryAttempt
	if err := tx.Model(publishTgAttemptTable).Ctx(ctx).Where("id", attemptID).LockUpdate().Scan(&attempt); err != nil {
		return err
	}
	if attempt.JobId != job.Id || attempt.TenantId != job.TenantId || attempt.BotId != job.BotId ||
		attempt.Phase != purpose || attempt.ResponseReceived != 0 || attempt.ConfirmedCount != 0 ||
		attempt.WebhookDeadline != nil ||
		(attempt.Status != telegramAttemptStatusSending && attempt.Status != telegramAttemptStatusRetryWait) {
		return errTelegramAttemptNeedsReconcile
	}
	audited, err := tx.Model(publishTgAttemptMessageTable).Ctx(ctx).Where("attempt_id", attemptID).Count()
	if err != nil {
		return err
	}
	if audited > 0 {
		return errTelegramAttemptNeedsReconcile
	}
	// Store a canonical fact, not a reconstructed error string. A proven
	// preparation failure may contain "deadline exceeded"; interpreting that
	// string later as an ambiguous Telegram send would lose its provenance.
	proof := transport + "_definite_rejection"
	var preparation *telegramMediaPreparationError
	if errors.As(cause, &preparation) {
		proof = transport + "_preparation_failed_before_send"
	}
	result, err := tx.Model(publishTgAttemptTable).Ctx(ctx).Where("id", attemptID).
		WhereIn("status", []string{telegramAttemptStatusSending, telegramAttemptStatusRetryWait}).
		Data(g.Map{"status": telegramAttemptStatusRetryWait, "error_message": proof}).Update()
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errTelegramJobSuperseded
	}
	return nil
}

func retireTelegramFallbackRejectedAttemptTx(ctx context.Context, tx gdb.TX, job telegramJobRecord, purpose string) error {
	var attempt telegramDeliveryAttempt
	if err := tx.Model(publishTgAttemptTable).Ctx(ctx).Where("job_id", job.Id).Where("phase", purpose).
		WhereIn("status", []string{telegramAttemptStatusSending, telegramAttemptStatusWaiting, telegramAttemptStatusRetryWait}).
		OrderDesc("id").Limit(1).LockUpdate().Scan(&attempt); !telegramOptionalAttemptScanSucceeded(err) {
		return err
	}
	if attempt.Id <= 0 {
		return nil
	}
	auditCount, err := tx.Model(publishTgAttemptMessageTable).Ctx(ctx).Where("attempt_id", attempt.Id).Count()
	if err != nil {
		return err
	}
	if !telegramFallbackAttemptCanRetire(attempt, auditCount) {
		return errTelegramAttemptNeedsReconcile
	}
	_, err = tx.Model(publishTgAttemptTable).Ctx(ctx).Where("id", attempt.Id).
		Where("status", telegramAttemptStatusRetryWait).
		Data(g.Map{"status": telegramAttemptStatusSuperseded}).Update()
	return err
}

// lockTelegramFallbackTaskTx fences both execution and terminal callbacks.
// Recovery changes the task key generation instead of reviving a terminal ID.
func lockTelegramFallbackTaskTx(ctx context.Context, tx gdb.TX, task *collectorin.AccountTask, finalizing bool) error {
	if task == nil || task.ID <= 0 {
		return errTelegramJobSuperseded
	}
	row, err := tx.Model(telegramFallbackAccountTaskTable).Ctx(ctx).Where("id", task.ID).LockUpdate().One()
	if err != nil {
		return err
	}
	if row.IsEmpty() || row["tenant_id"].Int64() != task.TenantID ||
		row["account_id"].Int64() != task.AccountID || row["task_key"].String() != task.TaskKey ||
		row["task_type"].String() != task.TaskType || row["attempt_count"].Int() != task.AttemptCount {
		return errTelegramJobSuperseded
	}
	if finalizing {
		if row["status"].String() != collectorin.AccountTaskStatusFinalizing {
			return errTelegramJobSuperseded
		}
		return nil
	}
	if row["status"].String() != collectorin.AccountTaskStatusProcessing ||
		task.LeaseOwner == "" || task.LeaseEpoch <= 0 ||
		row["lease_owner"].String() != task.LeaseOwner || row["lease_epoch"].Int64() != task.LeaseEpoch {
		return errTelegramJobSuperseded
	}
	until := dbtime.FromTimestamp(row["lease_until"].GTime())
	if until == nil || !until.Time.After(time.Now()) {
		return errTelegramJobSuperseded
	}
	return nil
}

func (s *sSysPublish) claimTelegramFallbackTask(ctx context.Context, task *collectorin.AccountTask, job telegramJobRecord, purpose string) (telegramJobRecord, error) {
	ctx, cancel := telegramFinalizeContext(ctx, "account_fallback.claim")
	defer cancel()
	err := telegramFallbackTransaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockProfilePublishFenceTx(ctx, tx, job); err != nil {
			return err
		}
		if err := lockTelegramFallbackTaskTx(ctx, tx, task, false); err != nil {
			return err
		}
		var current telegramJobRecord
		if err := tx.Model(publishTgJobTable).Ctx(ctx).Where("id", job.Id).LockUpdate().Scan(&current); err != nil {
			return err
		}
		if !telegramFallbackTaskIdentity(current, purpose, task) || !telegramFallbackOwnsPhase(current, purpose) {
			return errTelegramJobSuperseded
		}
		if err := checkTelegramFallbackProfileOperationTx(ctx, tx, current); err != nil {
			return err
		}
		// Unknown means remote delivery is unresolved, even if no message has
		// reached this process yet. Only reconciliation can resolve it.
		if current.Status == telegramJobStatusConfirming {
			return errTelegramAttemptNeedsReconcile
		}
		if current.Status == telegramJobStatusRetryWait {
			var resumePhase *SendPhase
			if purpose == "display" && current.SendPhase == SendPhaseEmpty {
				// Legacy producer cleared this phase despite cleanup having
				// finished before the original display send.
				resumePhase = jobPhase(SendPhaseCleanupConfirmed)
				current.SendPhase = SendPhaseCleanupConfirmed
			}
			changed, err := s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{
				JobId: current.Id, From: []JobStatus{telegramJobStatusRetryWait}, To: telegramJobStatusSending,
				NotExpiredAt: gtime.Now(),
				SendPhase:    resumePhase,
				Additional:   g.Map{"next_retry_at": gtime.Now().Add(telegramMediaFallbackRecoverAfter)},
			})
			if err != nil {
				return err
			}
			if !changed {
				return errTelegramJobSuperseded
			}
			current.Status = telegramJobStatusSending
		}
		job = current
		return nil
	})
	event, reason := "claimed", "account_lease_verified"
	if err != nil {
		event, reason = "rejected", "identity_lease_or_state_mismatch"
	}
	logTelegramFallbackHandoff(ctx, event, job, purpose, task.ID, reason, telegramJobStatusRetryWait, job.Status)
	return job, err
}

func (s *sSysPublish) checkTelegramFallbackExecution(ctx context.Context, job telegramJobRecord, purpose string) error {
	task, _ := ctx.Value(telegramFallbackTaskContextKey{}).(*collectorin.AccountTask)
	if task == nil {
		return errTelegramJobSuperseded
	}
	_, err := s.claimTelegramFallbackTask(ctx, task, job, purpose)
	return err
}

// resumeTelegramVerificationAfterFallback commits queue eligibility and durable
// intent together. No Telegram or Redis I/O occurs while holding these locks.
func (s *sSysPublish) resumeTelegramVerificationAfterFallback(ctx context.Context, job telegramJobRecord, task *collectorin.AccountTask) error {
	ctx, cancel := telegramFinalizeContext(ctx, "account_fallback.resume_verify")
	defer cancel()
	err := telegramFallbackTransaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockProfilePublishFenceTx(ctx, tx, job); err != nil {
			return err
		}
		return s.resumeTelegramVerificationAfterFallbackTx(ctx, tx, job, task)
	})
	if err != nil {
		return err
	}
	clearTelegramReadyStreamDedupe(ctx, job.Id)
	s.wakeTelegramOutbox()
	logTelegramFallbackHandoff(ctx, "completed", job, "display", task.ID, "verify_outbox_committed", telegramJobStatusSending, telegramJobStatusRetryWait)
	return nil
}

func (s *sSysPublish) resumeTelegramVerificationAfterFallbackTx(ctx context.Context, tx gdb.TX, job telegramJobRecord, task *collectorin.AccountTask) error {
	if err := lockProfilePublishFenceTx(ctx, tx, job); err != nil {
		return err
	}
	if err := lockTelegramFallbackTaskTx(ctx, tx, task, task != nil && task.Status == collectorin.AccountTaskStatusFinalizing); err != nil {
		return err
	}
	var current telegramJobRecord
	if err := tx.Model(publishTgJobTable).Ctx(ctx).Where("id", job.Id).LockUpdate().Scan(&current); err != nil {
		return err
	}
	if !telegramFallbackTaskIdentity(current, "display", task) || !telegramFallbackOwnsPhase(current, "display") ||
		current.SendPhase != telegramSendPhaseDisplayConfirmed {
		return errTelegramJobSuperseded
	}
	if err := checkTelegramFallbackProfileOperationTx(ctx, tx, current); err != nil {
		return err
	}
	return s.releaseTelegramFallbackDisplayTx(ctx, tx, current, task.ID)
}

func (s *sSysPublish) releaseTelegramFallbackDisplayTx(ctx context.Context, tx gdb.TX, current telegramJobRecord, taskID int64) error {
	changed, err := s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{
		JobId: current.Id, From: []JobStatus{current.Status}, To: telegramJobStatusRetryWait,
		SendPhases:     []SendPhase{SendPhaseDisplayConfirmed},
		DispatchStatus: jobDispatch(DispatchStatusIdle),
		Additional:     g.Map{"last_dispatch_error": "", "next_retry_at": nil, "error_message": ""},
	})
	if err != nil {
		return err
	}
	if !changed {
		return errTelegramJobSuperseded
	}
	// A separate durable intent cannot be lost to an older dispatcher
	// marking its original Outbox row delivered after this transaction.
	_, err = tx.Model(publishTgOutboxTable).Ctx(ctx).InsertIgnore(g.Map{
		"job_id": current.Id, "event_type": telegramOutboxEventPublish,
		"dedupe_key": fmt.Sprintf("telegram-job:%d:verify-after-account:%d:%d", current.Id, taskID, current.ReconcileCount),
		"status":     telegramOutboxPending, "available_at": gtime.Now(),
	})
	return err
}

// Late webhook confirmation can clear next_retry_at without releasing the
// Account marker. Restore only local progress from a confirmed durable phase;
// this path never creates a new external send or revives an AccountTask.
func (s *sSysPublish) recoverTelegramFallbackConfirmation(ctx context.Context, job telegramJobRecord, purpose string) (bool, error) {
	if job.SendPhase != SendPhaseDisplayConfirmed && job.SendPhase != SendPhaseVerifyConfirmed && job.SendPhase != SendPhaseCompletedNoVerify {
		return false, nil
	}
	ctx, cancel := telegramFinalizeContext(ctx, "account_fallback.recover_confirmation")
	defer cancel()
	hasVerify := job.SendPhase == SendPhaseVerifyConfirmed
	if job.SendPhase == SendPhaseDisplayConfirmed {
		verify, err := s.telegramJobMedia(ctx, job, "verify")
		if err != nil {
			return true, err
		}
		hasVerify = len(verify) > 0
	}
	resume := job.SendPhase == SendPhaseDisplayConfirmed && hasVerify
	err := telegramFallbackTransaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockProfilePublishFenceTx(ctx, tx, job); err != nil {
			return err
		}
		var current telegramJobRecord
		if err := tx.Model(publishTgJobTable).Ctx(ctx).Where("id", job.Id).LockUpdate().Scan(&current); err != nil {
			return err
		}
		if current.OperationNo != job.OperationNo || current.ReconcileCount != job.ReconcileCount ||
			current.SendPhase != job.SendPhase || !telegramFallbackOwnsPhase(current, purpose) {
			return errTelegramJobSuperseded
		}
		if err := checkTelegramFallbackProfileOperationTx(ctx, tx, current); err != nil {
			return err
		}
		if resume {
			return s.releaseTelegramFallbackDisplayTx(ctx, tx, current, 0)
		}
		if current.Status != telegramJobStatusSending {
			changed, err := s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{
				JobId: current.Id, From: []JobStatus{current.Status}, To: telegramJobStatusSending,
				SendPhases: []SendPhase{current.SendPhase},
			})
			if err != nil {
				return err
			}
			if !changed {
				return errTelegramJobSuperseded
			}
		}
		job = current
		job.Status = telegramJobStatusSending
		return nil
	})
	if err != nil {
		return true, err
	}
	if resume {
		clearTelegramReadyStreamDedupe(ctx, job.Id)
		s.wakeTelegramOutbox()
	} else if err := s.finalizeTelegramMediaDelivery(ctx, job, hasVerify, true); err != nil {
		return true, err
	}
	logTelegramFallbackHandoff(ctx, "recovered", job, purpose, 0, "confirmed_facts_restored", job.Status, telegramJobStatusRetryWait)
	return true, nil
}

func (s *sSysPublish) failTelegramFallbackTask(ctx context.Context, task *collectorin.AccountTask, job telegramJobRecord, purpose string, cause error) error {
	ctx, cancel := telegramFinalizeContext(ctx, "account_fallback.fail_task")
	defer cancel()
	message := telegramFallbackFailureMessage(cause)
	changed := false
	confirmed := false
	err := telegramFallbackTransaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockProfilePublishFenceTx(ctx, tx, job); err != nil {
			return err
		}
		if err := lockTelegramFallbackTaskTx(ctx, tx, task, true); err != nil {
			return err
		}
		var current telegramJobRecord
		if err := tx.Model(publishTgJobTable).Ctx(ctx).Where("id", job.Id).LockUpdate().Scan(&current); err != nil {
			return err
		}
		if !telegramFallbackTaskIdentity(current, purpose, task) || !telegramFallbackOwnsPhase(current, purpose) {
			return errTelegramJobSuperseded
		}
		if err := checkTelegramFallbackProfileOperationTx(ctx, tx, current); err != nil {
			return err
		}
		if accountFallbackPhaseConfirmed(current.SendPhase.String(), purpose) {
			// Facts win over a stale task failure. Recovery will resume the
			// confirmed phase; this callback must never fail it.
			confirmed = true
			job = current
			return nil
		}
		// An exhausted AccountTask does not prove a timed-out send failed.
		if err := retireTelegramFallbackRejectedAttemptTx(ctx, tx, current, purpose); err != nil {
			return err
		}
		var err error
		changed, err = s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{
			JobId: current.Id, From: []JobStatus{telegramJobStatusSending, telegramJobStatusRetryWait, telegramJobStatusConfirming},
			To: telegramJobStatusFailed, ErrorMessage: message,
			Additional: g.Map{"last_dispatch_error": ""},
		})
		job = current
		if err != nil || !changed {
			return err
		}
		// Exact operation CAS avoids resurrecting a failed projection over a
		// newer profile operation, including after this transaction commits.
		_, err = tx.Model(publishProfileStateTable).Ctx(ctx).
			Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
			Where("profile_id", job.ProfileId).Where("publish_operation_no", job.OperationNo).
			Data(g.Map{"publish_task_status": sysin.PublishTaskStatusFailed.String(), "publish_task_updated_at": gtime.Now()}).Update()
		return err
	})
	if err != nil {
		return err
	}
	if confirmed {
		if purpose == "display" {
			verify, err := s.telegramJobMedia(ctx, job, "verify")
			if err != nil {
				return err
			}
			if len(verify) == 0 {
				return s.completeConfirmedTelegramFallbackTask(ctx, task, job, purpose, false)
			}
			return s.resumeTelegramVerificationAfterFallback(ctx, job, task)
		}
		return s.completeConfirmedTelegramFallbackTask(ctx, task, job, purpose, true)
	}
	if !changed {
		return nil
	}
	logTelegramFallbackHandoff(ctx, "completed", job, purpose, task.ID, "account_task_exhausted", job.Status, telegramJobStatusFailed)
	return s.settleTelegramFallbackFailure(ctx, job, cause, message)
}

func (s *sSysPublish) completeConfirmedTelegramFallbackTask(ctx context.Context, task *collectorin.AccountTask, job telegramJobRecord, purpose string, hasVerify bool) error {
	ctx, cancel := telegramFinalizeContext(ctx, "account_fallback.complete_confirmed")
	defer cancel()
	err := telegramFallbackTransaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockProfilePublishFenceTx(ctx, tx, job); err != nil {
			return err
		}
		if err := lockTelegramFallbackTaskTx(ctx, tx, task, task != nil && task.Status == collectorin.AccountTaskStatusFinalizing); err != nil {
			return err
		}
		var current telegramJobRecord
		if err := tx.Model(publishTgJobTable).Ctx(ctx).Where("id", job.Id).LockUpdate().Scan(&current); err != nil {
			return err
		}
		if !telegramFallbackTaskIdentity(current, purpose, task) || !telegramFallbackOwnsPhase(current, purpose) {
			return errTelegramJobSuperseded
		}
		if hasVerify && current.SendPhase != SendPhaseVerifyConfirmed ||
			!hasVerify && current.SendPhase != SendPhaseDisplayConfirmed && current.SendPhase != SendPhaseCompletedNoVerify {
			return errTelegramAttemptNeedsReconcile
		}
		if err := checkTelegramFallbackProfileOperationTx(ctx, tx, current); err != nil {
			return err
		}
		if current.Status != telegramJobStatusSending {
			changed, err := s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{
				JobId: current.Id, From: []JobStatus{current.Status}, To: telegramJobStatusSending,
				SendPhases: []SendPhase{current.SendPhase},
			})
			if err != nil {
				return err
			}
			if !changed {
				return errTelegramJobSuperseded
			}
		}
		job = current
		job.Status = telegramJobStatusSending
		return nil
	})
	if err != nil {
		return err
	}
	if err := s.finalizeTelegramMediaDelivery(ctx, job, hasVerify, true); err != nil {
		return err
	}
	logTelegramFallbackHandoff(ctx, "completed", job, purpose, task.ID, "confirmed_phase_completed", job.Status, telegramJobStatusSent)
	return nil
}

func checkTelegramFallbackProfileOperationTx(ctx context.Context, tx gdb.TX, job telegramJobRecord) error {
	if job.ProfileId <= 0 || job.OperationNo == "" {
		return nil
	}
	if profilePublishCancelEnabled() && strings.HasPrefix(job.OperationNo, "full_push:") {
		// Callers already acquired the profile fence before AccountTask/Job.
		// Rechecking the current Job reenters those same transaction-held locks;
		// it must use channel-batch scope rather than the last UI operation.
		return lockProfilePublishFenceTx(ctx, tx, job)
	}
	state, err := tx.Model(publishProfileStateTable).Ctx(ctx).
		Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).Where("profile_id", job.ProfileId).
		Fields("publish_operation_no,publish_task_status").One()
	if err != nil {
		return err
	}
	if !state.IsEmpty() && (state["publish_task_status"].String() == "canceling" || state["publish_task_status"].String() == "canceled" || (state["publish_operation_no"].String() != "" && state["publish_operation_no"].String() != job.OperationNo)) {
		return errTelegramJobSuperseded
	}
	return nil
}

func telegramFallbackMarkerSQL() string {
	// NULL/empty legacy values are ordinary Bot jobs.
	return "(last_dispatch_error IS NULL OR last_dispatch_error NOT LIKE ?)"
}

func telegramFallbackTaskKeyIsCurrent(job telegramJobRecord, purpose, key string) bool {
	return strings.TrimSpace(key) == telegramFallbackTaskKey(job, purpose)
}

// failOwnedTelegramFallbackJob is the non-callback terminal path (for example a
// removed channel account). It cannot terminate an active AccountTask or adopt
// the ownership of a newer recovery generation.
func (s *sSysPublish) failOwnedTelegramFallbackJob(ctx context.Context, job telegramJobRecord, cause error) error {
	ctx, cancel := telegramFinalizeContext(ctx, "account_fallback.fail_owned")
	defer cancel()
	purpose := telegramMediaFallbackPurpose(job.LastDispatchError)
	if !telegramFallbackOwnsPhase(job, purpose) {
		return nil
	}
	changed := false
	message := telegramFallbackFailureMessage(cause)
	err := telegramFallbackTransaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockProfilePublishFenceTx(ctx, tx, job); err != nil {
			return err
		}
		// Keep the same AccountTask -> Job lock order as execution/callbacks.
		task, err := tx.Model(telegramFallbackAccountTaskTable).Ctx(ctx).
			Where("tenant_id", job.TenantId).Where("task_key", telegramFallbackTaskKey(job, purpose)).
			LockUpdate().One()
		if err != nil {
			return err
		}
		if !task.IsEmpty() && telegramMediaFallbackTaskStatusActive(task["status"].String()) {
			return errTelegramJobSuperseded
		}
		var current telegramJobRecord
		if err := tx.Model(publishTgJobTable).Ctx(ctx).Where("id", job.Id).LockUpdate().Scan(&current); err != nil {
			return err
		}
		if current.OperationNo != job.OperationNo || current.ReconcileCount != job.ReconcileCount ||
			!telegramFallbackOwnsPhase(current, purpose) || accountFallbackPhaseConfirmed(current.SendPhase.String(), purpose) {
			return errTelegramJobSuperseded
		}
		if err := checkTelegramFallbackProfileOperationTx(ctx, tx, current); err != nil {
			return err
		}
		if err := retireTelegramFallbackRejectedAttemptTx(ctx, tx, current, purpose); err != nil {
			return err
		}
		changed, err = s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{
			JobId: job.Id, From: []JobStatus{current.Status}, To: telegramJobStatusFailed,
			ErrorMessage: message, Additional: g.Map{"last_dispatch_error": ""},
		})
		if err != nil || !changed {
			return err
		}
		_, err = tx.Model(publishProfileStateTable).Ctx(ctx).
			Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
			Where("profile_id", job.ProfileId).Where("publish_operation_no", job.OperationNo).
			Data(g.Map{"publish_task_status": sysin.PublishTaskStatusFailed.String(), "publish_task_updated_at": gtime.Now()}).Update()
		return err
	})
	if err != nil || !changed {
		return err
	}
	logTelegramFallbackHandoff(ctx, "completed", job, purpose, 0, "account_unavailable", job.Status, telegramJobStatusFailed)
	return s.settleTelegramFallbackFailure(ctx, job, cause, message)
}

func telegramFallbackFailureMessage(cause error) string {
	message := "协议号媒体发送已达最大重试次数"
	if errors.Is(cause, errTelegramMediaFallbackUnavailable) {
		message = "协议号媒体降级不可用"
	}
	if cause != nil {
		message += "：" + telegramPublishLogError(cause)
	}
	return message
}

// Reconciliation also uses the historical failure entry point for ordinary
// Bot unknown/audit failures. A missing Account marker must not turn those
// terminal transitions into silent success.
func (s *sSysPublish) failNonAccountTelegramJob(ctx context.Context, job telegramJobRecord, cause error) error {
	ctx, cancel := telegramFinalizeContext(ctx, "job.fail_non_account")
	defer cancel()
	message := telegramFallbackFailureMessage(cause)
	changed := false
	err := telegramFallbackTransaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockProfilePublishFenceTx(ctx, tx, job); err != nil {
			return err
		}
		var err error
		changed, err = s.failNonAccountTelegramJobTx(ctx, tx, job, message)
		return err
	})
	if err != nil || !changed {
		return err
	}
	return s.settleTelegramFallbackFailure(ctx, job, cause, message)
}

func (s *sSysPublish) failNonAccountTelegramJobTx(ctx context.Context, tx gdb.TX, job telegramJobRecord, message string) (bool, error) {
	if err := lockProfilePublishFenceTx(ctx, tx, job); err != nil {
		return false, err
	}
	if job.Id <= 0 || (job.Status != telegramJobStatusSending && job.Status != telegramJobStatusConfirming) {
		return false, nil
	}
	var current telegramJobRecord
	if err := tx.Model(publishTgJobTable).Ctx(ctx).Where("id", job.Id).LockUpdate().Scan(&current); err != nil {
		if telegramOptionalAttemptScanSucceeded(err) {
			return false, nil
		}
		return false, err
	}
	if current.Id <= 0 || current.TenantId != job.TenantId || current.AccountId != job.AccountId ||
		current.ProfileId != job.ProfileId || current.OperationNo != job.OperationNo ||
		current.Status != job.Status || current.SendPhase != job.SendPhase ||
		telegramMediaFallbackJob(current.LastDispatchError) {
		return false, nil
	}
	if err := checkTelegramFallbackProfileOperationTx(ctx, tx, current); err != nil {
		if errors.Is(err, errTelegramJobSuperseded) {
			return false, nil
		}
		return false, err
	}
	transition := telegramJobTransition{
		JobId: job.Id, From: []JobStatus{job.Status}, To: telegramJobStatusFailed,
		SendPhases: []SendPhase{job.SendPhase}, ErrorMessage: message,
		Additional: g.Map{"last_dispatch_error": message},
	}
	from, data, err := telegramJobTransitionUpdate(transition)
	if err != nil {
		return false, err
	}
	result, err := transitionTelegramJobModel(tx.Model(publishTgJobTable).Ctx(ctx).
		Where("id", job.Id).Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
		Where("operation_no", job.OperationNo).WhereIn("status", from).
		Where(telegramFallbackMarkerSQL(), telegramMediaFallbackMarker+"%"), transition, data)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return false, err
	}
	_, err = tx.Model(publishProfileStateTable).Ctx(ctx).
		Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).Where("publish_operation_no", job.OperationNo).
		Data(g.Map{"publish_task_status": sysin.PublishTaskStatusFailed.String(), "publish_task_updated_at": gtime.Now()}).Update()
	return err == nil, err
}

// Run compensations only after an owned Job/operation failure CAS committed.
// Classify using the original error; persist and log only its redacted form.
func (s *sSysPublish) settleTelegramFallbackFailure(ctx context.Context, job telegramJobRecord, cause error, message string) error {
	observeTelegramPublishFailure(ctx, "media_fallback")
	sourceUnavailable := isTelegramMediaSourceUnavailableError(cause)
	if !sourceUnavailable && !errors.Is(cause, errTelegramMediaFallbackUnavailable) {
		s.enqueueTerminalPublishRecoveryBestEffort(ctx, job, cause)
	}
	if err := s.upsertPublishJobRecord(ctx, job, "failed", message); err != nil {
		g.Log().Warning(ctx, "协议号终态发布记录写入失败", g.Map{
			"event": "telegram_handoff_failure_projection", "jobId": job.Id, "operation": job.OperationNo,
			"error": telegramPublishLogError(err),
		})
	}
	s.appendTelegramJobLog(ctx, job, "account_fallback", "failed", message)
	if job.CollectEventId > 0 {
		if err := s.markCollectDispatchFailedByProfile(ctx, job.ProfileId, job.CollectEventId, message); err != nil {
			g.Log().Warning(ctx, "协议号终态采集分发记录写入失败", g.Map{
				"event": "telegram_handoff_collect_projection", "jobId": job.Id, "operation": job.OperationNo,
				"error": telegramPublishLogError(err),
			})
		}
	}
	if sourceUnavailable {
		queued, err := s.enqueueProfileMediaRepairTask(ctx, job.ProfileId, job.Id)
		if err != nil {
			g.Log().Warning(ctx, "协议号媒体源失效后提交恢复失败", g.Map{
				"event": "telegram_handoff_media_repair", "jobId": job.Id, "profileId": job.ProfileId,
				"operation": job.OperationNo, "error": telegramPublishLogError(err),
			})
		} else if queued {
			s.appendTelegramJobLog(ctx, job, "media_repair", "queued", "Telegram媒体源失效，已提交媒体恢复并重新上架")
		}
	}
	return s.wakeNextTelegramChannelJob(ctx, job)
}

func telegramFallbackTransportStep(purpose string) string {
	return "account_send_" + purpose
}

// prepareTelegramFallbackTransport closes only a proven rejection. The
// transport checkpoint prevents an old Bot Attempt being reused as Account.
func (s *sSysPublish) prepareTelegramFallbackTransport(ctx context.Context, job telegramJobRecord, purpose string) error {
	ctx, cancel := telegramFinalizeContext(ctx, "account_fallback.prepare_transport")
	defer cancel()
	task, _ := ctx.Value(telegramFallbackTaskContextKey{}).(*collectorin.AccountTask)
	return telegramFallbackTransaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockProfilePublishFenceTx(ctx, tx, job); err != nil {
			return err
		}
		if err := lockTelegramFallbackTaskTx(ctx, tx, task, false); err != nil {
			return err
		}
		var current telegramJobRecord
		if err := tx.Model(publishTgJobTable).Ctx(ctx).Where("id", job.Id).LockUpdate().Scan(&current); err != nil {
			return err
		}
		if !telegramFallbackTaskIdentity(current, purpose, task) || !telegramFallbackOwnsPhase(current, purpose) ||
			current.Status != telegramJobStatusSending {
			return errTelegramJobSuperseded
		}
		if err := retireTelegramFallbackRejectedAttemptTx(ctx, tx, current, purpose); err != nil {
			return err
		}
		// Retry from the prerequisite phase, never reuse the rejected token.
		phase := SendPhaseCleanupConfirmed
		expected := SendPhaseDisplaySending
		if purpose == "verify" {
			phase, expected = SendPhaseDisplayConfirmed, SendPhaseVerifySending
		}
		if current.SendPhase != phase && current.SendPhase != expected {
			return errTelegramJobSuperseded
		}
		_, err := tx.Model(publishTgJobTable).Ctx(ctx).Where("id", job.Id).Where("status", telegramJobStatusSending).
			Where("send_phase", current.SendPhase).Data(g.Map{"send_phase": phase}).Update()
		return err
	})
}

func (s *sSysPublish) markTelegramFallbackTransport(ctx context.Context, job telegramJobRecord, purpose string, attemptID int64) error {
	ctx, cancel := telegramFinalizeContext(ctx, "account_fallback.mark_transport")
	defer cancel()
	task, _ := ctx.Value(telegramFallbackTaskContextKey{}).(*collectorin.AccountTask)
	if task == nil {
		return errTelegramJobSuperseded
	}
	if err := s.checkTelegramFallbackExecution(ctx, job, purpose); err != nil {
		return err
	}
	return saveTelegramFallbackTransportCheckpoint(g.DB().Model(publishTgDeliveryStepTable).Ctx(ctx), job.Id, purpose, attemptID, task.TaskKey)
}

// OnDuplicate takes column names (or column mappings), not literal values.
// Keep checkpoint data bound through Data so task keys cannot become SQL identifiers.
func saveTelegramFallbackTransportCheckpoint(model *gdb.Model, jobID int64, purpose string, attemptID int64, taskKey string) error {
	_, err := model.Data(g.Map{
		"job_id": jobID, "step_key": telegramFallbackTransportStep(purpose), "attempt_id": attemptID,
		"status": telegramDeliveryStepRunning, "error_message": taskKey,
	}).OnConflict("job_id,step_key").OnDuplicate("attempt_id", "status", "error_message", "updated_at").Save()
	return err
}

// All handoff transactions share the configured finalize budget and short
// database-side lock/statement limits. Telegram sends never use this context.
func telegramFallbackTransaction(ctx context.Context, fn func(context.Context, gdb.TX) error) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := boundTelegramConfirmationTransaction(ctx, tx); err != nil {
			return err
		}
		return fn(ctx, tx)
	})
}

func (s *sSysPublish) recordTelegramFallbackAccountRejection(ctx context.Context, job telegramJobRecord, attemptID int64, purpose string, cause error) error {
	if !telegramFallbackBotFailureKnown(cause) {
		return errTelegramAttemptNeedsReconcile
	}
	ctx, cancel := telegramFinalizeContext(ctx, "account_fallback.protocol_rejection")
	defer cancel()
	task, _ := ctx.Value(telegramFallbackTaskContextKey{}).(*collectorin.AccountTask)
	return telegramFallbackTransaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockProfilePublishFenceTx(ctx, tx, job); err != nil {
			return err
		}
		if err := lockTelegramFallbackTaskTx(ctx, tx, task, false); err != nil {
			return err
		}
		var current telegramJobRecord
		if err := tx.Model(publishTgJobTable).Ctx(ctx).Where("id", job.Id).LockUpdate().Scan(&current); err != nil {
			return err
		}
		if !telegramFallbackTaskIdentity(current, purpose, task) || !telegramFallbackOwnsPhase(current, purpose) || current.Status != telegramJobStatusSending {
			return errTelegramJobSuperseded
		}
		return recordTelegramFallbackRejectionTx(ctx, tx, current, attemptID, purpose, cause, "protocol")
	})
}
