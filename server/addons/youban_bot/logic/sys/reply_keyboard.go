package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"

	botconsts "hotgo/addons/youban_bot/consts"
	"hotgo/internal/library/cache"
)

const replyKeyboardUserVersionTTL = 30 * 24 * time.Hour
const replyKeyboardSilentText = "\u2063"

func (s *sSysBot) ensureReplyKeyboardCurrent(ctx context.Context, botId int64, chatId string) error {
	telegramUserId := telegramUserIdFromCtx(ctx)
	if telegramUserId == "" || botId <= 0 || strings.TrimSpace(chatId) == "" {
		return nil
	}
	var markup models.ReplyMarkup = s.replyKeyboard(ctx)
	if markup == nil {
		markup = &models.ReplyKeyboardRemove{RemoveKeyboard: true}
	}
	version := replyKeyboardDeliveryVersion(markup)
	userKey := replyKeyboardUserVersionKey(botId, telegramUserId)
	value, cacheErr := cache.Instance().Get(ctx, userKey)
	if cacheErr == nil && !value.IsNil() && value.String() == version {
		return nil
	}
	return s.refreshReplyKeyboardSilently(ctx, botId, chatId, markup)
}

func (s *sSysBot) refreshReplyKeyboardSilently(ctx context.Context, botId int64, chatId string, replyMarkup models.ReplyMarkup) error {
	botToken := s.replyBotToken(ctx, botId)
	if botToken == "" {
		return nil
	}
	message, err := s.sendMessageWithMarkup(ctx, botToken, chatId, replyKeyboardSilentText, "", true, replyMarkup)
	s.trackReplyResult(ctx, botId, replyMarkup, err)
	if err != nil || message == nil {
		return err
	}
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	callCtx, cancel := telegramAPICtx()
	defer cancel()
	_, err = bot.DeleteMessage(callCtx, &tgbot.DeleteMessageParams{ChatID: chatId, MessageID: message.ID})
	return err
}

func (s *sSysBot) markReplyKeyboardDelivered(ctx context.Context, botId int64, replyMarkup models.ReplyMarkup) {
	if !isReplyKeyboardMarkup(replyMarkup) {
		return
	}
	telegramUserId := telegramUserIdFromCtx(ctx)
	if telegramUserId == "" || botId <= 0 {
		return
	}
	version := replyKeyboardDeliveryVersion(replyMarkup)
	if err := cache.Instance().Set(ctx, replyKeyboardUserVersionKey(botId, telegramUserId), version, replyKeyboardUserVersionTTL); err != nil {
		g.Log().Warningf(ctx, "记录Telegram底部键盘版本失败 botId:%d telegramUserId:%s err:%+v", botId, telegramUserId, err)
	}
}

func replyKeyboardDeliveryVersion(replyMarkup models.ReplyMarkup) string {
	builder := strings.Builder{}
	switch markup := replyMarkup.(type) {
	case *models.ReplyKeyboardMarkup:
		builder.WriteString("keyboard|")
		if markup != nil {
			builder.WriteString(fmt.Sprintf("%t|%t|%t|%s|%t|", markup.IsPersistent, markup.ResizeKeyboard, markup.OneTimeKeyboard, markup.InputFieldPlaceholder, markup.Selective))
			for _, row := range markup.Keyboard {
				for _, button := range row {
					builder.WriteString(button.Text)
					builder.WriteByte('\x1f')
				}
				builder.WriteByte('\x1e')
			}
		}
	case *models.ReplyKeyboardRemove:
		builder.WriteString("remove|")
		if markup != nil {
			builder.WriteString(fmt.Sprintf("%t|%t", markup.RemoveKeyboard, markup.Selective))
		}
	default:
		builder.WriteString("none")
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}

func replyKeyboardUserVersionKey(botId int64, telegramUserId string) string {
	return fmt.Sprintf("%s%d:%s", botconsts.ReplyKeyboardUserPrefix, botId, strings.TrimSpace(telegramUserId))
}

func isReplyKeyboardMarkup(replyMarkup models.ReplyMarkup) bool {
	switch replyMarkup.(type) {
	case *models.ReplyKeyboardMarkup, *models.ReplyKeyboardRemove:
		return true
	default:
		return false
	}
}
