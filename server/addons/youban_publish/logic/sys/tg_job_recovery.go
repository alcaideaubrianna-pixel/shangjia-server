package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const (
	telegramSendingJobRecoverAfter  = telegramPublishTaskTimeout + 2*time.Minute
	telegramPendingJobRecoverAfter  = 30 * time.Second
	telegramDispatchJobRecoverAfter = 5 * time.Minute
	messagePushPlanJobExpireAfter   = 30 * time.Minute
)

const expiredMessagePushPlanJobMessage = "历史消息计划任务已过期，系统自动终止"

func (s *sSysPublish) recoverInterruptedTelegramJobs(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	now := gtime.Now()
	if err := s.supersedeExpiredMessagePushPlanJobs(ctx, limit); err != nil {
		return err
	}
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		WhereIn("dispatch_status", []string{tgDispatchStatusQueued, tgDispatchStatusProcessing}).
		Where(telegramActiveChannelCondition()).
		Where(messagePushPlanRecoveryConditionSQL(publishTgJobTable), telegramRecoveryTimeText(now.Add(-messagePushPlanJobExpireAfter))).
		Data(g.Map{
			"dispatch_status":     tgDispatchStatusIdle,
			"next_retry_at":       now,
			"last_dispatch_error": "服务启动时发现任务调度中断，已重新进入待调度队列",
			"updated_at":          now,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "恢复调度中断的TG任务失败")
	}
	if affected, _ := result.RowsAffected(); affected > 0 {
		g.Log().Infof(ctx, "已恢复调度中断的TG任务：%d条", affected)
	}
	if err = s.recoverPendingIdleTelegramJobs(ctx, limit); err != nil {
		return err
	}
	if err = s.recoverStaleTelegramDispatchJobs(ctx, limit); err != nil {
		return err
	}
	return s.recoverStaleTelegramSendingJobs(ctx, limit)
}

func (s *sSysPublish) runTelegramJobRecovery(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	time.Sleep(15 * time.Second)
	if err := s.supersedeExpiredMessagePushPlanJobs(ctx, 1000); err != nil {
		g.Log().Warningf(ctx, "终止过期历史消息计划任务失败：%+v", err)
	}
	if err := s.recoverStaleTelegramSendingJobs(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "恢复卡住的TG推送任务失败：%+v", err)
	}
	if err := s.requeueExpiredTelegramAttempts(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "重新投递过期TG发送Attempt失败：%+v", err)
	}
	if err := s.recoverTerminalTelegramAttempts(ctx, 500); err != nil {
		g.Log().Warningf(ctx, "收敛TG发送Attempt状态失败：%+v", err)
	}
	if err := s.recoverPendingIdleTelegramJobs(ctx, 500); err != nil {
		g.Log().Warningf(ctx, "恢复待入队TG推送任务失败：%+v", err)
	}
	if err := s.recoverStaleTelegramDispatchJobs(ctx, 200); err != nil {
		g.Log().Warningf(ctx, "恢复调度中断TG推送任务失败：%+v", err)
	}
	if err := s.recoverMissingProfilePublishOperationStates(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "补建资料上架状态失败：%+v", err)
	}
	if err := s.recoverProfilePublishOperationStates(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "恢复资料上架状态失败：%+v", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.requeueExpiredTelegramAttempts(ctx, 100); err != nil {
				g.Log().Warningf(ctx, "重新投递过期TG发送Attempt失败：%+v", err)
			}
			if err := s.recoverTerminalTelegramAttempts(ctx, 500); err != nil {
				g.Log().Warningf(ctx, "收敛TG发送Attempt状态失败：%+v", err)
			}
			if err := s.supersedeExpiredMessagePushPlanJobs(ctx, 1000); err != nil {
				g.Log().Warningf(ctx, "终止过期历史消息计划任务失败：%+v", err)
			}
			if err := s.recoverPendingIdleTelegramJobs(ctx, 500); err != nil {
				g.Log().Warningf(ctx, "恢复待入队TG推送任务失败：%+v", err)
			}
			if err := s.recoverStaleTelegramDispatchJobs(ctx, 200); err != nil {
				g.Log().Warningf(ctx, "恢复调度中断TG推送任务失败：%+v", err)
			}
			if err := s.recoverStaleTelegramSendingJobs(ctx, 100); err != nil {
				g.Log().Warningf(ctx, "恢复卡住的TG推送任务失败：%+v", err)
			}
			if err := s.recoverMissingProfilePublishOperationStates(ctx, 100); err != nil {
				g.Log().Warningf(ctx, "补建资料上架状态失败：%+v", err)
			}
			if err := s.recoverProfilePublishOperationStates(ctx, 100); err != nil {
				g.Log().Warningf(ctx, "恢复资料上架状态失败：%+v", err)
			}
		}
	}
}

func (s *sSysPublish) requeueExpiredTelegramAttempts(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	var ids []int64
	err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Fields("id").Where("status", telegramAttemptStatusWaiting).
		WhereLTE("webhook_deadline", gtime.Now().Add(-5*time.Second)).
		OrderAsc("webhook_deadline").OrderAsc("id").Limit(limit).Scan(&ids)
	if err != nil {
		return gerror.Wrap(err, "读取过期TG发送Attempt失败")
	}
	queued := 0
	for _, attemptId := range ids {
		if err = s.enqueueTelegramAttemptTimeout(ctx, attemptId, 0); err != nil {
			g.Log().Warningf(ctx, "重新投递过期TG发送Attempt失败 attemptId:%d err:%+v", attemptId, err)
			continue
		}
		queued++
	}
	if queued > 0 {
		g.Log().Infof(ctx, "已重新投递过期TG发送Attempt：%d条", queued)
	}
	return nil
}

func (s *sSysPublish) recoverTerminalTelegramAttempts(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	var ids []int64
	err := g.DB().Model(publishTgAttemptTable+" attempt").Safe().Ctx(ctx).
		InnerJoin(publishTgJobTable+" job", "job.id=attempt.job_id").
		Fields("attempt.id").
		WhereIn("attempt.status", []string{telegramAttemptStatusSending, telegramAttemptStatusWaiting, telegramAttemptStatusRetryWait}).
		WhereIn("job.status", []string{"sent", "failed", "superseded"}).
		OrderAsc("attempt.id").Limit(limit).Scan(&ids)
	if err != nil {
		return gerror.Wrap(err, "读取终态TG任务遗留Attempt失败")
	}
	if len(ids) == 0 {
		return nil
	}
	result, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		WhereIn("status", []string{telegramAttemptStatusSending, telegramAttemptStatusWaiting, telegramAttemptStatusRetryWait}).
		Data(g.Map{"status": telegramAttemptStatusSuperseded, "webhook_deadline": nil, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "收敛终态TG任务遗留Attempt失败")
	}
	if affected, _ := result.RowsAffected(); affected > 0 {
		g.Log().Infof(ctx, "已收敛终态TG任务遗留Attempt：%d条", affected)
	}
	return nil
}

func (s *sSysPublish) recoverStaleTelegramDispatchJobs(ctx context.Context, limit int) error {
	startedAt := time.Now()
	scanned := 0
	var observeErr error
	defer func() { observeRecoveryRun(ctx, "stale_dispatch", startedAt, scanned, observeErr) }()
	if limit <= 0 {
		limit = 100
	}
	now := gtime.Now()
	deadline := telegramRecoveryTimeText(now.Add(-telegramDispatchJobRecoverAfter))
	var jobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		WhereIn("dispatch_status", []string{tgDispatchStatusQueued, tgDispatchStatusProcessing}).
		Where(telegramActiveChannelCondition()).
		Where(messagePushPlanRecoveryConditionSQL(publishTgJobTable), telegramRecoveryTimeText(now.Add(-messagePushPlanJobExpireAfter))).
		WhereLTE("dispatched_at", deadline).
		OrderAsc("dispatched_at").OrderAsc("id").
		Limit(limit).
		Scan(&jobs)
	if err != nil {
		observeErr = err
		return gerror.Wrap(err, "读取调度中断TG推送任务失败")
	}
	scanned = len(jobs)
	for _, job := range jobs {
		if job.Id <= 0 {
			continue
		}
		result, updateErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("id", job.Id).
			WhereIn("dispatch_status", []string{tgDispatchStatusQueued, tgDispatchStatusProcessing}).
			Data(g.Map{
				"dispatch_status":     tgDispatchStatusIdle,
				"next_retry_at":       gtime.Now(),
				"last_dispatch_error": "TG任务调度超时，已自动恢复入队",
				"updated_at":          gtime.Now(),
			}).Update()
		if updateErr != nil {
			g.Log().Warningf(ctx, "重置调度中断TG任务失败 job:%d err:%+v", job.Id, updateErr)
			continue
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			continue
		}
		if enqueueErr := s.enqueueTelegramJob(ctx, job.Id, 0); enqueueErr != nil {
			g.Log().Warningf(ctx, "重新入队调度中断TG任务失败 job:%d err:%+v", job.Id, enqueueErr)
		}
	}
	return nil
}

func (s *sSysPublish) recoverPendingIdleTelegramJobs(ctx context.Context, limit int) error {
	startedAt := time.Now()
	scanned := 0
	var observeErr error
	defer func() { observeRecoveryRun(ctx, "pending_idle", startedAt, scanned, observeErr) }()
	if limit <= 0 {
		limit = 100
	}
	now := gtime.Now()
	nowText := telegramRecoveryTimeText(now)
	deadline := telegramRecoveryTimeText(now.Add(-telegramPendingJobRecoverAfter))
	var jobs []telegramJobRecord
	query := `WITH eligible_jobs AS (
		SELECT j.*, ROW_NUMBER() OVER (
			PARTITION BY j.tenant_id, j.channel_id
			ORDER BY ` + telegramJobEffectivePrioritySQL("j") + ` ASC, j.id ASC
		) AS channel_rank
		FROM ` + publishTgJobTable + ` j
		WHERE j.status IN ('pending', 'failed_retry', 'unknown')
			AND (j.dispatch_status = ? OR j.dispatch_status = '')
			AND ` + telegramActiveChannelConditionForAlias("j") + `
			AND ` + messagePushPlanRecoveryConditionSQL("j") + `
			AND (j.next_retry_at IS NULL OR j.next_retry_at <= ?)
			AND j.updated_at <= ?
	)
	SELECT * FROM eligible_jobs
	WHERE channel_rank <= ?
	ORDER BY updated_at ASC, id ASC
	LIMIT ?`
	err := g.DB().GetScan(ctx, &jobs, query, tgDispatchStatusIdle, telegramRecoveryTimeText(now.Add(-messagePushPlanJobExpireAfter)), nowText, deadline, telegramChannelActiveJobLimit, limit)
	if err != nil {
		observeErr = err
		return gerror.Wrap(err, "读取待入队TG推送任务失败")
	}
	scanned = len(jobs)
	if len(jobs) > 0 {
		g.Log().Infof(ctx, "恢复待入队TG推送任务：%d条", len(jobs))
	}
	for _, job := range jobs {
		if job.Id <= 0 {
			continue
		}
		if err = s.enqueueTelegramJob(ctx, job.Id, 0); err != nil {
			g.Log().Warningf(ctx, "恢复待入队TG推送任务失败 job:%d err:%+v", job.Id, err)
		}
	}
	return nil
}

func telegramRecoveryTimeText(value *gtime.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("Y-m-d H:i:s")
}

func (s *sSysPublish) telegramJobChannelIsActive(ctx context.Context, job telegramJobRecord) (bool, error) {
	if job.ChannelId <= 0 || job.TenantId <= 0 {
		return false, nil
	}
	if isMessagePushOperationNo(job.OperationNo) {
		if job.AccountId <= 0 || job.TargetChatId == "" {
			return false, nil
		}
		count, err := g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
			Where("id", job.ChannelId).
			Where("tenant_id", job.TenantId).
			Where("tg_account_id", job.AccountId).
			Where("REPLACE(channel_id, '-100', '') = REPLACE(?, '-100', '')", normalizeTelegramChannelChatID(job.TargetChatId)).
			Count()
		if err != nil {
			return false, gerror.Wrap(err, "检查快速推送目标频道失败")
		}
		return count > 0, nil
	}
	count, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("id", job.ChannelId).
		Where("tenant_id", job.TenantId).
		Where("status", 1).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查TG任务目标频道失败")
	}
	return count > 0, nil
}

func (s *sSysPublish) recoverStaleTelegramSendingJobs(ctx context.Context, limit int) error {
	startedAt := time.Now()
	scanned := 0
	var observeErr error
	defer func() { observeRecoveryRun(ctx, "stale_sending", startedAt, scanned, observeErr) }()
	if limit <= 0 {
		limit = 100
	}
	now := gtime.Now()
	deadline := telegramRecoveryTimeText(now.Add(-telegramSendingJobRecoverAfter))
	var jobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("status", "sending").
		Where(telegramActiveChannelCondition()).
		Where(messagePushPlanRecoveryConditionSQL(publishTgJobTable), telegramRecoveryTimeText(now.Add(-messagePushPlanJobExpireAfter))).
		WhereLTE("updated_at", deadline).
		OrderAsc("updated_at").
		Limit(limit).
		Scan(&jobs)
	if err != nil {
		observeErr = err
		return gerror.Wrap(err, "读取卡住的TG推送任务失败")
	}
	scanned = len(jobs)
	for _, job := range jobs {
		if job.Id <= 0 {
			continue
		}
		if err = s.requeueStaleTelegramSendingJob(ctx, job); err != nil {
			g.Log().Warningf(ctx, "恢复卡住的TG推送任务失败 job:%d err:%+v", job.Id, err)
		}
	}
	return nil
}

func (s *sSysPublish) supersedeExpiredMessagePushPlanJobs(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	deadline := telegramRecoveryTimeText(gtime.Now().Add(-messagePushPlanJobExpireAfter))
	var ids []int64
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("id").
		Where("operation_no LIKE ?", "message_push_plan:%").
		WhereIn("status", []string{"pending", "sending", "failed_retry", "unknown"}).
		WhereLT("created_at", deadline).
		OrderAsc("id").
		Limit(limit).
		Scan(&ids); err != nil {
		return gerror.Wrap(err, "读取过期历史消息计划任务失败")
	}
	if len(ids) == 0 {
		return nil
	}
	now := gtime.Now()
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		WhereIn("status", []string{"pending", "sending", "failed_retry", "unknown"}).
		Data(g.Map{
			"status":              "superseded",
			"dispatch_status":     tgDispatchStatusDone,
			"next_retry_at":       nil,
			"error_message":       expiredMessagePushPlanJobMessage,
			"last_dispatch_error": expiredMessagePushPlanJobMessage,
			"updated_at":          now,
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "终止过期历史消息计划任务失败")
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		g.Log().Infof(ctx, "已终止过期历史消息计划任务：%d条", affected)
	}
	return nil
}

func messagePushPlanRecoveryConditionSQL(jobTable string) string {
	return "NOT (" + jobTable + ".operation_no LIKE 'message_push_plan:%' AND " + jobTable + ".created_at < ?)"
}

func telegramActiveChannelCondition() string {
	return telegramActiveChannelConditionForAlias(publishTgJobTable)
}

func telegramActiveChannelConditionForAlias(jobTable string) string {
	publishChannelCondition := "EXISTS (SELECT 1 FROM " + publishChannelTable + " c WHERE c.id=" + jobTable + ".channel_id AND c.tenant_id=" + jobTable + ".tenant_id AND c.status=1 AND c.deleted_at IS NULL)"
	messagePushChannelCondition := "((" + jobTable + ".operation_no LIKE 'message_push:%' OR " + jobTable + ".operation_no LIKE 'message_push_plan:%') AND EXISTS (SELECT 1 FROM " + publishTgChannelTable + " tc WHERE tc.id=" + jobTable + ".channel_id AND tc.tenant_id=" + jobTable + ".tenant_id AND tc.tg_account_id=" + jobTable + ".account_id AND REPLACE(tc.channel_id, '-100', '') = REPLACE(" + jobTable + ".target_chat_id, '-100', '')))"
	return "(" + publishChannelCondition + " OR " + messagePushChannelCondition + ")"
}

func (s *sSysPublish) requeueStaleTelegramSendingJob(ctx context.Context, job telegramJobRecord) error {
	now := gtime.Now()
	status := "failed_retry"
	message := "TG推送任务长时间处于发送中，已自动重新投递"
	nextRetryAt := any(nil)
	reconcileCount := job.ReconcileCount
	if job.SendPhase == telegramSendPhaseDisplaySending || job.SendPhase == telegramSendPhaseVerifySending {
		status = "unknown"
		message = "服务中断时TG发送结果不确定，已进入频道消息对账"
		nextRetryAt = now.Add(telegramUnknownReconcileDelay)
		reconcileCount = 0
	}
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		Where("status", "sending").
		WhereLTE("updated_at", now.Add(-telegramSendingJobRecoverAfter)).
		Data(g.Map{
			"status":          status,
			"dispatch_status": tgDispatchStatusIdle,
			"next_retry_at":   nextRetryAt,
			"error_message":   message,
			"reconcile_count": reconcileCount,
			"updated_at":      now,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "重置卡住的TG推送任务失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	s.appendTelegramJobLog(ctx, job, "publish", status, message)
	if err = s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusPending); err != nil {
		return err
	}
	return s.enqueueTelegramJob(ctx, job.Id, 0)
}
