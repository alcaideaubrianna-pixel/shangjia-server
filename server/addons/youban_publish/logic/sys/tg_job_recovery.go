package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const telegramSendingJobRecoverAfter = 5 * time.Minute

func (s *sSysPublish) runTelegramJobRecovery(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	time.Sleep(15 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.recoverStaleTelegramSendingJobs(ctx, 100); err != nil {
				g.Log().Warningf(ctx, "恢复卡住的TG推送任务失败：%+v", err)
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
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		Where("status", "sending").
		WhereLTE("updated_at", now.Add(-telegramSendingJobRecoverAfter)).
		Data(g.Map{
			"status":        "failed_retry",
			"next_retry_at": nil,
			"error_message": "TG推送任务长时间处于发送中，已自动重新投递",
			"updated_at":    now,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "重置卡住的TG推送任务失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	_, _ = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", job.TaskId).
		Where("status", "publishing").
		Data(g.Map{"tg_status": "pending", "updated_at": now}).
		Update()
	s.appendTelegramJobLog(ctx, job, "publish", "requeued", "TG推送任务长时间处于发送中，已自动重新投递")
	return s.enqueueTelegramJob(ctx, job.Id, 0)
}
