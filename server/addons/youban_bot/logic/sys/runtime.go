package sys

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
)

func (s *sSysBot) restartRuntime(ctx context.Context) {
	_ = s.syncAllTelegramBotMenus(ctx)
	if err := gatewayservice.Gateway().Refresh(ctx); err != nil {
		g.Log().Warningf(ctx, "刷新 Telegram Gateway 失败：%+v", err)
	}
}

func botAllowedUpdatesMatch(actual []string) bool {
	expected := botAllowedUpdates()
	if len(actual) != len(expected) {
		return false
	}
	set := make(map[string]struct{}, len(actual))
	for _, update := range actual {
		set[strings.TrimSpace(update)] = struct{}{}
	}
	for _, update := range expected {
		if _, ok := set[update]; !ok {
			return false
		}
	}
	return true
}

func (s *sSysBot) handleUpdate(ctx context.Context, botId int64, update *models.Update) error {
	if update == nil {
		return nil
	}
	if update.InlineQuery != nil {
		return s.handleProfileInlineQuery(ctx, botId, update.InlineQuery)
	}
	if update.CallbackQuery != nil {
		g.Log().Infof(ctx, "收到Telegram Callback botId:%d chatId:%s userId:%s data:%s", botId, callbackQueryChatId(update.CallbackQuery), callbackQueryUserId(update.CallbackQuery), strings.TrimSpace(update.CallbackQuery.Data))
		if handled, err := s.handleInstantRegisterCallback(ctx, botId, update.CallbackQuery); handled || err != nil {
			if err != nil {
				g.Log().Warningf(ctx, "Telegram Callback处理失败 handler:instant_register botId:%d chatId:%s userId:%s data:%s err:%+v", botId, callbackQueryChatId(update.CallbackQuery), callbackQueryUserId(update.CallbackQuery), strings.TrimSpace(update.CallbackQuery.Data), err)
			}
			return err
		}
		if handled, err := s.handleQuickPushCallback(ctx, botId, update.CallbackQuery); handled || err != nil {
			if err != nil {
				g.Log().Warningf(ctx, "Telegram Callback处理失败 handler:quick_push botId:%d chatId:%s userId:%s data:%s err:%+v", botId, callbackQueryChatId(update.CallbackQuery), callbackQueryUserId(update.CallbackQuery), strings.TrimSpace(update.CallbackQuery.Data), err)
			}
			return err
		}
		if handled, err := s.handleProfileCallback(ctx, botId, update.CallbackQuery); handled || err != nil {
			if err != nil {
				g.Log().Warningf(ctx, "Telegram Callback处理失败 handler:profile botId:%d chatId:%s userId:%s data:%s err:%+v", botId, callbackQueryChatId(update.CallbackQuery), callbackQueryUserId(update.CallbackQuery), strings.TrimSpace(update.CallbackQuery.Data), err)
			}
			return err
		}
		if handled, err := s.handleScanCallback(ctx, botId, update.CallbackQuery); handled || err != nil {
			if err != nil {
				g.Log().Warningf(ctx, "Telegram Callback处理失败 handler:scan botId:%d chatId:%s userId:%s data:%s err:%+v", botId, callbackQueryChatId(update.CallbackQuery), callbackQueryUserId(update.CallbackQuery), strings.TrimSpace(update.CallbackQuery.Data), err)
			}
			return err
		}
		if err := s.handleExchangeRateCallback(ctx, botId, update.CallbackQuery); err != nil {
			g.Log().Warningf(ctx, "Telegram Callback处理失败 handler:exchange_rate botId:%d chatId:%s userId:%s data:%s err:%+v", botId, callbackQueryChatId(update.CallbackQuery), callbackQueryUserId(update.CallbackQuery), strings.TrimSpace(update.CallbackQuery.Data), err)
			return err
		}
		return nil
	}
	msg := botMessageFromUpdate(update)
	if msg == nil {
		return nil
	}
	userId := ""
	if msg.From != nil {
		userId = fmt.Sprintf("%d", msg.From.ID)
		ctx = context.WithValue(ctx, telegramUserIdCtxKey{}, userId)
	}
	g.Log().Infof(ctx, "收到Telegram Update botId:%d chatId:%d userId:%s text:%s", botId, msg.Chat.ID, userId, strings.TrimSpace(firstNonEmpty(msg.Text, msg.Caption)))
	if err := s.storeTelegramMessage(ctx, botId, msg); err != nil {
		g.Log().Warningf(ctx, "保存Telegram消息日志失败 botId:%d err:%+v", botId, err)
	}
	text := strings.TrimSpace(firstNonEmpty(msg.Text, msg.Caption))
	_, err := s.dispatchBotMessage(ctx, &botMessageEvent{BotId: botId, Msg: msg, Text: text})
	if err != nil {
		g.Log().Warningf(ctx, "Telegram消息处理失败 botId:%d chatId:%d userId:%s text:%s err:%+v", botId, msg.Chat.ID, userId, text, err)
	}
	if msg.Chat.Type == models.ChatTypePrivate {
		if s.activeProfileSession(ctx, botId, msg) != nil {
			return err
		}
		if refreshErr := s.ensureReplyKeyboardCurrent(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID)); refreshErr != nil {
			g.Log().Warningf(ctx, "刷新Telegram底部键盘失败 botId:%d chatId:%d userId:%s err:%+v", botId, msg.Chat.ID, userId, refreshErr)
		}
	}
	return err
}

func isTelegramCommand(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "/")
}

func callbackQueryChatId(query *models.CallbackQuery) string {
	if query == nil || query.Message.Message == nil {
		return ""
	}
	return fmt.Sprintf("%d", query.Message.Message.Chat.ID)
}

func callbackQueryUserId(query *models.CallbackQuery) string {
	if query == nil {
		return ""
	}
	return fmt.Sprintf("%d", query.From.ID)
}

func (s *sSysBot) storeTelegramMessage(ctx context.Context, botId int64, msg *models.Message) error {
	if msg == nil {
		return nil
	}
	now := gtime.Now()
	telegramUserId := ""
	telegramUsername := ""
	telegramFirstName := ""
	telegramLastName := ""
	if msg.From != nil {
		telegramUserId = fmt.Sprintf("%d", msg.From.ID)
		telegramUsername = strings.TrimPrefix(strings.TrimSpace(msg.From.Username), "@")
		telegramFirstName = msg.From.FirstName
		telegramLastName = msg.From.LastName
	}
	chatId := fmt.Sprintf("%d", msg.Chat.ID)
	text := strings.TrimSpace(firstNonEmpty(msg.Text, msg.Caption))
	messageType := telegramMessageType(msg)
	rawBytes, _ := json.Marshal(msg)
	lastAt := now
	if msg.Date > 0 {
		lastAt = gtime.NewFromTime(time.Unix(int64(msg.Date), 0))
	}
	if err := s.upsertTelegramChannelCache(ctx, botId, msg, text, lastAt); err != nil {
		return err
	}
	if telegramUserId != "" {
		userData := g.Map{
			"bot_id":              botId,
			"telegram_user_id":    telegramUserId,
			"telegram_username":   telegramUsername,
			"telegram_first_name": telegramFirstName,
			"telegram_last_name":  telegramLastName,
			"chat_id":             chatId,
			"chat_type":           string(msg.Chat.Type),
			"chat_title":          firstNonEmpty(msg.Chat.Title, strings.TrimSpace(msg.Chat.FirstName+" "+msg.Chat.LastName), msg.Chat.Username),
			"last_message_text":   text,
			"last_message_at":     lastAt,
			"status":              1,
			"updated_at":          now,
		}
		var exists struct {
			Id int64 `json:"id"`
		}
		_ = g.DB().Model(userTable).Safe().Ctx(ctx).Fields("id").Where("bot_id", botId).Where("telegram_user_id", telegramUserId).Scan(&exists)
		if exists.Id > 0 {
			if _, err := g.DB().Model(userTable).Safe().Ctx(ctx).Where("id", exists.Id).Data(userData).Increment("message_count", 1); err != nil {
				return gerror.Wrap(err, "更新Telegram用户失败")
			}
		} else {
			userData["message_count"] = 1
			userData["created_at"] = now
			if _, err := g.DB().Model(userTable).Safe().Ctx(ctx).Data(userData).Insert(); err != nil {
				return gerror.Wrap(err, "写入Telegram用户失败")
			}
		}
		g.Log().Infof(ctx, "已记录Telegram用户 botId:%d telegramUserId:%s chatId:%s", botId, telegramUserId, chatId)
	}
	var exists struct {
		Id int64 `json:"id"`
	}
	if err := g.DB().Model(messageTable).Safe().Ctx(ctx).
		Fields("id").
		Where("chat_id", chatId).
		Where("message_id", msg.ID).
		Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = nil
		} else {
			return gerror.Wrap(err, "读取Telegram消息失败")
		}
	}
	if exists.Id > 0 {
		return nil
	}
	_, err := g.DB().Model(messageTable).Safe().Ctx(ctx).Data(g.Map{
		"bot_id":            botId,
		"telegram_user_id":  telegramUserId,
		"telegram_username": telegramUsername,
		"chat_id":           chatId,
		"chat_type":         string(msg.Chat.Type),
		"message_id":        msg.ID,
		"message_type":      messageType,
		"text":              text,
		"raw_json":          string(rawBytes),
		"created_at":        lastAt,
	}).Insert()
	if err != nil && isDuplicateKeyError(err) {
		return nil
	}
	if err != nil {
		return gerror.Wrap(err, "保存Telegram消息失败")
	}
	return nil
}

func telegramMessageType(msg *models.Message) string {
	switch {
	case msg.Text != "":
		return "text"
	case msg.Caption != "":
		return "caption"
	case len(msg.Photo) > 0:
		return "photo"
	case msg.Video != nil:
		return "video"
	case msg.Document != nil:
		return "document"
	case msg.Voice != nil:
		return "voice"
	case msg.Sticker != nil:
		return "sticker"
	default:
		return "other"
	}
}
