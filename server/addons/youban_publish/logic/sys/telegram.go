package sys

import (
	"context"
	"encoding/json"
	"strings"
	"time"

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
		AllowedUpdates: telegramAllowedUpdateNames(),
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
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		_, err = bot.DeleteWebhook(ctx, &tgbot.DeleteWebhookParams{DropPendingUpdates: false})
		if err == nil {
			return nil
		}
		lastErr = err
		if !isTelegramWebhookTransientError(err) {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func isTelegramWebhookTransientError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "unexpected end of JSON input") ||
		strings.Contains(message, "EOF") ||
		strings.Contains(message, "Client.Timeout exceeded")
}

func (s *sSysPublish) TelegramWebhookRaw(ctx context.Context, botId int64, body []byte) error {
	var update models.Update
	if err := json.Unmarshal(body, &update); err != nil {
		return gerror.Wrap(err, "解析Telegram webhook失败")
	}
	if botId > 0 {
		if _, err := s.getBotById(ctx, botId, 0); err != nil {
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
	msg, updateType := telegramUpdateMessage(update)
	if msg == nil {
		return
	}
	text := telegramMessageText(msg)
	g.Log().Infof(ctx, "收到上架插件Telegram消息 bot:%d type:%s chat:%d message:%d text:%s", botId, updateType, msg.Chat.ID, msg.ID, text)
	s.handleTelegramAutoDelete(ctx, botId, msg, text)
	s.collectBotMessage(ctx, botId, msg)
}
