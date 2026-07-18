package telegram

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/addons/lazysheep_tggo/service"
)

type botCreateSession struct {
	BotKey          string
	ChatID          int64
	UserID          int64
	PromptMessageID int
}

var botCreateSessions sync.Map

func init() {
	RegisterCallbackHandler(&botCreateCallback{})
}

func openBotCreateSession(ctx context.Context, b *bot.Bot, req *PluginRequest) error {
	if req == nil || req.Update == nil || req.Update.Message == nil || req.Update.Message.From == nil {
		return nil
	}
	chatID := req.Update.Message.Chat.ID
	userID := req.Update.Message.From.ID
	if chatID == 0 || userID == 0 {
		return nil
	}
	if session, ok := getBotCreateSession(chatID, userID); ok && session.PromptMessageID > 0 {
		_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: session.PromptMessageID})
	}
	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: chatID,
		Text:   "请直接发送你的 Telegram Bot Token，系统会自动创建并绑定你的专属机器人。",
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "返回", CallbackData: "botcreate:back"}},
			{{Text: "取消", CallbackData: "botcreate:cancel"}},
		}},
	})
	if err != nil || sent == nil {
		return err
	}
	storeBotCreateSession(botCreateSession{
		BotKey:          req.BotKey,
		ChatID:          chatID,
		UserID:          userID,
		PromptMessageID: sent.ID,
	})
	return nil
}

func HandleCreateBotInput(ctx context.Context, b *bot.Bot, update *models.Update) (bool, error) {
	msg := messageFromUpdate(update)
	if msg == nil || msg.From == nil {
		return false, nil
	}
	token := strings.TrimSpace(msg.Text)
	if token == "" {
		return false, nil
	}
	session, ok := getBotCreateSession(msg.Chat.ID, msg.From.ID)
	if !ok {
		return false, nil
	}
	if session.BotKey != "" && session.BotKey != currentBotKey(ctx) {
		ctx = WithBotKey(ctx, session.BotKey)
	}
	if strings.Contains(token, " ") {
		token = strings.Fields(token)[0]
	}
	progressMsg, _ := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: msg.Chat.ID,
		Text:   "收到，正在验证 Token 并创建机器人，请稍候...",
	})
	info, err := service.SysLazysheepTggo().InspectBot(ctx, &sysin.BotInspectInp{Token: token})
	if err != nil {
		text := fmt.Sprintf("Token 校验失败：%v\n\n请重新发送，或点击取消。", err)
		if progressMsg != nil {
			_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
				ChatID:    msg.Chat.ID,
				MessageID: progressMsg.ID,
				Text:      text,
			})
		} else {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: text})
		}
		return true, nil
	}
	key, err := service.SysLazysheepTggo().UpsertBot(ctx, &sysin.BotUpsertInp{
		Role:        "user",
		MemberId:    msg.From.ID,
		Token:       token,
		DisplayName: info.DisplayName,
		Username:    info.Username,
		Enabled:     true,
	})
	if err != nil {
		text := fmt.Sprintf("创建机器人失败：%v", err)
		if progressMsg != nil {
			_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
				ChatID:    msg.Chat.ID,
				MessageID: progressMsg.ID,
				Text:      text,
			})
		} else {
			_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: text})
		}
		return true, nil
	}
	clearBotCreateSession(msg.Chat.ID, msg.From.ID)
	if session.PromptMessageID > 0 {
		_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: msg.Chat.ID, MessageID: session.PromptMessageID})
	}
	_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: msg.Chat.ID, MessageID: msg.ID})
	text := formatBotCreateSuccess(info.DisplayName, info.Username, key)
	if progressMsg != nil {
		_, _ = b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:    msg.Chat.ID,
			MessageID: progressMsg.ID,
			Text:      text,
		})
	} else {
		_, _ = b.SendMessage(ctx, &bot.SendMessageParams{ChatID: msg.Chat.ID, Text: text})
	}
	return true, nil
}

type botCreateCallback struct{}

func (h *botCreateCallback) Key() string              { return "bot_create" }
func (h *botCreateCallback) Pattern() string          { return "botcreate:" }
func (h *botCreateCallback) MatchType() bot.MatchType { return bot.MatchTypePrefix }
func (h *botCreateCallback) Description() string      { return "创建机器人流程" }
func (h *botCreateCallback) Handle(ctx context.Context, b *bot.Bot, update *models.Update) error {
	if update == nil || update.CallbackQuery == nil {
		return nil
	}
	chatID := callbackChatID(update.CallbackQuery)
	userID := update.CallbackQuery.From.ID
	switch update.CallbackQuery.Data {
	case "botcreate:back", "botcreate:cancel":
		clearBotCreateSession(chatID, userID)
		if update.CallbackQuery.Message.Message != nil {
			_, _ = b.DeleteMessage(ctx, &bot.DeleteMessageParams{
				ChatID:    chatID,
				MessageID: update.CallbackQuery.Message.Message.ID,
			})
		}
		if update.CallbackQuery.Data == "botcreate:cancel" {
			return replyCallback(ctx, b, update, "已取消创建机器人。")
		}
		return replyCallback(ctx, b, update, "已返回。")
	default:
		return replyCallback(ctx, b, update, "未知操作。")
	}
}

func formatBotCreateSuccess(displayName, username, key string) string {
	parts := []string{"机器人已创建并保存。"}
	if strings.TrimSpace(displayName) != "" {
		parts = append(parts, "名称："+strings.TrimSpace(displayName))
	}
	if strings.TrimSpace(username) != "" {
		username = strings.TrimPrefix(strings.TrimSpace(username), "@")
		parts = append(parts, "用户名：@"+username)
	}
	if strings.TrimSpace(key) != "" {
		parts = append(parts, "机器人标识："+strings.TrimSpace(key))
	}
	return strings.Join(parts, "\n")
}

func storeBotCreateSession(session botCreateSession) {
	botCreateSessions.Store(botCreateSessionKey(session.ChatID, session.UserID), session)
}

func getBotCreateSession(chatID, userID int64) (botCreateSession, bool) {
	raw, ok := botCreateSessions.Load(botCreateSessionKey(chatID, userID))
	if !ok {
		return botCreateSession{}, false
	}
	session, ok := raw.(botCreateSession)
	return session, ok
}

func clearBotCreateSession(chatID, userID int64) {
	botCreateSessions.Delete(botCreateSessionKey(chatID, userID))
}

func botCreateSessionKey(chatID, userID int64) string {
	return fmt.Sprintf("%d:%d", chatID, userID)
}
