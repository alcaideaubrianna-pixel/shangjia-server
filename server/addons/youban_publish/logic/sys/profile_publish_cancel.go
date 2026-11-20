package sys

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const publishProfileCancelTable = "hg_youban_publish_profile_cancel"

type profilePublishCancelUUIDKey struct{}

func profilePublishCancelEnabled() bool {
	value := os.Getenv("YOUBAN_PUBLISH_PROFILE_CANCEL_ENABLED")
	return value == "" || strings.EqualFold(value, "true")
}

// profilePublishCancel is an operation tombstone. It never changes the
// profile's business status or channel loop configuration.
type profilePublishCancel struct {
	ID           int64  `orm:"id"`
	TenantID     int64  `orm:"tenant_id"`
	AccountID    int64  `orm:"account_id"`
	ProfileID    int64  `orm:"profile_id"`
	Uuid         string `orm:"uuid"`
	OperationNo  string `orm:"operation_no"`
	Status       string `orm:"status"`
	ErrorMessage string `orm:"error_message"`
}

func profileCancelScope(model *gdb.Model, job telegramJobRecord) *gdb.Model {
	return model.Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).Where("operation_no", job.OperationNo)
}

func profileOperationCanceledTx(ctx context.Context, tx gdb.TX, job telegramJobRecord) (bool, error) {
	if !profilePublishCancelEnabled() || job.ProfileId <= 0 || job.OperationNo == "" {
		return false, nil
	}
	count, err := profileCancelScope(tx.Model(publishProfileCancelTable).Ctx(ctx), job).Count()
	return count > 0, err
}

// lockProfilePublishFenceTx serializes generation, completion and cancellation.
// Lock order is profile_state before Job/Attempt/Outbox.
func lockProfilePublishFenceTx(ctx context.Context, tx gdb.TX, job telegramJobRecord) error {
	if !profilePublishCancelEnabled() || job.ProfileId <= 0 || job.OperationNo == "" {
		return nil
	}
	if strings.HasPrefix(job.OperationNo, "full_push:") {
		return lockProfileFullPushFenceTx(ctx, tx, job)
	}
	state, err := tx.Model(publishProfileStateTable).Ctx(ctx).
		Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).Fields("publish_operation_no,publish_task_status").
		LockUpdate().One()
	if err != nil {
		return err
	}
	return checkProfilePublishOperationState(job, state)
}

func checkProfilePublishOperationState(job telegramJobRecord, state gdb.Record) error {
	if !state.IsEmpty() && (state["publish_operation_no"].String() != job.OperationNo ||
		state["publish_task_status"].String() == "canceling" || state["publish_task_status"].String() == "canceled") {
		return errTelegramJobSuperseded
	}
	return nil
}

func checkProfilePublishOperationBeforePrepare(ctx context.Context, job telegramJobRecord) error {
	if !profilePublishCancelEnabled() || job.ProfileId <= 0 || job.OperationNo == "" ||
		strings.HasPrefix(job.OperationNo, "full_push:") {
		return nil
	}
	// This is an early rejection only. The Attempt transaction still locks
	// and checks the operation state before any Telegram send can begin.
	state, err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).Master().
		Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).Fields("publish_operation_no,publish_task_status").One()
	if err != nil {
		return err
	}
	return checkProfilePublishOperationState(job, state)
}

func (s *sSysPublish) AdminProfilePublishCancel(ctx context.Context, in *sysin.ProfilePublishCancelInp) (*sysin.ProfileTaskStatusModel, error) {
	if !profilePublishCancelEnabled() {
		return nil, gerror.New("取消发布暂未启用，请先完成后端迁移和工作节点升级")
	}
	if err := in.Filter(ctx); err != nil {
		return nil, err
	}
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	profile, err := s.profileTaskStatusBySelector(ctx, &sysin.ProfileViewInp{Uuid: in.Uuid})
	if err != nil {
		return nil, err
	}
	if account.TenantId > 0 && profile.TenantId != account.TenantId {
		return nil, gerror.New("资料不存在或无权操作")
	}
	if err = s.ensureAdminManageableAccount(ctx, account, profile.AccountId); err != nil {
		return nil, gerror.New("资料不存在或无权操作")
	}
	ctx = context.WithValue(ctx, profilePublishCancelUUIDKey{}, profile.Uuid)
	job := telegramJobRecord{TenantId: profile.TenantId, AccountId: profile.AccountId, ProfileId: profile.Id, OperationNo: in.OperationNo}
	err = s.withProfileLifecycleLock(ctx, job.TenantId, job.ProfileId, func() error {
		return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			return s.acceptProfilePublishCancelTx(ctx, tx, job)
		})
	})
	if err != nil {
		return nil, err
	}
	logProfileCancel(ctx, "accepted", job, 0)
	// Do not hold profile_state while waiting on a sending Job's row lock.
	// A sender may need that state to finish its local callback. The committed
	// intent is already an execution fence, and recovery retries this phase.
	stopCtx, cancelStop := context.WithTimeout(ctx, 3*time.Second)
	defer cancelStop()
	if err := g.DB().Transaction(stopCtx, func(ctx context.Context, tx gdb.TX) error {
		return s.stopCanceledProfileJobsTx(ctx, tx, job)
	}); err != nil {
		g.Log().Warningf(ctx, "取消意图已受理，任务停止由恢复周期重试 profileId:%d operationNo:%s err:%+v", job.ProfileId, job.OperationNo, err)
	}
	return s.AdminProfileTaskStatus(ctx, &sysin.ProfileViewInp{Uuid: in.Uuid})
}

func profileCancelRequestAllowed(currentOperation, currentStatus, requestedOperation string) error {
	if currentOperation != requestedOperation {
		return gerror.New("发布轮次已变更，请刷新后重试")
	}
	switch currentStatus {
	case "canceling", "canceled", "pending", "publishing", "cycling", "waiting_media":
		return nil
	default:
		return gerror.New("本轮发布已结束，如需删除消息请下架")
	}
}

func (s *sSysPublish) acceptProfilePublishCancelTx(ctx context.Context, tx gdb.TX, job telegramJobRecord) error {
	state, err := tx.Model(publishProfileStateTable).Ctx(ctx).
		Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).WhereNull("deleted_at").
		Fields("publish_operation_no,publish_task_status").LockUpdate().One()
	if err != nil {
		return err
	}
	if state.IsEmpty() {
		return gerror.New("资料不存在或无权操作")
	}
	currentStatus := state["publish_task_status"].String()
	if err = profileCancelRequestAllowed(state["publish_operation_no"].String(), currentStatus, job.OperationNo); err != nil {
		logProfileCancel(ctx, "round_conflict", job, 0)
		return err
	}
	if currentStatus == "canceling" || currentStatus == "canceled" {
		return nil
	}
	// Same transaction as the state change and cleanup intents. A crash or Redis
	// outage after commit cannot lose cancellation or turn it into success.
	uuid, _ := ctx.Value(profilePublishCancelUUIDKey{}).(string)
	if _, err = tx.Model(publishProfileCancelTable).Ctx(ctx).InsertIgnore(g.Map{
		"tenant_id": job.TenantId, "account_id": job.AccountId, "profile_id": job.ProfileId,
		"operation_no": job.OperationNo, "status": "canceling", "uuid": uuid,
	}); err != nil {
		return gerror.Wrap(err, "保存取消发布意图失败，请确认迁移已执行")
	}
	if err = persistProfileCancelInitialCleanupTx(ctx, tx, job); err != nil {
		return err
	}
	return s.writeProfilePublishOperationStateTx(ctx, tx, job.TenantId, job.AccountId,
		job.ProfileId, job.OperationNo, sysin.PublishTaskStatusCanceling, true)
}

func persistProfileCancelInitialCleanupTx(ctx context.Context, tx gdb.TX, operation telegramJobRecord) error {
	var jobs []telegramJobRecord
	// Generation holds the same profile state lock as the caller, so the Job
	// set is now fixed. A plain read avoids state -> Job / Job -> state inversion.
	if err := profileCancelScope(tx.Model(publishTgJobTable).Ctx(ctx), operation).
		OrderAsc("id").Scan(&jobs); err != nil {
		return err
	}
	for _, job := range jobs {
		if err := persistProfileCancelCleanupTx(ctx, tx, job); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) stopCanceledProfileJobsTx(ctx context.Context, tx gdb.TX, operation telegramJobRecord) error {
	var jobs []telegramJobRecord
	if err := profileCancelScope(tx.Model(publishTgJobTable).Ctx(ctx), operation).
		OrderAsc("id").LockUpdate().Scan(&jobs); err != nil {
		return err
	}
	for _, job := range jobs {
		if _, err := s.transitionTelegramJobsTx(ctx, tx, []int64{job.Id}, telegramJobTransition{
			From: telegramJobActiveStatuses(), To: JobStatusSuperseded,
			Additional: g.Map{"next_retry_at": nil, "last_dispatch_error": "资料本轮发布已取消"},
		}); err != nil {
			return err
		}
		// Sending attempts retain their receipt/reconciliation evidence. They
		// must not be declared absent merely because the Job is superseded.
		if err := persistProfileCancelCleanupTx(ctx, tx, job); err != nil {
			return err
		}
		// Settle only publish delivery intents, never the cleanup intents just
		// persisted above. Redis hints are not execution authority.
		if _, err := tx.Model(publishTgOutboxTable).Ctx(ctx).Where("job_id", job.Id).
			Where("event_type", telegramOutboxEventPublish).
			Data(g.Map{"status": telegramOutboxDelivered, "lease_owner": "", "lease_expires_at": nil}).Update(); err != nil {
			return err
		}
	}
	return nil
}

// Uses the existing durable cleanup Outbox. Called inside the cancellation or
// receipt transaction, never a best-effort post-commit enqueue.
func persistProfileCancelCleanupTx(ctx context.Context, tx gdb.TX, job telegramJobRecord) error {
	key := fmt.Sprintf("cancel:%x:initial", sha256.Sum256([]byte(job.OperationNo)))
	_, _, err := ensureTelegramCleanupIntent(ctx, job.Id, telegramOutboxCleanupPrefix+"manual", key,
		tgQueuePayload{JobId: job.Id, CleanupOperationNo: key})
	return err
}

func logProfileCancel(ctx context.Context, event string, job telegramJobRecord, taskID int64) {
	uuid, _ := ctx.Value(profilePublishCancelUUIDKey{}).(string)
	if uuid == "" && profilePublishCancelEnabled() {
		if value, err := profileCancelScope(g.DB().Model(publishProfileCancelTable).Ctx(ctx), job).Value("uuid"); err == nil {
			uuid = value.String()
		}
	}
	g.Log().Info(ctx, "资料取消发布", g.Map{"event": event, "tenantId": job.TenantId,
		"accountId": job.AccountId, "profileId": job.ProfileId, "operationNo": job.OperationNo,
		"uuid": uuid, "jobId": job.Id, "taskId": taskID})
}

// Keep state callbacks from converting a cancel into success/failure. A new
// operation cannot start until the current cancellation has fully settled.
func (s *sSysPublish) fenceProfileStateWriteTx(ctx context.Context, tx gdb.TX, tenantID, accountID, profileID int64, operationNo string, status sysin.PublishTaskStatus, requireCurrent bool) error {
	if !profilePublishCancelEnabled() {
		return nil
	}
	state, err := tx.Model(publishProfileStateTable).Ctx(ctx).
		Where("tenant_id", tenantID).Where("account_id", accountID).Where("profile_id", profileID).
		Fields("publish_operation_no,publish_task_status").LockUpdate().One()
	if err != nil {
		return err
	}
	if requireCurrent {
		return nil
	}
	if state["publish_task_status"].String() == "canceling" {
		return gerror.New("本轮发布正在取消，请等待清理完成")
	}
	job := telegramJobRecord{TenantId: tenantID, AccountId: accountID, ProfileId: profileID, OperationNo: operationNo}
	canceled, err := profileOperationCanceledTx(ctx, tx, job)
	if err != nil {
		return err
	}
	if canceled {
		return errTelegramJobSuperseded
	}
	return nil
}

// Receipt identity is immutable: a later message gets a distinct cleanup
// Outbox row even when an earlier cleanup for the same Job was acknowledged.
func persistCanceledReceiptCleanupTx(ctx context.Context, tx gdb.TX, job telegramJobRecord, messages []*telegramSentMessage) error {
	for _, message := range messages {
		if message == nil || message.MessageId <= 0 {
			continue
		}
		key := fmt.Sprintf("cancel:%x:%d", sha256.Sum256([]byte(job.OperationNo)), message.MessageId)
		if _, _, err := ensureTelegramCleanupIntent(ctx, job.Id, telegramOutboxCleanupPrefix+"manual", key,
			tgQueuePayload{JobId: job.Id, CleanupOperationNo: key}); err != nil {
			return err
		}
	}
	// A genuinely new late message reopens cleanup even if an earlier sweep
	// finished. Replaying deleted evidence does not resurrect a cancellation.
	remaining, err := tx.Model(publishTgMessageTable).Ctx(ctx).Where("job_id", job.Id).Where("status", "sent").Count()
	if err != nil {
		return err
	}
	if remaining > 0 {
		if _, err := profileCancelScope(tx.Model(publishProfileCancelTable).Ctx(ctx), job).
			Data(g.Map{"status": "canceling", "next_check_at": gtime.Now()}).Update(); err != nil {
			return err
		}
		state, err := tx.Model(publishProfileStateTable).Ctx(ctx).
			Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).Where("profile_id", job.ProfileId).
			Fields("publish_operation_no,publish_task_status").LockUpdate().One()
		if err != nil {
			return err
		}
		// Cleanup may reopen, but a released frontend or a newer round must not.
		if state["publish_operation_no"].String() != job.OperationNo || state["publish_task_status"].String() == "canceled" {
			return nil
		}
		if err := (&sSysPublish{}).writeProfilePublishOperationStateTx(ctx, tx, job.TenantId, job.AccountId,
			job.ProfileId, job.OperationNo, sysin.PublishTaskStatusCanceling, true); err != nil {
			return err
		}
	}
	logProfileCancel(ctx, "late_receipt_cleanup_persisted", job, 0)
	return nil
}

func lockCanceledReceiptOperationTx(ctx context.Context, tx gdb.TX, job telegramJobRecord) (bool, error) {
	if !profilePublishCancelEnabled() || job.ProfileId <= 0 || job.OperationNo == "" {
		return false, nil
	}
	if _, err := tx.Model(publishProfileStateTable).Ctx(ctx).Where("tenant_id", job.TenantId).
		Where("account_id", job.AccountId).Where("profile_id", job.ProfileId).
		Fields("profile_id").LockUpdate().One(); err != nil {
		return false, err
	}
	return profileOperationCanceledTx(ctx, tx, job)
}

func captureCanceledAttemptReceiptTx(ctx context.Context, tx gdb.TX, attempt telegramDeliveryAttempt, messages []*telegramSentMessage) error {
	var job telegramJobRecord
	if err := tx.Model(publishTgJobTable).Ctx(ctx).Where("id", attempt.JobId).Scan(&job); err != nil {
		return err
	}
	canceled, err := lockCanceledReceiptOperationTx(ctx, tx, job)
	if err != nil || !canceled {
		return err
	}
	if err := saveTelegramSentMessagesTx(ctx, tx, job, messages); err != nil {
		return err
	}
	// Partial evidence is persisted and cleaned, but never proves the in-flight
	// request has completed. Unknown outcomes remain canceling for reconciliation.
	count, err := tx.Model(publishTgAttemptMessageTable).Ctx(ctx).Where("attempt_id", attempt.Id).Count()
	if err != nil || count < attempt.ExpectedCount {
		return err
	}
	_, err = tx.Model(publishTgAttemptTable).Ctx(ctx).Where("id", attempt.Id).
		Data(g.Map{"status": telegramAttemptStatusConfirmed, "response_received": 1,
			"confirmed_count": count, "webhook_deadline": nil}).Update()
	return err
}

func logProfileCancelRecoveryStage(ctx context.Context, job telegramJobRecord, stage string, startedAt time.Time, err error) {
	fields := g.Map{"event": "profile_cancel_recovery_stage", "stage": stage,
		"tenantId": job.TenantId, "accountId": job.AccountId, "profileId": job.ProfileId,
		"operationNo": job.OperationNo, "jobId": job.Id,
		"durationMs": time.Since(startedAt).Milliseconds(), "result": "ok"}
	if err != nil {
		fields["result"] = "error"
		fields["errorType"] = fmt.Sprintf("%T", err)
	}
	// Detailed errors remain at the existing boundary; do not log SQL payloads.
	g.Log().Info(ctx, "取消发布恢复阶段", fields)
}

// Existing publish recovery role owns this bounded, indexed lane. Updating
// next_check_at before work rotates stalled operations and prevents starvation.
func (s *sSysPublish) recoverProfilePublishCancels(ctx context.Context, limit int) error {
	if !profilePublishCancelEnabled() {
		return nil
	}
	var intents []profilePublishCancel
	if err := g.DB().Model(publishProfileCancelTable).Ctx(ctx).
		Where("status", "canceling").WhereLTE("next_check_at", gtime.Now()).
		OrderAsc("next_check_at,id").Limit(limit).Scan(&intents); err != nil {
		return err
	}
	for _, intent := range intents {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, err := g.DB().Model(publishProfileCancelTable).Ctx(ctx).Where("id", intent.ID).
			Data(g.Map{"next_check_at": gtime.Now().Add(5e9)}).Update(); err != nil {
			return err
		}
		job := telegramJobRecord{TenantId: intent.TenantID, AccountId: intent.AccountID, ProfileId: intent.ProfileID, OperationNo: intent.OperationNo}
		runStage := func(stage string, work func() error) error {
			startedAt := time.Now()
			err := work()
			logProfileCancelRecoveryStage(ctx, job, stage, startedAt, err)
			return err
		}
		if err := runStage("stop_jobs", func() error {
			return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
				return s.stopCanceledProfileJobsTx(ctx, tx, job)
			})
		}); err != nil {
			g.Log().Warningf(ctx, "取消发布停止Job待重试 profileId:%d operationNo:%s err:%+v", intent.ProfileID, intent.OperationNo, err)
			continue
		}
		if err := runStage("stop_cycle", func() error { return s.stopCanceledProfileCycleRun(ctx, intent) }); err != nil {
			g.Log().Warningf(ctx, "取消资料循环运行待重试 profileId:%d operationNo:%s err:%+v", intent.ProfileID, intent.OperationNo, err)
			continue
		}
		if err := runStage("settle", func() error {
			return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
				return s.settleProfilePublishCancelTx(ctx, tx, intent)
			})
		}); err != nil {
			g.Log().Warning(ctx, "取消发布收尾待重试", g.Map{"profileId": intent.ProfileID,
				"tenantId": intent.TenantID, "accountId": intent.AccountID, "operationNo": intent.OperationNo, "error": err.Error()})
		}
	}
	return nil
}

func (s *sSysPublish) settleProfilePublishCancelTx(ctx context.Context, tx gdb.TX, intent profilePublishCancel) error {
	job := telegramJobRecord{TenantId: intent.TenantID, AccountId: intent.AccountID, ProfileId: intent.ProfileID, OperationNo: intent.OperationNo}
	// Same ordering as acceptance and generation; no network I/O under this lock.
	lockStartedAt := time.Now()
	_, lockErr := tx.Model(publishProfileStateTable).Ctx(ctx).Where("tenant_id", job.TenantId).
		Where("account_id", job.AccountId).Where("profile_id", job.ProfileId).Fields("profile_id").LockUpdate().One()
	logProfileCancelRecoveryStage(ctx, job, "state_lock", lockStartedAt, lockErr)
	if lockErr != nil {
		return lockErr
	}
	var jobs []telegramJobRecord
	if err := profileCancelScope(tx.Model(publishTgJobTable).Ctx(ctx), job).OrderAsc("id").Scan(&jobs); err != nil {
		return err
	}
	for _, current := range jobs {
		if !current.Status.IsTerminal() {
			return nil // The independent Job-stop phase must finish first.
		}
	}
	// Release only after checking the entire round. The tombstone remains
	// canceling until all remote evidence and cleanup have actually settled.
	if _, err := profileCancelScope(tx.Model(publishProfileCancelTable).Ctx(ctx), job).
		Where("status", "canceling").Where("error_message", "").Data(g.Map{"error_message": "发布已停止，后台仍在对账并清理频道消息；未知发送结果不会视为未发送"}).Update(); err != nil {
		return err
	}
	if err := s.writeProfilePublishOperationStateTx(ctx, tx, job.TenantId, job.AccountId, job.ProfileId,
		job.OperationNo, sysin.PublishTaskStatusCanceled, true); err != nil {
		return err
	}
	for _, current := range jobs {
		// Protocol task account_id is the Telegram account, not the business
		// account. Match the exact Job + opaque operation digest in its key.
		digest := sha256.Sum256([]byte(current.OperationNo))
		taskPattern := fmt.Sprintf("message-media-fallback:%d:%%:%x:%%", current.Id, digest[:12])
		tasks, err := tx.Model(telegramFallbackAccountTaskTable).Ctx(ctx).
			Where("tenant_id", current.TenantId).WhereLike("task_key", taskPattern).
			WhereIn("status", []string{"processing", "finalizing"}).Count()
		if err != nil || tasks > 0 {
			return err
		}
		// Keep unknown/in-flight attempts unresolved. Receipt recovery, not a
		// local timeout, determines whether a remote send actually happened.
		attemptStartedAt := time.Now()
		active, err := s.recoverCanceledProfileAttemptsTx(ctx, tx, current)
		logProfileCancelRecoveryStage(ctx, current, "attempt_evidence", attemptStartedAt, err)
		if err != nil || active > 0 {
			if err == nil && active > 0 {
				_, err = profileCancelScope(tx.Model(publishProfileCancelTable).Ctx(ctx), job).
					Where("COALESCE(error_message, '') <> ?", "发布已停止，后台仍在等待在途发送结果并清理消息；未知结果将继续对账").
					Data(g.Map{"error_message": "发布已停止，后台仍在等待在途发送结果并清理消息；未知结果将继续对账"}).Update()
			}
			return err
		}
		messages, err := tx.Model(publishTgMessageTable).Ctx(ctx).Where("job_id", current.Id).WhereNot("status", "deleted").Count()
		if err != nil || messages > 0 {
			if err == nil && messages > 0 {
				_, err = profileCancelScope(tx.Model(publishProfileCancelTable).Ctx(ctx), job).
					Where("COALESCE(error_message, '') <> ?", "本轮频道消息正在清理；清理失败将由现有队列重试").
					Data(g.Map{"error_message": "本轮频道消息正在清理；清理失败将由现有队列重试"}).Update()
			}
			return err
		}
		pending, err := tx.Model(publishTgOutboxTable).Ctx(ctx).Where("job_id", current.Id).
			WhereLike("event_type", telegramOutboxCleanupPrefix+"%").WhereNot("status", telegramOutboxDelivered).Count()
		if err != nil || pending > 0 {
			return err
		}
	}
	if err := s.writeProfilePublishOperationStateTx(ctx, tx, job.TenantId, job.AccountId, job.ProfileId,
		job.OperationNo, sysin.PublishTaskStatusCanceled, true); err != nil {
		return err
	}
	_, err := profileCancelScope(tx.Model(publishProfileCancelTable).Ctx(ctx), job).
		Where("status", "canceling").Data(g.Map{"status": "canceled", "error_message": ""}).Update()
	if err == nil {
		logProfileCancel(ctx, "canceled", job, 0)
	}
	return err
}

// Record canceled RPC outcomes before considering any fallback. A definite
// rejection is evidence of absence; transport timeouts are not.
func recordCanceledSendOutcome(ctx context.Context, job telegramJobRecord, attempt telegramDeliveryAttempt, messages []*telegramSentMessage, cause error) (bool, error) {
	if !profilePublishCancelEnabled() {
		return false, nil
	}
	finalizeCtx, cancel := telegramFinalizeContext(ctx, "publish_cancel.rpc_outcome")
	defer cancel()
	canceled := false
	err := g.DB().Transaction(finalizeCtx, func(ctx context.Context, tx gdb.TX) error {
		var err error
		canceled, err = lockCanceledReceiptOperationTx(ctx, tx, job)
		if err != nil || !canceled {
			return err
		}
		if err := recordTelegramAttemptMessagesTx(ctx, tx, attempt, messages, "response"); err != nil {
			return err
		}
		if cause != nil && !isTelegramAmbiguousDeliveryError(cause) && len(messages) == 0 {
			_, err := tx.Model(publishTgAttemptTable).Ctx(ctx).Where("id", attempt.Id).
				Data(g.Map{"status": telegramAttemptStatusRetryWait, "error_message": cause.Error()}).Update()
			return err
		}
		return nil
	})
	return canceled, err
}

// Cancellation may retire an attempt after the sender persisted a canonical
// rejection proof. Superseding its lifecycle must not erase that no-send fact.
func canceledAttemptHasDefiniteNoSendProof(attempt telegramDeliveryAttempt, auditCount int) bool {
	if attempt.Id <= 0 || (attempt.Status != telegramAttemptStatusRetryWait && attempt.Status != telegramAttemptStatusSuperseded) ||
		auditCount != 0 || attempt.ResponseReceived != 0 || attempt.ConfirmedCount != 0 || attempt.WebhookDeadline != nil {
		return false
	}
	switch strings.TrimSpace(attempt.ErrorMessage) {
	case "bot_definite_rejection", "bot_preparation_failed_before_send", "protocol_definite_rejection", "protocol_preparation_failed_before_send":
		return true
	default:
		return false
	}
}

// canceledAttemptReceiptNeedsReplay skips only complete, confirmed receipts whose
// message identities are all durably deleted. Missing, live, partial or unknown
// receipts still follow the original capture and cleanup path.
func canceledAttemptReceiptNeedsReplay(attempt telegramDeliveryAttempt, messages []*telegramSentMessage, deleted map[int64]struct{}) bool {
	if attempt.Status != telegramAttemptStatusConfirmed || attempt.ResponseReceived <= 0 || attempt.ExpectedCount <= 0 || len(messages) < attempt.ExpectedCount {
		return true
	}
	for _, message := range messages {
		if message == nil || message.MessageId <= 0 {
			return true
		}
		if _, ok := deleted[message.MessageId]; !ok {
			return true
		}
	}
	return false
}

// Recovery also examines superseded Attempts: an older worker may already
// have retired one, but its audit remains evidence. Never resume sending.
func (s *sSysPublish) recoverCanceledProfileAttemptsTx(ctx context.Context, tx gdb.TX, job telegramJobRecord) (int, error) {
	var attempts []telegramDeliveryAttempt
	if err := tx.Model(publishTgAttemptTable).Ctx(ctx).Where("job_id", job.Id).OrderAsc("id").Scan(&attempts); err != nil {
		return 0, err
	}
	// Deleted message facts are durable cleanup proof. Replaying their already
	// confirmed receipts would only rewrite metadata and recheck cleanup intents.
	deletedMessages := make(map[int64]struct{})
	for _, attempt := range attempts {
		if attempt.Status != telegramAttemptStatusConfirmed || attempt.ResponseReceived <= 0 || attempt.ExpectedCount <= 0 {
			continue
		}
		var deleted []struct {
			MessageId int64 `orm:"tg_message_id"`
		}
		if err := tx.Model(publishTgMessageTable).Unscoped().Ctx(ctx).
			Where("job_id", job.Id).Where("status", "deleted").Fields("tg_message_id").Scan(&deleted); err != nil {
			return 0, err
		}
		for _, message := range deleted {
			deletedMessages[message.MessageId] = struct{}{}
		}
		break
	}
	skippedReceipts := 0
	defer func() {
		if skippedReceipts > 0 {
			g.Log().Info(ctx, "取消恢复复用已删除回执", g.Map{
				"event": "profile_cancel_receipts_reused", "jobId": job.Id,
				"profileId": job.ProfileId, "operationNo": job.OperationNo, "skippedAttempts": skippedReceipts,
			})
		}
	}()
	unresolved := 0
	for _, attempt := range attempts {
		var rows []telegramAttemptMessageRecord
		if err := tx.Model(publishTgAttemptMessageTable).Ctx(ctx).Where("attempt_id", attempt.Id).OrderAsc("id").Scan(&rows); err != nil {
			return 0, err
		}
		messages := make([]*telegramSentMessage, 0, len(rows))
		for _, row := range rows {
			message := &telegramSentMessage{MessageId: row.MessageId, Purpose: attempt.Phase}
			if strings.TrimSpace(row.MessagePayload) != "" {
				if err := json.Unmarshal([]byte(row.MessagePayload), message); err != nil {
					return 0, gerror.Wrap(err, "恢复取消发布消息审计失败")
				}
				message.MessageId = row.MessageId
			}
			messages = append(messages, message)
		}
		if len(messages) > 0 {
			if canceledAttemptReceiptNeedsReplay(attempt, messages, deletedMessages) {
				if err := captureCanceledAttemptReceiptTx(ctx, tx, attempt, messages); err != nil {
					return 0, err
				}
			} else {
				skippedReceipts++
			}
		}
		if len(messages) >= attempt.ExpectedCount && (attempt.ExpectedCount > 0 || attempt.ResponseReceived > 0) {
			continue
		}
		// A definite rejection with no IDs proves there is nothing to delete.
		// Merely changing an attempt to superseded does not provide that proof.
		if canceledAttemptHasDefiniteNoSendProof(attempt, len(rows)) ||
			telegramUnknownAttemptRecoveryAction(attempt, len(rows)) == telegramUnknownAttemptRetry {
			continue
		}
		unresolved++
	}
	return unresolved, nil
}
