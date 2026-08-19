package sys

import (
	"context"
	"fmt"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const telegramBotMessageSourceTable = "hg_youban_publish_bot_message_source"

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
	s.recordTelegramBotMessageSource(ctx, botId, tenantId, updateType, msg, text)
	s.handleTelegramAutoDelete(ctx, botId, tenantId, msg, text)
	if err := s.cacheBotMessage(ctx, tenantId, botId, msg); err != nil {
		g.Log().Warningf(ctx, "缓存上架Bot频道消息失败 bot:%d chat:%d err:%+v", botId, msg.Chat.ID, err)
	}
}

func (s *sSysPublish) recordTelegramBotMessageSource(ctx context.Context, botId, tenantId int64, updateType string, msg *models.Message, text string) {
	if msg == nil || msg.ID <= 0 || msg.Chat.ID == 0 {
		return
	}
	chatId := normalizeTelegramChannelChatID(fmt.Sprintf("%d", msg.Chat.ID))
	data := g.Map{
		"tenant_id": tenantId, "received_bot_id": botId, "chat_id": chatId,
		"message_id": msg.ID, "media_group_id": msg.MediaGroupID,
		"update_type": updateType, "message_text": strings.TrimSpace(text),
		"received_at": gtime.Now(),
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
		OnConflict("received_bot_id,chat_id,message_id").
		OnDuplicate("tenant_id,media_group_id,sender_user_id,sender_username,sender_chat_id,sender_chat_title,reply_to_message_id,reply_job_id,reply_profile_id,reply_purpose,update_type,message_text,received_at").
		Save()
	if err != nil {
		g.Log().Warningf(ctx, "记录Telegram Bot消息来源失败 bot:%d chat:%s message:%d err:%+v", botId, chatId, msg.ID, err)
	}
}
