package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) DeleteTelegramJobMessages(ctx context.Context, jobId int64) error {
	job, err := s.telegramJobById(ctx, jobId)
	if err != nil {
		return err
	}
	s.appendTelegramJobLog(ctx, job, "cycle_delete", "skipped", "旧循环上架队列已废弃，改由循环计划执行")
	_ = s.disableTelegramJobCycle(ctx, job.Id)
	return nil
}

func (s *sSysPublish) CleanupTelegramJobMessages(ctx context.Context, jobId int64) error {
	job, err := s.telegramJobById(ctx, jobId)
	if err != nil {
		return err
	}
	return s.deleteTelegramMessageSet(ctx, job, "资料清理")
}

func (s *sSysPublish) telegramJobById(ctx context.Context, jobId int64) (telegramJobRecord, error) {
	var job telegramJobRecord
	if jobId <= 0 {
		return job, gerror.New("TG任务ID不能为空")
	}
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobId).Scan(&job); err != nil {
		return job, gerror.Wrap(err, "读取TG任务失败")
	}
	if job.Id <= 0 {
		return job, gerror.New("TG任务不存在")
	}
	return job, nil
}

func (s *sSysPublish) telegramJobActiveMessages(ctx context.Context, job telegramJobRecord) ([]telegramDeleteMessage, error) {
	var rows []telegramDeleteMessage
	err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Fields("id,target_chat_id,tg_message_id AS message_id").
		Where("job_id", job.Id).
		Where("status", "sent").
		OrderAsc("id").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG消息记录失败")
	}
	return rows, nil
}

type telegramDeleteMessage struct {
	Id           int64  `json:"id"`
	TargetChatId string `json:"targetChatId"`
	MessageId    int64  `json:"messageId"`
}

func (s *sSysPublish) requeueTelegramCyclePublish(ctx context.Context, job telegramJobRecord) error {
	plan := newPublishCyclePlan(job)
	task, err := s.cycleTaskForJob(ctx, job)
	if err != nil {
		return err
	}
	if !plan.CanRepublish(task) {
		s.appendTelegramJobLog(ctx, job, "cycle_publish", "skipped", cycleSkipMessage(job, task))
		return nil
	}
	_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		Data(g.Map{
			"status":        "pending",
			"retry_count":   0,
			"next_retry_at": nil,
			"error_message": "",
			"updated_at":    gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "重置循环上架任务失败")
	}
	s.appendTelegramJobLog(ctx, job, "cycle_publish", "queued", "循环上架发布已加入队列")
	return s.enqueueTelegramJob(ctx, job.Id, 0)
}

func (s *sSysPublish) disableTelegramJobCycle(ctx context.Context, jobId int64) error {
	if jobId <= 0 {
		return nil
	}
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).
		Data(g.Map{
			"cycle_enabled": 0,
			"next_cycle_at": nil,
			"updated_at":    gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "清理无效循环上架任务失败")
	}
	return nil
}
