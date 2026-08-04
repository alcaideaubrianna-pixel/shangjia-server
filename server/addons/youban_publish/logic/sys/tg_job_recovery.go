package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const telegramSendingJobRecoverAfter = 2 * time.Minute

func (s *sSysPublish) recoverInterruptedTelegramJobs(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	now := gtime.Now()
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		WhereIn("dispatch_status", []string{tgDispatchStatusQueued, tgDispatchStatusProcessing}).
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
	return s.recoverStaleTelegramSendingJobs(ctx, limit)
}

func (s *sSysPublish) runTelegramJobRecovery(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	time.Sleep(15 * time.Second)
	if err := s.recoverStaleTelegramSendingJobs(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "恢复卡住的TG推送任务失败：%+v", err)
	}
	if err := s.recoverMissingProfilePublishOperationStates(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "补建资料上架状态失败：%+v", err)
	}
	if err := s.recoverProfilePublishOperationStates(ctx, 100); err != nil {
		g.Log().Warningf(ctx, "恢复资料上架状态失败：%+v", err)
	}
	if err := s.dispatchTelegramDueJobs(ctx, g.Cfg().MustGet(ctx, "youbanPublish.queue.schedulerBatchSize", 50).Int()); err != nil {
		g.Log().Warningf(ctx, "调度待恢复TG推送任务失败：%+v", err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.recoverStaleTelegramSendingJobs(ctx, 100); err != nil {
				g.Log().Warningf(ctx, "恢复卡住的TG推送任务失败：%+v", err)
			}
			if err := s.recoverMissingProfilePublishOperationStates(ctx, 100); err != nil {
				g.Log().Warningf(ctx, "补建资料上架状态失败：%+v", err)
			}
			if err := s.recoverProfilePublishOperationStates(ctx, 100); err != nil {
				g.Log().Warningf(ctx, "恢复资料上架状态失败：%+v", err)
			}
			if err := s.dispatchTelegramDueJobs(ctx, g.Cfg().MustGet(ctx, "youbanPublish.queue.schedulerBatchSize", 50).Int()); err != nil {
				g.Log().Warningf(ctx, "调度待恢复TG推送任务失败：%+v", err)
			}
		}
	}
}

func (s *sSysPublish) recoverStaleTelegramSendingJobs(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	deadline := gtime.Now().Add(-telegramSendingJobRecoverAfter)
	var jobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("status", "sending").
		WhereLTE("updated_at", deadline).
		OrderAsc("updated_at").
		Limit(limit).
		Scan(&jobs)
	if err != nil {
		return gerror.Wrap(err, "读取卡住的TG推送任务失败")
	}
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
