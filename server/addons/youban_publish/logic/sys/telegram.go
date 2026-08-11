package sys

import (
	"context"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
)

func (s *sSysPublish) telegramUpdateHandler(ctx context.Context, currentBot *tgbot.Bot, update *models.Update) {
	s.handleTelegramUpdate(ctx, 0, 0, update)
}

func (s *sSysPublish) handleTelegramUpdate(ctx context.Context, botId int64, tenantId int64, update *models.Update) {
	msg, updateType := telegramUpdateMessage(update)
	if msg == nil {
		return
	}
	text := telegramMessageText(msg)
	g.Log().Debugf(ctx, "收到上架插件Telegram消息 bot:%d type:%s chat:%d message:%d text:%s", botId, updateType, msg.Chat.ID, msg.ID, text)
	s.handleTelegramAutoDelete(ctx, botId, tenantId, msg, text)
	if err := s.cacheBotMessage(ctx, tenantId, botId, msg); err != nil {
		g.Log().Warningf(ctx, "缓存上架Bot频道消息失败 bot:%d chat:%d err:%+v", botId, msg.Chat.ID, err)
	}
	if err := s.collectBotMessage(ctx, botId, msg); err != nil {
		g.Log().Warningf(ctx, "处理Bot采集消息失败 bot:%d chat:%d message:%d err:%+v", botId, msg.Chat.ID, msg.ID, err)
	}
}
