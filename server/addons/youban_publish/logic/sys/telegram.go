package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/utility/simple"
)

const telegramBotMessageSourceTable = "hg_youban_publish_bot_message_source"

const telegramUpdateSlowStageLogThreshold = 100 * time.Millisecond

func (s *sSysPublish) telegramUpdateHandler(ctx context.Context, currentBot *tgbot.Bot, update *models.Update) {
	botId := int64(0)
	if currentBot != nil {
		botId = currentBot.ID()
	}
	s.handleTelegramUpdate(ctx, botId, 0, update)
}

func (s *sSysPublish) handleTelegramUpdate(ctx context.Context, botId int64, tenantId int64, update *models.Update) {
	msg, updateType := telegramUpdateMessage(update)
	if msg == nil {
		return
	}
	text := telegramMessageText(msg)
	g.Log().Debugf(ctx, "收到上架插件Telegram消息 bot:%d type:%s chat:%d message:%d text:%s", botId, updateType, msg.Chat.ID, msg.ID, text)
	fact := telegramWebhookReconcileMessage{
		ChatID:       normalizeTelegramChannelChatID(fmt.Sprintf("%d", msg.Chat.ID)),
		MessageID:    int64(msg.ID),
		MediaGroupID: msg.MediaGroupID,
		MessageType:  telegramWebhookMessageType(msg),
		ReceivedAt:   time.Now(),
	}
	reserved, reserveErr := reserveTelegramWebhookReconcileMessage(ctx, fact)
	if reserveErr != nil {
		g.Log().Warning(ctx, "TG消息事实Redis去重不可用，回退数据库幂等", g.Map{
			"event":   "telegram_webhook_fact_dedupe_fallback",
			"chat_id": fact.ChatID, "message_id": fact.MessageID, "error": reserveErr,
		})
		reserved = true
	}
	stageStartedAt := time.Now()
	if reserved {
		if err := s.recordTelegramBotMessageSource(ctx, botId, tenantId, update.ID, updateType, msg, text); err != nil {
			if cache.Initialized() {
				_, _ = cache.Instance().Remove(ctx, telegramWebhookReconcileMessageKey(fact.ChatID, fact.MessageID))
			}
		}
	} else {
		g.Log().Info(ctx, "TG消息事实Redis命中，跳过来源表重复写入", g.Map{
			"event":   "telegram_webhook_fact_deduplicated",
			"chat_id": fact.ChatID, "message_id": fact.MessageID,
			"dedupe_key": telegramWebhookReconcileMessageKey(fact.ChatID, fact.MessageID),
		})
	}
	logTelegramUpdateSlowStage(ctx, update.ID, botId, "record_source", time.Since(stageStartedAt), nil)
	stageStartedAt = time.Now()
	s.handleTelegramAutoDelete(ctx, botId, tenantId, msg, text)
	logTelegramUpdateSlowStage(ctx, update.ID, botId, "auto_delete_check", time.Since(stageStartedAt), nil)
	stageStartedAt = time.Now()
	if err := s.cacheBotMessage(ctx, tenantId, botId, msg); err != nil {
		g.Log().Warningf(ctx, "缓存上架Bot频道消息失败 bot:%d chat:%d err:%+v", botId, msg.Chat.ID, err)
		logTelegramUpdateSlowStage(ctx, update.ID, botId, "channel_cache", time.Since(stageStartedAt), err)
	} else {
		logTelegramUpdateSlowStage(ctx, update.ID, botId, "channel_cache", time.Since(stageStartedAt), nil)
	}
	if reserved {
		cacheTelegramWebhookReconcileMessage(ctx, fact)
	}
	g.Log().Info(ctx, "TG链路 webhook_fact_cached", g.Map{
		"event":                "telegram_webhook_fact_cached",
		"update_id":            update.ID,
		"update_type":          updateType,
		"bot_id":               botId,
		"tenant_id":            tenantId,
		"chat_id":              normalizeTelegramChannelChatID(fmt.Sprintf("%d", msg.Chat.ID)),
		"message_id":           msg.ID,
		"media_group_id":       msg.MediaGroupID,
		"message_type":         telegramMessageMediaType(msg),
		"source_table":         telegramBotMessageSourceTable,
		"redis_fact":           true,
		"source":               "telegram_update",
		"source_table_written": true,
	})
	matchCtx := context.WithoutCancel(ctx)
	simple.SafeGo(matchCtx, func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		s.matchTelegramDeliveryAttempt(ctx, botId, msg, text)
	})
}

func logTelegramUpdateSlowStage(ctx context.Context, updateID, botID int64, stage string, duration time.Duration, err error) {
	if duration < telegramUpdateSlowStageLogThreshold && err == nil {
		return
	}
	if err != nil {
		g.Log().Warningf(ctx, "TG链路 publish_update_slow_stage updateId:%d botId:%d stage:%s duration:%s result:error errorType:%T", updateID, botID, stage, duration, err)
		return
	}
	g.Log().Infof(ctx, "TG链路 publish_update_slow_stage updateId:%d botId:%d stage:%s duration:%s result:ok", updateID, botID, stage, duration)
}

func (s *sSysPublish) recordTelegramBotMessageSource(ctx context.Context, botId, tenantId int64, updateId int64, updateType string, msg *models.Message, text string) error {
	if msg == nil || msg.ID <= 0 || msg.Chat.ID == 0 {
		return nil
	}
	chatId := normalizeTelegramChannelChatID(fmt.Sprintf("%d", msg.Chat.ID))
	data := g.Map{
		"tenant_id": tenantId, "received_bot_id": botId, "chat_id": chatId,
		"update_id":  updateId,
		"message_id": msg.ID, "media_group_id": msg.MediaGroupID,
		"tg_file_id": telegramMessageFileId(msg), "tg_file_unique_id": telegramMessageFileUniqueId(msg),
		"update_type": updateType, "message_text": strings.TrimSpace(text),
		"process_status": "pending", "received_at": gtime.Now(),
	}
	if msg.From != nil {
		data["sender_user_id"] = msg.From.ID
		data["sender_username"] = msg.From.Username
	}
	if msg.SenderChat != nil {
		data["sender_chat_id"] = msg.SenderChat.ID
		data["sender_chat_title"] = msg.SenderChat.Title
	}
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.ID > 0 {
		data["reply_to_message_id"] = msg.ReplyToMessage.ID
		var reply struct {
			JobId     int64  `orm:"job_id"`
			ProfileId int64  `orm:"profile_id"`
			Purpose   string `orm:"purpose"`
		}
		if err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
			Fields("job_id,profile_id,purpose").
			Where("target_chat_id", chatId).
			Where("tg_message_id", msg.ReplyToMessage.ID).
			OrderDesc("id").Scan(&reply); err == nil {
			data["reply_job_id"] = reply.JobId
			data["reply_profile_id"] = reply.ProfileId
			data["reply_purpose"] = reply.Purpose
		}
	}
	_, err := g.DB().Model(telegramBotMessageSourceTable).Safe().Ctx(ctx).
		Data(data).
		OnConflict("chat_id,message_id").
		OnDuplicate("tenant_id,update_id,media_group_id,tg_file_id,tg_file_unique_id,sender_user_id,sender_username,sender_chat_id,sender_chat_title,reply_to_message_id,reply_job_id,reply_profile_id,reply_purpose,update_type,message_text,received_at").
		Save()
	if err != nil {
		g.Log().Warningf(ctx, "记录Telegram Bot消息来源失败 bot:%d chat:%s message:%d err:%+v", botId, chatId, msg.ID, err)
		return err
	}
	return nil
}
