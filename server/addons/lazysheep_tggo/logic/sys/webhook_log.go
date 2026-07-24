package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/internal/library/cache"
)

func (s *sLazySheepTGGo) saveWebhookLog(ctx context.Context, botKey string, payload []byte, update *models.Update) {
	if strings.TrimSpace(botKey) == "" || len(payload) == 0 {
		return
	}
	s.rememberTelegramChat(ctx, botKey, update)
	updateType, chatID, userID, username, messageID, summary := summarizeWebhookUpdate(update)
	if update != nil && update.ID != 0 && updateType == "" {
		updateType = "unknown"
	}
	if err := s.ensureWebhookLogTable(ctx); err != nil {
		g.Log().Warningf(ctx, "初始化 webhook 原始日志表失败 botKey:%s err:%+v", botKey, err)
		return
	}
	_, err := g.DB().Exec(ctx, `
		INSERT INTO hg_addon_lazysheep_tggo_webhook_log
		(bot_key, update_id, update_type, chat_id, user_id, username, message_id, summary, payload, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, botKey, updateID(update), updateType, chatID, userID, username, messageID, summary, strings.ToValidUTF8(string(payload), "�"), gtime.Now(), gtime.Now())
	if err != nil {
		g.Log().Warningf(ctx, "保存 webhook 原始日志失败 botKey:%s err:%+v", botKey, err)
	}
}

func (s *sLazySheepTGGo) rememberTelegramChat(ctx context.Context, botKey string, update *models.Update) {
	chat, ok := telegramChatFromUpdate(update)
	if !ok || chat.ID == 0 {
		return
	}
	if err := s.ensureChatMapTable(ctx); err != nil {
		g.Log().Warningf(ctx, "初始化频道映射表失败 botKey:%s chat:%d err:%+v", botKey, chat.ID, err)
		return
	}
	title := strings.TrimSpace(chat.Title)
	if title == "" {
		title = strings.TrimSpace(strings.TrimSpace(chat.FirstName + " " + chat.LastName))
	}
	username := strings.TrimPrefix(strings.TrimSpace(chat.Username), "@")
	label := title
	if label == "" && username != "" {
		label = "@" + username
	}
	if label == "" {
		label = fmt.Sprintf("%d", chat.ID)
	}
	now := gtime.Now()
	if g.DB().GetConfig().Type == "pgsql" {
		_, _ = g.DB().Exec(ctx, `
			INSERT INTO hg_addon_lazysheep_tggo_chat_map
			(bot_key, chat_id, chat_type, title, username, label, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (bot_key, chat_id) DO UPDATE SET
			chat_type=EXCLUDED.chat_type,title=EXCLUDED.title,username=EXCLUDED.username,label=EXCLUDED.label,updated_at=EXCLUDED.updated_at
		`, botKey, chat.ID, string(chat.Type), title, username, label, now, now)
		_, _ = cache.Instance().Remove(ctx, monitorChatMapCacheKey)
		return
	}
	_, _ = g.DB().Exec(ctx, `
		INSERT INTO hg_addon_lazysheep_tggo_chat_map
		(bot_key, chat_id, chat_type, title, username, label, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE chat_type=VALUES(chat_type),title=VALUES(title),username=VALUES(username),label=VALUES(label),updated_at=VALUES(updated_at)
	`, botKey, chat.ID, string(chat.Type), title, username, label, now, now)
	_, _ = cache.Instance().Remove(ctx, monitorChatMapCacheKey)
}

func telegramChatFromUpdate(update *models.Update) (models.Chat, bool) {
	if update == nil {
		return models.Chat{}, false
	}
	switch {
	case update.Message != nil:
		return update.Message.Chat, true
	case update.EditedMessage != nil:
		return update.EditedMessage.Chat, true
	case update.ChannelPost != nil:
		return update.ChannelPost.Chat, true
	case update.EditedChannelPost != nil:
		return update.EditedChannelPost.Chat, true
	case update.BusinessMessage != nil:
		return update.BusinessMessage.Chat, true
	case update.EditedBusinessMessage != nil:
		return update.EditedBusinessMessage.Chat, true
	case update.CallbackQuery != nil && update.CallbackQuery.Message.Message != nil:
		return update.CallbackQuery.Message.Message.Chat, true
	case update.CallbackQuery != nil && update.CallbackQuery.Message.InaccessibleMessage != nil:
		return update.CallbackQuery.Message.InaccessibleMessage.Chat, true
	case update.MyChatMember != nil:
		return update.MyChatMember.Chat, true
	case update.ChatMember != nil:
		return update.ChatMember.Chat, true
	case update.ChatJoinRequest != nil:
		return update.ChatJoinRequest.Chat, true
	default:
		return models.Chat{}, false
	}
}

func updateID(update *models.Update) int64 {
	if update == nil {
		return 0
	}
	return update.ID
}

func summarizeWebhookUpdate(update *models.Update) (updateType string, chatID int64, userID int64, username string, messageID int64, summary string) {
	if update == nil {
		return "", 0, 0, "", 0, ""
	}
	switch {
	case update.Message != nil:
		return summarizeMessageUpdate("message", update.Message)
	case update.EditedMessage != nil:
		return summarizeMessageUpdate("edited_message", update.EditedMessage)
	case update.ChannelPost != nil:
		return summarizeMessageUpdate("channel_post", update.ChannelPost)
	case update.EditedChannelPost != nil:
		return summarizeMessageUpdate("edited_channel_post", update.EditedChannelPost)
	case update.BusinessMessage != nil:
		return summarizeMessageUpdate("business_message", update.BusinessMessage)
	case update.EditedBusinessMessage != nil:
		return summarizeMessageUpdate("edited_business_message", update.EditedBusinessMessage)
	case update.CallbackQuery != nil:
		updateType = "callback_query"
		userID = update.CallbackQuery.From.ID
		username = update.CallbackQuery.From.Username
		summary = strings.TrimSpace(update.CallbackQuery.Data)
		if update.CallbackQuery.Message.Message != nil {
			chatID = update.CallbackQuery.Message.Message.Chat.ID
			messageID = int64(update.CallbackQuery.Message.Message.ID)
			if summary == "" {
				summary = messageSummary(update.CallbackQuery.Message.Message)
			}
		} else if update.CallbackQuery.Message.InaccessibleMessage != nil {
			chatID = update.CallbackQuery.Message.InaccessibleMessage.Chat.ID
			messageID = int64(update.CallbackQuery.Message.InaccessibleMessage.MessageID)
		}
		return
	case update.MyChatMember != nil:
		updateType = "my_chat_member"
		chatID = update.MyChatMember.Chat.ID
		userID = update.MyChatMember.From.ID
		username = update.MyChatMember.From.Username
		summary = fmt.Sprintf("%s -> %s", chatMemberTypeName(update.MyChatMember.OldChatMember), chatMemberTypeName(update.MyChatMember.NewChatMember))
		return
	case update.ChatMember != nil:
		updateType = "chat_member"
		chatID = update.ChatMember.Chat.ID
		userID = update.ChatMember.From.ID
		username = update.ChatMember.From.Username
		summary = fmt.Sprintf("%s -> %s", chatMemberTypeName(update.ChatMember.OldChatMember), chatMemberTypeName(update.ChatMember.NewChatMember))
		return
	case update.ChatJoinRequest != nil:
		updateType = "chat_join_request"
		chatID = update.ChatJoinRequest.Chat.ID
		userID = update.ChatJoinRequest.From.ID
		username = update.ChatJoinRequest.From.Username
		summary = strings.TrimSpace(update.ChatJoinRequest.Bio)
		return
	case update.InlineQuery != nil:
		updateType = "inline_query"
		userID = update.InlineQuery.From.ID
		username = update.InlineQuery.From.Username
		summary = strings.TrimSpace(update.InlineQuery.Query)
		return
	case update.ChosenInlineResult != nil:
		updateType = "chosen_inline_result"
		userID = update.ChosenInlineResult.From.ID
		username = update.ChosenInlineResult.From.Username
		summary = strings.TrimSpace(update.ChosenInlineResult.Query)
		return
	default:
		raw, _ := json.Marshal(update)
		return "unknown", 0, 0, "", 0, truncateSummary(string(raw), 240)
	}
}

func summarizeMessageUpdate(updateType string, msg *models.Message) (string, int64, int64, string, int64, string) {
	if msg == nil {
		return updateType, 0, 0, "", 0, ""
	}
	chatID := msg.Chat.ID
	messageID := int64(msg.ID)
	userID := int64(0)
	username := ""
	if msg.From != nil {
		userID = msg.From.ID
		username = msg.From.Username
	}
	return updateType, chatID, userID, username, messageID, messageSummary(msg)
}

func messageSummary(msg *models.Message) string {
	if msg == nil {
		return ""
	}
	if strings.TrimSpace(msg.Text) != "" {
		return truncateSummary(msg.Text, 240)
	}
	if strings.TrimSpace(msg.Caption) != "" {
		return truncateSummary(msg.Caption, 240)
	}
	switch {
	case len(msg.Photo) > 0:
		return "photo"
	case msg.Video != nil:
		return "video"
	case msg.Document != nil:
		return "document"
	case msg.Voice != nil:
		return "voice"
	case msg.Audio != nil:
		return "audio"
	case msg.Location != nil:
		return "location"
	case msg.Contact != nil:
		return "contact"
	case msg.Sticker != nil:
		return "sticker"
	default:
		return ""
	}
}

func chatMemberTypeName(member models.ChatMember) string {
	switch member.Type {
	case models.ChatMemberTypeOwner:
		return "creator"
	case models.ChatMemberTypeAdministrator:
		return "administrator"
	case models.ChatMemberTypeMember:
		return "member"
	case models.ChatMemberTypeRestricted:
		return "restricted"
	case models.ChatMemberTypeLeft:
		return "left"
	case models.ChatMemberTypeBanned:
		return "kicked"
	default:
		return "unknown"
	}
}

func truncateSummary(text string, max int) string {
	text = strings.ToValidUTF8(strings.TrimSpace(text), "�")
	if max <= 0 || len(text) <= max {
		return text
	}
	if max < 3 {
		return safeUTF8Prefix(text, max)
	}
	return safeUTF8Prefix(text, max-3) + "..."
}

func safeUTF8Prefix(text string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(text) <= maxBytes {
		return text
	}
	for maxBytes > 0 && !utf8.ValidString(text[:maxBytes]) {
		maxBytes--
	}
	return text[:maxBytes]
}
