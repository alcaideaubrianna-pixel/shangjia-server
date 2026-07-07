package sys

import (
	"context"
	"fmt"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) deleteTelegramMessageSet(ctx context.Context, job telegramJobRecord, reason string) error {
	messages, err := s.telegramJobActiveMessages(ctx, job)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		s.appendTelegramJobLog(ctx, job, "delete", "skipped", reason+"，未找到可删除的TG消息")
		return nil
	}
	botToken, err := s.telegramJobBotToken(ctx, job.BotId, job.TenantId)
	if err != nil {
		return err
	}
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	s.appendTelegramJobLog(ctx, job, "delete", "started", reason+"，开始删除TG消息")
	for _, item := range messages {
		chatId := normalizeTelegramChannelChatID(item.TargetChatId)
		if item.MessageId <= 0 || chatId == "" {
			continue
		}
		_, err = bot.DeleteMessage(ctx, &tgbot.DeleteMessageParams{ChatID: chatId, MessageID: int(item.MessageId)})
		if err != nil {
			message := fmt.Sprintf("%s，删除TG消息失败，频道:%s，消息:%d，错误:%s", reason, chatId, item.MessageId, err.Error())
			s.appendTelegramJobLog(ctx, job, "delete", "failed", message)
			return gerror.New(message)
		}
		_, _ = g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
			Where("id", item.Id).
			Data(g.Map{"status": "deleted", "deleted_at": gtime.Now(), "updated_at": gtime.Now()}).
			Update()
	}
	s.appendTelegramJobLog(ctx, job, "delete", "success", reason+"，TG消息删除成功")
	return nil
}
