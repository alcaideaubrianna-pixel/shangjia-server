package sys

import (
	"context"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"
)

const telegramMessageRetention = 24 * time.Hour

func (s *sSysBot) startTelegramMessageCleanup(ctx context.Context) {
	s.stopTelegramMessageCleanup()
	cleanupCtx, cancel := context.WithCancel(ctx)
	s.cleanupMu.Lock()
	s.cleanupCancel = cancel
	s.cleanupMu.Unlock()
	go s.telegramMessageCleanupLoop(cleanupCtx)
}

func (s *sSysBot) stopTelegramMessageCleanup() {
	s.cleanupMu.Lock()
	cancel := s.cleanupCancel
	s.cleanupCancel = nil
	s.cleanupMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *sSysBot) telegramMessageCleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	_ = s.cleanupTelegramMessages(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.cleanupTelegramMessages(ctx)
		}
	}
}

func (s *sSysBot) cleanupTelegramMessages(ctx context.Context) error {
	limitAt := gtime.Now().Add(-telegramMessageRetention)
	_, err := g.DB().Model(messageTable).Safe().Ctx(ctx).
		WhereNull("retained_at").
		WhereLT("created_at", limitAt).
		Delete()
	return err
}

func (s *sSysBot) upsertTelegramChannelCache(ctx context.Context, botId int64, msg *models.Message, text string, lastAt *gtime.Time) error {
	if msg == nil || msg.Chat.ID == 0 {
		return nil
	}
	chatType := strings.TrimSpace(string(msg.Chat.Type))
	if !isTelegramChannelCacheChatType(chatType) {
		return nil
	}
	chatId := strings.TrimSpace(chatIDFromTelegramMessage(msg))
	if chatId == "" {
		return nil
	}
	chatTitle := firstNonEmpty(msg.Chat.Title, strings.TrimSpace(msg.Chat.FirstName+" "+msg.Chat.LastName), msg.Chat.Username)
	chatUsername := strings.TrimSpace(msg.Chat.Username)
	isBroadcast := 0
	if chatType == "channel" {
		isBroadcast = 1
	}
	isMegagroup := 0
	if chatType == "group" || chatType == "supergroup" {
		isMegagroup = 1
	}

	data := g.Map{
		"bot_id":            botId,
		"chat_id":           chatId,
		"chat_type":         chatType,
		"chat_title":        chatTitle,
		"chat_username":     chatUsername,
		"is_broadcast":      isBroadcast,
		"is_megagroup":      isMegagroup,
		"last_message_text": text,
		"last_message_at":   lastAt,
		"updated_at":        gtime.Now(),
	}
	updateData := g.Map{
		"bot_id":            botId,
		"chat_id":           chatId,
		"chat_type":         chatType,
		"chat_title":        chatTitle,
		"chat_username":     chatUsername,
		"is_broadcast":      isBroadcast,
		"is_megagroup":      isMegagroup,
		"message_count":     gdb.Raw("message_count + 1"),
		"last_message_text": text,
		"last_message_at":   lastAt,
		"updated_at":        gtime.Now(),
	}
	var exists struct {
		Id int64 `json:"id"`
	}
	if err := g.DB().Model(channelCacheTable).Safe().Ctx(ctx).
		Fields("id").
		Where("bot_id", botId).
		Where("chat_id", chatId).
		Scan(&exists); err != nil {
		return err
	}
	if exists.Id > 0 {
		_, err := g.DB().Model(channelCacheTable).Safe().Ctx(ctx).
			Where("id", exists.Id).
			Data(updateData).
			Update()
		return err
	}
	data["message_count"] = 1
	data["created_at"] = gtime.Now()
	_, err := g.DB().Model(channelCacheTable).Safe().Ctx(ctx).Data(data).Insert()
	if err != nil && isDuplicateKeyError(err) {
		_, err = g.DB().Model(channelCacheTable).Safe().Ctx(ctx).
			Where("bot_id", botId).
			Where("chat_id", chatId).
			Data(updateData).
			Update()
	}
	return err
}

func isTelegramChannelCacheChatType(chatType string) bool {
	switch strings.TrimSpace(chatType) {
	case "channel", "group", "supergroup":
		return true
	default:
		return false
	}
}

func chatIDFromTelegramMessage(msg *models.Message) string {
	if msg == nil || msg.Chat.ID == 0 {
		return ""
	}
	return gconv.String(msg.Chat.ID)
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate") || strings.Contains(message, "unique constraint") || strings.Contains(message, "duplicate key")
}

func (s *sSysBot) cleanTelegramMessageCacheByBatch(ctx context.Context, batchSize int) error {
	if batchSize <= 0 {
		batchSize = 1000
	}
	limitAt := gtime.Now().Add(-telegramMessageRetention)
	_, err := g.DB().Model(messageTable).Safe().Ctx(ctx).
		WhereNull("retained_at").
		WhereLT("created_at", limitAt).
		Limit(batchSize).
		Delete()
	return err
}
