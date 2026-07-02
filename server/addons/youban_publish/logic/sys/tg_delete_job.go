package sys

import (
	"context"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) DeleteTelegramJobMessages(ctx context.Context, jobId int64) error {
	job, err := s.telegramJobById(ctx, jobId)
	if err != nil {
		return err
	}
	botToken, err := s.telegramJobBotToken(ctx, job.BotId, job.TenantId)
	if err != nil {
		return err
	}
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	messages, err := s.telegramJobActiveMessages(ctx, job)
	if err != nil {
		return err
	}
	for _, item := range messages {
		if item.MessageId <= 0 || item.TargetChatId == "" {
			continue
		}
		_, err = bot.DeleteMessage(ctx, &tgbot.DeleteMessageParams{
			ChatID:    item.TargetChatId,
			MessageID: int(item.MessageId),
		})
		if err != nil {
			s.appendTelegramJobLog(ctx, job, "delete", "failed", err.Error())
			return err
		}
		_, _ = g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
			Where("id", item.Id).
			Data(g.Map{"status": "deleted", "deleted_at": gtime.Now(), "updated_at": gtime.Now()}).
			Update()
	}
	s.appendTelegramJobLog(ctx, job, "delete", "success", "TG历史消息删除成功")
	return nil
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
