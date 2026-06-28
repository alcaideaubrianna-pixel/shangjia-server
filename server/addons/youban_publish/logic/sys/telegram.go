package sys

import (
	"context"
	"encoding/json"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func (s *sSysPublish) telegramSetWebhook(ctx context.Context, botToken string, webhookURL string) error {
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return err
	}
	params := &tgbot.SetWebhookParams{
		URL:            webhookURL,
		AllowedUpdates: []string{"message", "edited_message"},
	}
	if conf.WebhookSecret != "" {
		params.SecretToken = conf.WebhookSecret
	}
	_, err = bot.SetWebhook(ctx, params)
	return err
}

func (s *sSysPublish) telegramDeleteWebhook(ctx context.Context, botToken string) error {
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	_, err = bot.DeleteWebhook(ctx, &tgbot.DeleteWebhookParams{DropPendingUpdates: false})
	return err
}

func (s *sSysPublish) TelegramWebhookRaw(ctx context.Context, botId int64, body []byte) error {
	var update models.Update
	if err := json.Unmarshal(body, &update); err != nil {
		return gerror.Wrap(err, "解析Telegram webhook失败")
	}
	if botId > 0 {
		if _, err := s.getBotById(ctx, botId); err != nil {
			return err
		}
	}
	s.handleTelegramUpdate(ctx, botId, &update)
	return nil
}

func (s *sSysPublish) telegramUpdateHandler(ctx context.Context, currentBot *tgbot.Bot, update *models.Update) {
	s.handleTelegramUpdate(ctx, 0, update)
}

func (s *sSysPublish) handleTelegramUpdate(ctx context.Context, botId int64, update *models.Update) {
	if update == nil {
		return
	}
	msg := update.Message
	if msg == nil {
		msg = update.EditedMessage
	}
	if msg == nil {
		return
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" && msg.Caption != "" {
		text = strings.TrimSpace(msg.Caption)
	}
	g.Log().Infof(ctx, "收到上架插件Telegram消息 bot:%d chat:%d message:%d text:%s", botId, msg.Chat.ID, msg.ID, text)
}
