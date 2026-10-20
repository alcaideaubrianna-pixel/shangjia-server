package sys

import (
	"context"
	"fmt"
	relayconsts "hotgo/addons/youban_publish/consts"
	"sync"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/utility/dbtime"
)

const (
	telegramSendBudget                = 10 * time.Minute
	telegramSendMaxRetries            = 3
	telegramSendPhaseTimeoutCancelled = "timeout_cancelled"
	telegramSendTimeoutMessage        = "频道发送超过10分钟，旧任务已取消，整组资料移至队尾；迟到消息只删除不补发"
)

// telegramSendBudgetContext uses a durable deadline shared by every retry and
// Account worker. Heartbeats and queue handoffs never extend the budget.
func (s *sSysPublish) telegramSendBudgetContext(ctx context.Context, job telegramJobRecord) (context.Context, context.CancelFunc, error) {
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).
		WhereIn("status", []string{telegramJobStatusSending, telegramJobStatusConfirming}).
		WhereNull("send_deadline_at").Data(g.Map{"send_deadline_at": gtime.Now().Add(telegramSendBudget)}).Update()
	if err != nil {
		return nil, nil, gerror.Wrap(err, "保存频道发送超时预算失败")
	}
	current, err := s.telegramJobById(ctx, job.Id)
	if err != nil {
		return nil, nil, err
	}
	if current.Status == telegramJobStatusSuperseded || current.SendDeadlineAt == nil {
		return nil, nil, errTelegramJobSuperseded
	}
	deadline := dbtime.FromTimestamp(current.SendDeadlineAt)
	if !deadline.Time.After(time.Now()) {
		return nil, nil, gerror.New(telegramSendTimeoutMessage)
	}
	bounded, cancel := context.WithDeadline(ctx, deadline.Time)
	return bounded, cancel, nil
}

// expireTelegramSendTx fences the old sender and creates exactly one tail job
// together with its Outbox. No external I/O occurs while holding the row lock.
func (s *sSysPublish) expireTelegramSendTx(ctx context.Context, tx gdb.TX, id int64) (old telegramJobRecord, replacement int64, err error) {
	if err = tx.Model(publishTgJobTable).Where("id", id).LockUpdate().Scan(&old); err != nil {
		return
	}
	if old.Id <= 0 || old.SendDeadlineAt == nil || dbtime.FromTimestamp(old.SendDeadlineAt).Time.After(time.Now()) ||
		(old.Status != telegramJobStatusPending && old.Status != telegramJobStatusSending && old.Status != telegramJobStatusRetryWait && old.Status != telegramJobStatusConfirming) {
		return
	}
	// A complete, authoritative response needs only local projection repair.
	if old.SendPhase == telegramSendPhaseVerifyConfirmed || old.SendPhase == telegramSendPhaseCompletedNoVerify {
		return
	}
	if old.SendPhase == relayconsts.RelaySendPhaseSending {
		_, err = s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{JobId: old.Id, From: []string{old.Status}, To: telegramJobStatusFailed, ErrorMessage: "整包复制超过核实时限，已停止自动补发，请核对目标频道"})
		return
	}
	changed, transitionErr := s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{
		JobId: old.Id, From: []string{old.Status}, To: telegramJobStatusSuperseded,
		ErrorMessage: telegramSendTimeoutMessage,
		Additional:   g.Map{"send_phase": telegramSendPhaseTimeoutCancelled, "timeout_cleanup_at": gtime.Now()},
	})
	if transitionErr != nil || !changed {
		err = transitionErr
		return
	}
	if _, err = tx.Model(publishTgAttemptTable).Where("job_id", old.Id).
		WhereIn("status", []string{telegramAttemptStatusSending, telegramAttemptStatusWaiting, telegramAttemptStatusRetryWait}).
		Data(g.Map{"status": telegramAttemptStatusSuperseded, "webhook_deadline": nil}).Update(); err != nil {
		return
	}
	if _, err = tx.Model(publishTgOutboxTable).Where("job_id", old.Id).
		Data(g.Map{"status": telegramOutboxDelivered, "lease_owner": "", "lease_expires_at": nil,
			"last_error": telegramSendTimeoutMessage, "delivered_at": gtime.Now()}).Update(); err != nil {
		return
	}
	state, stateErr := tx.Model(publishProfileStateTable).Where("tenant_id", old.TenantId).
		Where("account_id", old.AccountId).Where("profile_id", old.ProfileId).
		Where("publish_operation_no", old.OperationNo).WhereNull("deleted_at").One()
	if stateErr != nil {
		err = stateErr
		return
	}
	if state.IsEmpty() {
		return
	}
	eligible, eligibleErr := tx.Model("hg_content_profile p").
		InnerJoin(publishChannelTable+" c", "1=1").Where("c.id", old.ChannelId).Where("c.tenant_id", old.TenantId).
		Where("p.id", old.ProfileId).Where("p.status", 1).Where("p.visibility", "public").WhereNull("p.deleted_at").
		Where("c.status", 1).WhereNull("c.deleted_at").Count()
	if eligibleErr != nil {
		err = eligibleErr
		return
	}
	if eligible == 0 {
		return
	}
	if old.TimeoutRetryCount >= telegramSendMaxRetries {
		_, err = tx.Model(publishTgJobTable).Where("id", old.Id).
			Data(g.Map{"error_message": "发送超时重试已达3次，停止自动重新上架，等待人工处理"}).Update()
		return
	}
	// Retain the original operation and collection provenance. The fresh ID and
	// creation time place the whole publication behind all existing channel jobs.
	data := g.Map{
		"operation_no": old.OperationNo, "tenant_id": old.TenantId, "merchant_id": old.TenantId,
		"account_id": old.AccountId, "profile_id": old.ProfileId, "channel_id": old.ChannelId,
		"bot_id": old.BotId, "push_mode": old.PushMode, "target_chat_id": old.TargetChatId,
		"collect_event_id": old.CollectEventId, "collect_source_id": old.CollectSourceId,
		"collect_source_chat_id": old.CollectSourceChatId, "collect_source_message_id": old.CollectSourceMessageId,
		"status": telegramJobStatusFailed, "dispatch_status": tgDispatchStatusIdle, "priority": 0,
		"send_phase":            "timeout_wait_cleanup",
		"timeout_parent_job_id": old.Id,
		"timeout_retry_count":   old.TimeoutRetryCount + 1,
		"cycle_enabled":         old.CycleEnabled, "cycle_days": old.CycleDays, "cycle_publish_time": old.CyclePublishTime,
	}
	// Preserve NULL task IDs: zero participates in a different unique index.
	taskID, taskErr := tx.Model(publishTgJobTable).Where("id", old.Id).Fields("task_id").Value()
	if taskErr != nil {
		err = taskErr
		return
	}
	if !taskID.IsNil() {
		data["task_id"] = taskID.Int64()
	}
	replacement, err = tx.Model(publishTgJobTable).Data(data).InsertAndGetId()
	if err == nil {
		// Keep the replacement unscheduled until deletion succeeds. The cleaner
		// owns the durable handoff and survives process restart.
		_, err = tx.Model(publishTgJobTable).Where("id", replacement).
			Data(g.Map{"next_retry_at": gtime.Now().Add(24 * time.Hour)}).Update()
	}
	return
}

func (s *sSysPublish) runTelegramSendWatchdog(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.recoverTelegramSendTimeouts(ctx)
			s.cleanupTelegramSendTimeouts(ctx)
		}
	}
}

var telegramTimeoutCleanupClaimSQL = `WITH candidates AS (
 SELECT id FROM ` + publishTgJobTable + ` WHERE send_phase='timeout_cancelled'
 AND timeout_cleanup_at<=? ORDER BY timeout_cleanup_at,id
 LIMIT 40 FOR UPDATE SKIP LOCKED
 ) UPDATE ` + publishTgJobTable + ` j SET timeout_cleanup_at=?
 FROM candidates c WHERE j.id=c.id RETURNING j.*`

func (s *sSysPublish) cleanupTelegramSendTimeouts(ctx context.Context) {
	var jobs []telegramJobRecord
	// Claim a bounded batch across replicas; a crashed worker releases it by expiry.
	now := gtime.Now()
	if err := g.DB().GetScan(ctx, &jobs, telegramTimeoutCleanupClaimSQL, now, now.Add(2*time.Minute)); err != nil {
		g.Log().Warningf(ctx, "TG超时清理扫描失败 err:%+v", err)
		return
	}
	var workers sync.WaitGroup
	slots := make(chan struct{}, 8)
	for _, job := range jobs {
		if ctx.Err() != nil {
			break
		}
		slots <- struct{}{}
		workers.Add(1)
		go func(job telegramJobRecord) {
			defer workers.Done()
			defer func() { <-slots }()
			s.cleanupTelegramSendTimeout(ctx, job)
		}(job)
	}
	workers.Wait()
}

func (s *sSysPublish) cleanupTelegramSendTimeout(ctx context.Context, job telegramJobRecord) {
	cleanupCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	// Do not spend all cleanup workers waiting for a busy in-process channel lock.
	lock := s.telegramChannelLock(job.TargetChatId)
	var err error
	if !lock.TryLock() {
		err = gerror.New("频道正在处理其他操作，稍后重试清理")
	} else {
		err = s.withTelegramChannelDBLock(cleanupCtx, telegramChannelLockKey(job.TargetChatId), func() error {
			return s.deleteTelegramMessageSetLockedByChannel(cleanupCtx, job, telegramSendTimeoutMessage)
		})
		lock.Unlock()
	}
	cancel()
	if err == nil {
		remaining, countErr := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
			Where("job_id", job.Id).WhereIn("status", []string{"sent", "undeletable"}).Count()
		if countErr != nil {
			err = countErr
		} else if remaining > 0 {
			err = gerror.New("旧消息尚未确认删除，等待协议号删除完成")
		}
	}
	if err != nil {
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).
			Where("timeout_cleanup_at", job.TimeoutCleanupAt).Data(g.Map{"timeout_cleanup_at": gtime.Now().Add(time.Minute)}).Update()
		g.Log().Warningf(ctx, "TG超时清理失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, err)
		return
	}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// A late response changes this timestamp. Do not activate the replacement
		// from an obsolete cleanup snapshot; the next pass must delete it first.
		current, err := tx.Model(publishTgJobTable).Where("id", job.Id).
			Where("timeout_cleanup_at", job.TimeoutCleanupAt).LockUpdate().One()
		if err != nil {
			return err
		}
		if current.IsEmpty() {
			return nil
		}
		remaining, err := tx.Model(publishTgMessageTable).Where("job_id", job.Id).
			WhereIn("status", []string{"sent", "undeletable"}).Count()
		if err != nil {
			return err
		}
		if remaining > 0 {
			return gerror.New("清理期间收到新消息，暂不重新入队")
		}
		unresolved, err := telegramTimeoutUnresolvedAttemptCount(tx, job.Id)
		if err != nil {
			return err
		}
		if unresolved > 0 {
			return gerror.New("旧发送仍缺少完整确认，已清理已知消息，等待对账或人工补偿；不自动重新上架")
		}
		var tail telegramJobRecord
		if err := tx.Model(publishTgJobTable).Where("timeout_parent_job_id", job.Id).LockUpdate().Scan(&tail); !telegramOptionalAttemptScanSucceeded(err) {
			return err
		}
		if tail.Id > 0 && tail.Status == telegramJobStatusFailed && tail.SendPhase == "timeout_wait_cleanup" {
			if _, err := s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{JobId: tail.Id, From: []string{telegramJobStatusFailed}, To: telegramJobStatusPending, Additional: g.Map{"next_retry_at": nil, "send_phase": ""}}); err != nil {
				return err
			}
			if err := s.ensureTelegramJobOutboxTx(ctx, tx, tail.Id, 0); err != nil {
				return err
			}
		}
		_, err = tx.Model(publishTgJobTable).Where("id", job.Id).Where("timeout_cleanup_at", job.TimeoutCleanupAt).Data(g.Map{"timeout_cleanup_at": nil}).Update()
		return err
	})
	if err != nil {
		g.Log().Warningf(ctx, "TG超时清理后重新入队失败 jobId:%d err:%+v", job.Id, err)
	}
}

func telegramTimeoutUnresolvedAttemptCount(tx gdb.TX, jobID int64) (int, error) {
	var attempts []telegramDeliveryAttempt
	if err := tx.Model(publishTgAttemptTable).
		Fields("id,status,response_received,confirmed_count,error_message").
		Where("job_id", jobID).Where("status", telegramAttemptStatusSuperseded).
		Where("response_received", 0).Scan(&attempts); err != nil {
		return 0, err
	}
	unresolved := 0
	for _, attempt := range attempts {
		// Reuse the retry classifier: a retired, definite rejection with no
		// delivery evidence is not an outstanding remote send.
		if !telegramSupersededAttemptCanResume(attempt, 0) {
			unresolved++
			continue
		}
		auditCount, err := tx.Model(publishTgAttemptMessageTable).Where("attempt_id", attempt.Id).Count()
		if err != nil {
			return 0, err
		}
		if auditCount > 0 {
			unresolved++
		}
	}
	return unresolved, nil
}

func (s *sSysPublish) recoverTelegramSendTimeouts(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	// Bounded compatibility backfill for senders created by older releases.
	// Waiting, never-started publications do not acquire a deadline.
	_, backfillErr := g.DB().Exec(ctx, `WITH candidates AS (
 SELECT id FROM `+publishTgJobTable+`
 WHERE send_deadline_at IS NULL
 AND (status IN ('sending','unknown') OR
      (status='failed_retry' AND send_phase IN ('cleanup_processing','display_sending','display_confirmed','verify_sending')))
 ORDER BY id LIMIT 500 FOR UPDATE SKIP LOCKED
 ) UPDATE `+publishTgJobTable+` j
 SET send_deadline_at=COALESCE((SELECT MIN(a.created_at) FROM `+publishTgAttemptTable+` a WHERE a.job_id=j.id),j.updated_at,j.created_at,CURRENT_TIMESTAMP::timestamp)+INTERVAL '10 minutes'
 FROM candidates c WHERE j.id=c.id AND j.send_deadline_at IS NULL`)
	if backfillErr != nil {
		g.Log().Warningf(ctx, "TG历史发送截止时间回填失败 err:%+v", backfillErr)
		return
	}
	var jobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("status", []string{telegramJobStatusPending, telegramJobStatusSending, telegramJobStatusRetryWait, telegramJobStatusConfirming}).
		WhereLTE("send_deadline_at", gtime.Now()).
		WhereNotIn("send_phase", []string{telegramSendPhaseVerifyConfirmed, telegramSendPhaseCompletedNoVerify}).
		OrderAsc("send_deadline_at").Limit(100).Scan(&jobs)
	if err != nil {
		g.Log().Errorf(ctx, "TG发送超时看门狗查询失败 action:watchdog_scan err:%+v", err)
		return
	}
	for _, job := range jobs {
		var old telegramJobRecord
		var replacement int64
		err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			var expireErr error
			old, replacement, expireErr = s.expireTelegramSendTx(ctx, tx, job.Id)
			return expireErr
		})
		if err != nil {
			g.Log().Errorf(ctx, "TG超时任务隔离失败 jobId:%d channelId:%d action:timeout_fence err:%+v", job.Id, job.ChannelId, err)
			continue
		}
		if replacement > 0 {
			s.appendTelegramJobLog(ctx, old, "timeout", "superseded", fmt.Sprintf("%s replacementJobId:%d", telegramSendTimeoutMessage, replacement))
			g.Log().Warningf(ctx, "TG频道超时已隔离 jobId:%d replacementJobId:%d channelId:%d phase:%s action:timeout_requeue", old.Id, replacement, old.ChannelId, old.SendPhase)
			if err = s.wakeNextTelegramChannelJob(ctx, old); err != nil {
				g.Log().Errorf(ctx, "TG超时释放后唤醒频道失败 jobId:%d channelId:%d err:%+v", old.Id, old.ChannelId, err)
			}
		} else if old.TimeoutRetryCount >= telegramSendMaxRetries && old.Status != telegramJobStatusSuperseded {
			if err := s.updateProfilePublishOperationState(ctx, old, sysin.PublishTaskStatusFailed); err != nil {
				g.Log().Warningf(ctx, "TG超时重试耗尽状态回写失败 jobId:%d err:%+v", old.Id, err)
			}
			g.Log().Warningf(ctx, "TG发送超时重试已达上限 jobId:%d channelId:%d retryCount:%d action:manual_repair", old.Id, old.ChannelId, old.TimeoutRetryCount)
			if err := s.wakeNextTelegramChannelJob(ctx, old); err != nil {
				g.Log().Warningf(ctx, "TG重试耗尽后唤醒频道失败 jobId:%d err:%+v", old.Id, err)
			}
		}
	}
}
