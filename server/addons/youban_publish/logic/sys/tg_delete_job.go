package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func (s *sSysPublish) CleanupTelegramJobMessages(ctx context.Context, jobId int64) error {
	job, err := s.telegramJobById(ctx, jobId)
	if err != nil {
		return err
	}
	lease, ok, err := s.tryTelegramChannelLease(ctx, job.TargetChatId)
	if err != nil {
		return err
	}
	if !ok {
		delay := s.telegramChannelBusyDelay(ctx, jobId)
		return s.requeueTelegramJob(ctx, tgTaskTypeCleanup, jobId, delay)
	}
	defer s.releaseTelegramChannelLease(ctx, lease)
	return s.deleteTelegramMessageSetLockedByChannel(ctx, job, "资料清理")
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
