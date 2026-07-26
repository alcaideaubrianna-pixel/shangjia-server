package sys

import (
	"context"
	"encoding/json"
	"errors"
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
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		_, err = bot.SetWebhook(ctx, params)
		if err == nil {
			return nil
		}
		lastErr = err
		var tooMany *tgbot.TooManyRequestsError
		if errors.As(err, &tooMany) && tooMany.RetryAfter > 0 {
			if err = telegramWebhookWait(ctx, time.Duration(tooMany.RetryAfter)*time.Second); err != nil {
				return err
			}
			continue
		}
		if !isTelegramWebhookTransientError(err) {
			return err
		}
		if err = telegramWebhookWait(ctx, time.Duration(attempt+1)*500*time.Millisecond); err != nil {
			return err
		}
	}
	return lastErr
}

func telegramWebhookWait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
	tenantId := int64(0)
	if botId > 0 {
		bot, err := s.getBotById(ctx, botId, 0)
		if err != nil {
			return err
		}
		tenantId = bot.TenantId
	}
	s.handleTelegramUpdate(ctx, botId, tenantId, &update)
	return nil
}

func (s *sSysPublish) telegramUpdateHandler(ctx context.Context, currentBot *tgbot.Bot, update *models.Update) {
	s.handleTelegramUpdate(ctx, 0, 0, update)
}

func (s *sSysPublish) handleTelegramUpdate(ctx context.Context, botId int64, tenantId int64, update *models.Update) {
	msg, updateType := telegramUpdateMessage(update)
	if msg == nil {
		return
	}
	text := telegramMessageText(msg)
	g.Log().Infof(ctx, "收到上架插件Telegram消息 bot:%d type:%s chat:%d message:%d text:%s", botId, updateType, msg.Chat.ID, msg.ID, text)
	s.handleTelegramAutoDelete(ctx, botId, tenantId, msg, text)
	s.collectBotMessage(ctx, botId, msg)
}
