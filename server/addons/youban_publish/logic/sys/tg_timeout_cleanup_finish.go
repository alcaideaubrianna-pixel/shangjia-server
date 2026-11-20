package sys

import (
	"context"
	"errors"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/lib/pq"
)

type telegramTimeoutTailDecision struct {
	TailID   int64
	ActiveID int64
	Action   string
	Reason   string
}

func telegramTimeoutActiveConflict(err error) bool {
	var pg *pq.Error
	return errors.As(err, &pg) && pg.Code == "23505" &&
		pg.Constraint == "uk_ybp_tg_job_active_profile_channel"
}

// A uniqueness conflict aborts PostgreSQL's transaction. Retry only after
// Transaction has rolled it back, and re-run every cleanup/generation gate.
func (s *sSysPublish) finishTelegramSendTimeoutCleanup(ctx context.Context, job telegramJobRecord) error {
	for retry := 0; retry < 2; retry++ {
		var decision telegramTimeoutTailDecision
		err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			var err error
			decision, err = s.finishTelegramSendTimeoutCleanupTx(ctx, tx, job)
			return err
		})
		if err == nil {
			if decision.Action != "snapshot_changed" && telegramChannelQueueEnabled() {
				finishTelegramPredecessorWake(ctx, job.Id)
				if _, wakeErr := wakeTelegramChannelOrderOwner(ctx, job); wakeErr != nil {
					g.Log().Warning(ctx, "旧消息已清理，提前唤醒频道顺序持有任务失败", g.Map{
						"event": "channel_queue_cleanup_wake_failed", "jobId": job.Id,
						"channelId": job.ChannelId, "error": wakeErr.Error(),
					})
				}
			}
			g.Log().Info(ctx, "TG超时清理事务完成", g.Map{
				"event": "timeout_cleanup_finished", "parentJobId": job.Id,
				"tailJobId": decision.TailID, "activeJobId": decision.ActiveID,
				"tenantId": job.TenantId, "accountId": job.AccountId,
				"profileId": job.ProfileId, "channelId": job.ChannelId,
				"operationNo": job.OperationNo, "action": decision.Action,
				"reason": decision.Reason, "conflictRetry": retry,
			})
			return nil
		}
		if retry != 0 || !telegramTimeoutActiveConflict(err) {
			return err
		}
		g.Log().Warning(ctx, "TG超时尾任务激活竞争，回滚后重新核对", g.Map{
			"event": "timeout_cleanup_active_conflict", "parentJobId": job.Id,
			"profileId": job.ProfileId, "channelId": job.ChannelId,
		})
	}
	return nil
}

func telegramTimeoutSameIdentity(parent, tail telegramJobRecord) bool {
	return parent.TenantId > 0 && parent.AccountId > 0 && parent.ProfileId > 0 &&
		parent.ChannelId > 0 && parent.OperationNo != "" &&
		parent.TenantId == tail.TenantId && parent.AccountId == tail.AccountId &&
		parent.ProfileId == tail.ProfileId && parent.ChannelId == tail.ChannelId &&
		parent.OperationNo == tail.OperationNo
}

func (s *sSysPublish) finishTelegramSendTimeoutCleanupTx(ctx context.Context, tx gdb.TX, snapshot telegramJobRecord) (decision telegramTimeoutTailDecision, resultErr error) {
	decision.Action = "snapshot_changed"
	parentModel := tx.Model(publishTgJobTable).Where("id", snapshot.Id)
	if snapshot.TimeoutCleanupAt == nil {
		parentModel = parentModel.WhereNull("timeout_cleanup_at")
	} else {
		parentModel = parentModel.Where("timeout_cleanup_at", snapshot.TimeoutCleanupAt)
	}
	var parent telegramJobRecord
	if err := parentModel.LockUpdate().Scan(&parent); !telegramOptionalAttemptScanSucceeded(err) {
		return decision, err
	}
	if parent.Id <= 0 {
		return decision, nil
	}
	timeoutParent := parent.Status == JobStatusSuperseded && parent.SendPhase == telegramSendPhaseTimeoutCancelled
	failedCleanup := parent.Status == JobStatusFailed &&
		parent.SendPhase == telegramSendPhaseTimeoutCancelled && snapshot.TimeoutCleanupAt != nil
	if !telegramTimeoutSameIdentity(parent, snapshot) || (!timeoutParent && !failedCleanup) {
		return decision, gerror.New("TG超时清理父任务身份或代次已变化")
	}
	// NULL cleanup timestamps do not prove deletion. Legacy/manual callers
	// must pass exactly the same evidence gates as leased cleanup snapshots.
	remaining, err := tx.Model(publishTgMessageTable).Where("job_id", parent.Id).
		WhereIn("status", []string{"sent", "undeletable"}).Count()
	if err != nil {
		return decision, err
	}
	if remaining > 0 {
		return decision, gerror.New("清理期间收到新消息，暂不重新入队")
	}
	unresolved, err := telegramTimeoutUnresolvedAttemptCount(tx, parent.Id)
	if err != nil {
		return decision, err
	}
	missing, err := telegramTimeoutUnprojectedAuditCount(tx, parent.Id)
	if err != nil {
		return decision, err
	}
	if missing > 0 {
		return decision, gerror.New("TG响应审计尚未投影到清理记录，停止自动重新上架")
	}
	if unresolved > 0 {
		return decision, gerror.New("旧发送仍缺少完整确认，已清理已知消息，等待对账或人工补偿；不自动重新上架")
	}
	var tail telegramJobRecord
	if err := tx.Model(publishTgJobTable).Where("timeout_parent_job_id", parent.Id).LockUpdate().Scan(&tail); !telegramOptionalAttemptScanSucceeded(err) {
		return decision, err
	}
	decision.TailID = tail.Id
	decision.Action = "no_waiting_tail"
	if timeoutParent && tail.Id > 0 && tail.Status == JobStatusFailed && tail.SendPhase == "timeout_wait_cleanup" {
		reason, activeID, err := telegramTimeoutTailEligibilityTx(ctx, tx, parent, tail)
		if err != nil {
			return decision, err
		}
		decision.ActiveID = activeID
		if reason == "" && snapshot.TimeoutCleanupAt == nil {
			// NULL is a legacy/completed marker, not a valid recovery claim.
			// It may only retire a stale tail after all evidence gates pass.
			reason = "missing_cleanup_claim"
		}
		if reason == "" {
			changed, err := s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{
				JobId: tail.Id, From: []JobStatus{JobStatusFailed}, To: JobStatusPending,
				SendPhase: jobPhase(""), Additional: g.Map{"next_retry_at": nil},
			})
			if err != nil {
				return decision, err // caller retries only the exact unique conflict
			}
			if changed {
				if err := s.ensureTelegramJobOutboxTx(ctx, tx, tail.Id, 0); err != nil {
					return decision, err
				}
				decision.Action = "activated"
			} else {
				// The locked row cannot change underneath us. A cancel fence
				// may have appeared after the explicit eligibility read.
				reason = "activation_cancel_fenced"
			}
		}
		if reason != "" {
			changed, err := s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{
				JobId: tail.Id, From: []JobStatus{JobStatusFailed}, To: JobStatusSuperseded,
				ErrorMessage: "超时尾任务不再允许自动上架：" + reason,
				SendPhase:    jobPhase(""),
				Additional:   g.Map{"next_retry_at": nil},
			})
			if err != nil {
				return decision, err
			}
			if !changed {
				return decision, gerror.New("超时尾任务收口未通过状态栅栏")
			}
			// Superseded is explicitly permitted by the unified cancel fence;
			// do not bypass it with a raw status write or touch the active job.
			if _, err := tx.Model(publishTgOutboxTable).Where("job_id", tail.Id).
				WhereIn("status", []string{telegramOutboxPending, telegramOutboxProcessing}).
				Data(g.Map{"status": telegramOutboxDelivered, "lease_owner": "", "lease_expires_at": nil,
					"last_error": reason, "delivered_at": gdb.Raw("CURRENT_TIMESTAMP")}).Update(); err != nil {
				return decision, err
			}
			decision.Action, decision.Reason = "superseded", reason
		}
	}
	// LockUpdate mutates the read model. Build a fresh write model in the
	// same transaction so SELECT's FOR UPDATE never reaches the UPDATE SQL.
	// Keep the cleanup generation predicate: a late receipt must fence this write.
	parentUpdate := tx.Model(publishTgJobTable).Where("id", snapshot.Id)
	if snapshot.TimeoutCleanupAt == nil {
		parentUpdate = parentUpdate.WhereNull("timeout_cleanup_at")
	} else {
		parentUpdate = parentUpdate.Where("timeout_cleanup_at", snapshot.TimeoutCleanupAt)
	}
	data := g.Map{"timeout_cleanup_at": nil}
	if failedCleanup {
		decision.Action = "terminal_failure_cleaned"
	}
	_, err = parentUpdate.Data(data).Update()
	return decision, err
}

func telegramTimeoutTailEligibilityTx(ctx context.Context, tx gdb.TX, parent, tail telegramJobRecord) (string, int64, error) {
	if !telegramTimeoutSameIdentity(parent, tail) || tail.TimeoutParentJobId != parent.Id {
		return "", 0, gerror.New("TG超时尾任务身份不匹配，保留原任务等待人工核查")
	}
	// Read, never LockUpdate: new publication takes profile_state before Job.
	// Taking those locks in reverse order here would deadlock the two paths.
	state, err := tx.Model(publishProfileStateTable).Where("tenant_id", parent.TenantId).
		Where("account_id", parent.AccountId).Where("profile_id", parent.ProfileId).
		WhereNull("deleted_at").Fields("publish_operation_no,publish_task_status").One()
	if err != nil {
		return "", 0, err
	}
	if state.IsEmpty() || state["publish_operation_no"].String() != parent.OperationNo {
		return "operation_changed", 0, nil
	}
	if status := state["publish_task_status"].String(); status == "canceling" || status == "canceled" || status == "cancelled" {
		return "operation_cancelled", 0, nil
	}
	cancelled, err := profileOperationCanceledTx(ctx, tx, parent)
	if err != nil {
		return "", 0, err
	}
	if cancelled {
		return "operation_cancelled", 0, nil
	}
	eligible, err := tx.Model("hg_content_profile p").InnerJoin(publishChannelTable+" c", "1=1").
		Where("p.id", parent.ProfileId).Where("p.status", 1).Where("p.visibility", "public").WhereNull("p.deleted_at").
		Where("c.id", parent.ChannelId).Where("c.tenant_id", parent.TenantId).
		Where("c.status", 1).WhereNull("c.deleted_at").Count()
	if err != nil {
		return "", 0, err
	}
	if eligible == 0 {
		return "profile_or_channel_disabled", 0, nil
	}
	// Match the unique index's global (profile_id,channel_id) scope exactly.
	// No active-row lock: the unique index remains the final concurrency fence.
	active, err := tx.Model(publishTgJobTable).Where("profile_id", tail.ProfileId).
		Where("channel_id", tail.ChannelId).Where("id <> ?", tail.Id).
		WhereIn("status", telegramJobActiveStatuses()).Fields("id").Limit(1).One()
	if err != nil {
		return "", 0, err
	}
	if !active.IsEmpty() {
		return "active_job_exists", active["id"].Int64(), nil
	}
	return "", 0, nil
}
