package sys

import (
	"context"
	"errors"
	"fmt"
	"hotgo/addons/youban_publish/model"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
)

// A full-push operation belongs to one concrete channel batch, not the
// profile's last UI operation. Parsing alone never grants execution authority.
type profileFullPushScope struct {
	BatchNo   string
	ChannelID int64
	ProfileID int64
}

func parseProfileFullPushScope(operationNo string) (profileFullPushScope, bool) {
	parts := strings.Split(operationNo, ":")
	if len(parts) != 4 || parts[0] != "full_push" {
		return profileFullPushScope{}, false
	}
	values := [3]int64{}
	for i := range values {
		value, err := strconv.ParseInt(parts[i+1], 10, 64)
		if err != nil || value <= 0 || strconv.FormatInt(value, 10) != parts[i+1] {
			return profileFullPushScope{}, false
		}
		values[i] = value
	}
	return profileFullPushScope{
		BatchNo: strings.Join(parts[:3], ":"), ChannelID: values[0], ProfileID: values[2],
	}, true
}

func profileFullPushBatchTx(ctx context.Context, tx gdb.TX, tenantID int64, scope profileFullPushScope) (gdb.Record, error) {
	// Do not lock the batch: batch finalization is an aggregate projection and
	// must not introduce batch -> state / state -> batch lock inversion.
	return tx.Model(publishFullPushBatchTable).Ctx(ctx).
		Where("tenant_id", tenantID).Where("channel_id", scope.ChannelID).
		Where("batch_no", scope.BatchNo).One()
}

func profileFullPushAdmissionStatusIsTerminal(status string) bool {
	switch status {
	case "", "failed", "canceled":
		return true
	default:
		return false
	}
}

func lockProfileFullPushFenceTx(ctx context.Context, tx gdb.TX, job telegramJobRecord) error {
	if !profilePublishCancelEnabled() {
		return nil
	}
	scope, valid := parseProfileFullPushScope(job.OperationNo)
	admission := job.Id == 0 && job.ChannelId > 0
	minimalRecovery := job.Id == 0 && job.ChannelId == 0
	if minimalRecovery {
		// Recovery has an operation identity, but no creation target. It may
		// recover only that exact current UI operation (checked under lock).
		job.ChannelId = scope.ChannelID
	}
	if !valid || job.TenantId <= 0 || job.AccountId < 0 ||
		job.ProfileId != scope.ProfileID || job.ChannelId != scope.ChannelID {
		return errTelegramJobSuperseded
	}
	state, err := tx.Model(publishProfileStateTable).Ctx(ctx).
		Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).WhereNull("deleted_at").
		Fields("publish_operation_no,publish_task_status").LockUpdate().One()
	if err != nil {
		return err
	}
	if state.IsEmpty() || (minimalRecovery && state["publish_operation_no"].String() != job.OperationNo) {
		return errTelegramJobSuperseded
	}
	stateStatus := state["publish_task_status"].String()
	switch stateStatus {
	case "canceling", "unpublishing":
		return errTelegramJobSuperseded
	case "canceled":
		// A distinct new admission may replace a canceled round. Existing
		// Jobs/recovery cannot revive it, even if its tombstone is absent.
		previousOperation := state["publish_operation_no"].String()
		if !admission || previousOperation == "" || previousOperation == job.OperationNo {
			return errTelegramJobSuperseded
		}
	}
	columns := dao.ContentProfile.Columns()
	profile, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).
		Where(columns.Id, job.ProfileId).WhereNull(columns.DeletedAt).
		Fields(columns.Status, columns.Visibility).One()
	if err != nil {
		return err
	}
	if profile.IsEmpty() || profile[columns.Status].Int() != 1 ||
		profile[columns.Visibility].String() != consts.ContentVisibilityPublic {
		return errTelegramJobSuperseded
	}
	if canceled, cancelErr := profileOperationCanceledTx(ctx, tx, job); cancelErr != nil {
		return cancelErr
	} else if canceled {
		return errTelegramJobSuperseded
	}
	batch, err := profileFullPushBatchTx(ctx, tx, job.TenantId, scope)
	if err != nil {
		return err
	}
	// completed/partial describe an aggregate (which historically omits some
	// reconciliation states), not permission to discard a remaining Job.
	if batch.IsEmpty() || batch["id"].Int64() <= 0 ||
		batch["snapshot_max_profile_id"].Int64() < job.ProfileId {
		return errTelegramJobSuperseded
	}
	displayOperation := state["publish_operation_no"].String()
	if admission && displayOperation == "" && !profileFullPushAdmissionStatusIsTerminal(stateStatus) {
		return errTelegramJobSuperseded
	}
	if displayOperation != "" && displayOperation != job.OperationNo {
		displayScope, isFullPush := parseProfileFullPushScope(displayOperation)
		if !isFullPush {
			// Only explicit new admission may replace a completed ordinary
			// publish slot. A running Job or minimal recovery cannot use this
			// exception to cross a newer operation's lifecycle barrier.
			displayType := profileOperationTypeFromNo(displayOperation)
			if !admission || !profileFullPushAdmissionStatusIsTerminal(stateStatus) ||
				displayType == profileOperationUnpublish || displayType == profileOperationDelete ||
				strings.HasPrefix(displayOperation, "unpublish:") || strings.HasPrefix(displayOperation, "delete:") ||
				strings.HasPrefix(displayOperation, "full_push:") {
				return errTelegramJobSuperseded
			}
			// The incoming operation's tombstone was checked above. An older
			// operation's cancellation must not prohibit a distinct new batch.
		} else {
			if displayScope.ProfileID != job.ProfileId {
				return errTelegramJobSuperseded
			}
			displayBatch, readErr := profileFullPushBatchTx(ctx, tx, job.TenantId, displayScope)
			if readErr != nil {
				return readErr
			}
			if displayBatch.IsEmpty() || displayBatch["snapshot_max_profile_id"].Int64() < job.ProfileId ||
				(displayScope.ChannelID == scope.ChannelID && displayBatch["id"].Int64() > batch["id"].Int64()) {
				return errTelegramJobSuperseded
			}
		}
	}
	// active_key is unique. Never search historical batches by channel/id:
	// LIMIT alone would not bound that scan with the existing batch indexes.
	active, err := tx.Model(publishFullPushBatchTable).Ctx(ctx).
		Where("active_key", fmt.Sprintf("%d:%d", job.TenantId, job.ChannelId)).
		Fields("id,tenant_id,channel_id").One()
	if err != nil {
		return err
	}
	if !active.IsEmpty() && (active["tenant_id"].Int64() != job.TenantId ||
		active["channel_id"].Int64() != job.ChannelId || active["id"].Int64() > batch["id"].Int64()) {
		return errTelegramJobSuperseded
	}
	if job.Id <= 0 {
		// New admission needs positive ownership of the unique active slot.
		// Otherwise a historical completed batch could create missing Jobs
		// after its successor cleared active_key. Existing Jobs and exact-current
		// minimal recovery do not depend on this aggregate slot remaining active.
		if !minimalRecovery && (active.IsEmpty() || active["id"].Int64() != batch["id"].Int64()) {
			return errTelegramJobSuperseded
		}
		return nil
	}
	var current telegramJobRecord
	if err := tx.Model(publishTgJobTable).Ctx(ctx).Where("id", job.Id).LockUpdate().Scan(&current); err != nil {
		return err
	}
	if !profilePublishJobOperationMatches(job, current) ||
		current.Status == JobStatusSuperseded || current.SendPhase == telegramSendPhaseTimeoutCancelled {
		return errTelegramJobSuperseded
	}
	newer, err := tx.Model(publishTgJobTable).Ctx(ctx).
		Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).Where("channel_id", job.ChannelId).
		WhereGT("id", job.Id).WhereNot("operation_no", job.OperationNo).
		Where("(COALESCE(operation_type,'')='' OR operation_type='publish')").
		Fields("id").Limit(1).One()
	if err != nil {
		return err
	}
	if !newer.IsEmpty() {
		return errTelegramJobSuperseded
	}
	return nil
}

func profilePublishJobOperationMatches(observed, current telegramJobRecord) bool {
	return observed.Id > 0 && observed.Id == current.Id &&
		observed.TenantId == current.TenantId && observed.AccountId == current.AccountId &&
		observed.ProfileId == current.ProfileId && observed.ChannelId == current.ChannelId &&
		observed.OperationNo == current.OperationNo
}

func profilePublishJobGenerationMatches(observed, current telegramJobRecord) bool {
	// Transport and queue metadata may change within a valid operation. Only
	// the execution generation fences a late error handler after handoff.
	return profilePublishJobOperationMatches(observed, current) &&
		observed.ReconcileCount == current.ReconcileCount
}

func profileSupersededJobHasNoSendEvidence(job telegramJobRecord, attemptCount, messageCount int) bool {
	if attemptCount != 0 || messageCount != 0 || job.TaskId != 0 || job.SentAt != nil {
		return false
	}
	switch job.SendPhase {
	case "", telegramSendPhaseCleanupProcessing, telegramSendPhaseCleanupConfirmed:
		return true
	default:
		return false
	}
}

// Called only for the superseded error path. A late handler must not retire a
// replacement generation; any Attempt (even superseded) retains receipt rights.
func (s *sSysPublish) settleSupersededTelegramSendTx(ctx context.Context, tx gdb.TX, observed telegramJobRecord) (retired, awaiting, transitioned bool, resultErr error) {
	if observed.Id <= 0 {
		return false, false, false, gerror.New("废弃TG发送任务缺少任务ID")
	}
	if _, err := tx.Model(publishProfileStateTable).Ctx(ctx).
		Where("tenant_id", observed.TenantId).Where("account_id", observed.AccountId).
		Where("profile_id", observed.ProfileId).Fields("profile_id").LockUpdate().One(); err != nil {
		return false, false, false, err
	}
	var current telegramJobRecord
	if err := tx.Model(publishTgJobTable).Ctx(ctx).Where("id", observed.Id).LockUpdate().Scan(&current); err != nil {
		return false, false, false, err
	}
	if !profilePublishJobGenerationMatches(observed, current) ||
		(current.Status != JobStatusSending && current.Status != JobStatusSuperseded) {
		return false, false, false, nil
	}
	if current.Status == JobStatusSending {
		// The original error is only an observation. Recheck after both locks:
		// it must not retire a currently valid sender of the same generation.
		if fenceErr := lockProfilePublishFenceTx(ctx, tx, current); fenceErr == nil {
			return false, false, false, gerror.Wrap(errTelegramJobSuperseded, "当前TG任务归属仍有效，保留任务等待重新检查")
		} else if !errors.Is(fenceErr, errTelegramJobSuperseded) {
			return false, false, false, fenceErr
		}
	}
	// State -> Job; Attempt existence is a read, never a receipt mutation.
	// Do not lock Attempt: with cancellation disabled, API confirmation can
	// lock Attempt before its Job CAS. Preparation still needs the Job lock,
	// so it cannot create a new Attempt while we prove absence here.
	attempts, err := tx.Model(publishTgAttemptTable).Ctx(ctx).
		Where("job_id", current.Id).Fields("id").Limit(1).All()
	if err != nil {
		return false, false, false, err
	}
	messages, err := tx.Model(publishTgMessageTable).Unscoped().Ctx(ctx).
		Where("job_id", current.Id).Count()
	if err != nil {
		return false, false, false, err
	}
	audit, err := tx.Model(publishTgAttemptMessageTable).Ctx(ctx).Where("job_id", current.Id).Count()
	if err != nil {
		return false, false, false, err
	}
	noEvidence := profileSupersededJobHasNoSendEvidence(current, len(attempts), messages+audit)
	// A stale ownership callback can arrive after this same Job has already
	// durably confirmed its display send. In that case the display evidence is
	// the current operation's own evidence, not evidence of an obsolete sender.
	// Keep the confirmed phase and make the Job immediately eligible to resume
	// the verification step; converting it to unknown here strands the Job
	// before a verification Attempt can be created.
	if current.Status == JobStatusSending && current.SendPhase == telegramSendPhaseDisplayConfirmed &&
		len(attempts) > 0 && messages+audit > 0 {
		message := "当前TG任务展示资料已确认，恢复验证资料发送"
		transition := telegramJobTransition{
			From: []JobStatus{JobStatusSending}, To: JobStatusRetryWait,
			SendPhases: []SendPhase{SendPhaseDisplayConfirmed}, SendPhase: jobPhase(SendPhaseDisplayConfirmed),
			DispatchStatus: jobDispatch(DispatchStatusIdle), RetryDelay: 0,
			Additional: g.Map{"last_dispatch_error": message},
		}
		affected, err := s.transitionTelegramJobsTx(ctx, tx, []int64{current.Id}, transition)
		if err != nil || affected == 0 {
			return false, false, false, err
		}
		return false, true, true, nil
	}
	if current.Status == JobStatusSuperseded {
		return noEvidence, false, false, nil // Retry only an already safe outbox close.
	}
	target := JobStatusUnknown
	message := "旧发送归属已变化，保留Attempt及迟到回执，等待既有对账或取消清理"
	if noEvidence {
		target = JobStatusSuperseded
		message = "旧发送归属已变化，确认没有Attempt或远端发送证据，已停止任务"
	}
	transition := telegramJobTransition{
		From: []JobStatus{JobStatusSending}, To: target, ErrorMessage: message,
		RetryDelay: telegramUnknownReconcileDelay,
		Additional: g.Map{"last_dispatch_error": message},
	}
	if !noEvidence {
		transition.DispatchStatus = jobDispatch(DispatchStatusProcessing)
	}
	affected, err := s.transitionTelegramJobsTx(ctx, tx, []int64{current.Id}, transition)
	if err != nil || affected == 0 {
		return false, false, false, err
	}
	if !noEvidence {
		return false, true, true, nil
	}
	return true, false, true, nil
}

func (s *sSysPublish) settleSupersededTelegramSend(ctx context.Context, job telegramJobRecord) error {
	var retired, awaiting, transitioned bool
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var err error
		retired, awaiting, transitioned, err = s.settleSupersededTelegramSendTx(ctx, tx, job)
		return err
	})
	if err != nil {
		return gerror.Wrap(err, "持久化TG旧发送收尾失败")
	}
	decision := "ownership_changed"
	if retired && !transitioned {
		decision = "superseded_outbox_repair"
	} else if retired {
		decision = "superseded_without_send_evidence"
	} else if awaiting {
		decision = "superseded_awaiting_reconciliation"
	}
	if retired && transitioned {
		observePublishTerminalCommit(ctx, job, JobStatusSuperseded, true, nil)
	}
	g.Log().Info(ctx, "TG发布操作收尾", g.Map{
		"event": "telegram_operation_settled", "jobId": job.Id, "profileId": job.ProfileId,
		"channelId": job.ChannelId, "operationNo": job.OperationNo, "decision": decision,
		"retired": retired, "awaitingReconciliation": awaiting,
	})
	s.appendTelegramJobLog(ctx, job, "publish", decision,
		fmt.Sprintf("旧发送收尾 jobId:%d retired:%t awaitingReconciliation:%t", job.Id, retired, awaiting))
	if retired || awaiting {
		status := model.PublishRecordStatusUnknown
		if retired {
			status = model.PublishRecordStatusSuperseded
		}
		if err := s.upsertPublishJobRecord(ctx, job, status, decision); err != nil {
			return err
		}
	}
	if retired {
		if err := s.clearSupersededProfileOperationIfTerminal(ctx, job); err != nil {
			return err
		}
		// Reuse the canonical close after commit: it also removes the channel
		// pending-index hint. Never close a receipt-bearing Job's outbox here.
		rows, err := g.DB().Model(publishTgOutboxTable).Ctx(ctx).Where("job_id", job.Id).
			Where("event_type", telegramOutboxEventPublish).Fields("id").All()
		if err != nil {
			return err
		}
		for _, row := range rows {
			if err := s.completeTelegramOutbox(ctx, row["id"].Int64()); err != nil {
				return err
			}
		}
	}
	if retired || awaiting {
		return s.wakeNextTelegramChannelJob(ctx, job)
	}
	return nil
}
