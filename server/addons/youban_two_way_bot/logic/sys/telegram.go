package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	twdao "hotgo/addons/youban_two_way_bot/internal/dao"
	"hotgo/addons/youban_two_way_bot/internal/model/entity"
	"hotgo/addons/youban_two_way_bot/model/input/sysin"
)

func (s *sSysTwoWayBot) TelegramWebhookRaw(ctx context.Context, in *sysin.WebhookInp) error {
	if in == nil || len(in.Body) == 0 {
		return gerror.New("Webhook消息不能为空")
	}
	if in.BotId <= 0 {
		return gerror.New("缺少Bot ID")
	}
	var update models.Update
	if err := json.Unmarshal(in.Body, &update); err != nil {
		return gerror.Wrap(err, "解析Telegram webhook失败")
	}
	row, err := s.botByWebhookId(ctx, in.BotId)
	if err != nil {
		return err
	}
	if row.Status != sysin.TwoWayBotStatusEnabled {
		return nil
	}
	bot, err := s.telegramBot(ctx, row.BotToken)
	if err != nil {
		return err
	}
	return s.handleTelegramUpdate(ctx, bot, row, &update)
}

func (s *sSysTwoWayBot) handleTelegramUpdate(ctx context.Context, bot *tgbot.Bot, row *entity.YoubanTwoWayBotBot, update *models.Update) error {
	if row == nil || update == nil {
		return nil
	}
	msg := update.Message
	if msg == nil {
		msg = update.EditedMessage
	}
	if msg == nil || msg.Chat.ID == 0 {
		return nil
	}
	if fmt.Sprintf("%d", msg.Chat.ID) == strings.TrimSpace(row.SupergroupId) {
		return s.handleTopicMessage(ctx, bot, row, msg)
	}
	if strings.EqualFold(string(msg.Chat.Type), "private") {
		return s.handlePrivateMessage(ctx, bot, row, msg)
	}
	return nil
}

func (s *sSysTwoWayBot) handlePrivateMessage(ctx context.Context, bot *tgbot.Bot, row *entity.YoubanTwoWayBotBot, msg *models.Message) error {
	if msg.From == nil || msg.From.ID <= 0 {
		return nil
	}
	if strings.TrimSpace(row.SupergroupId) == "" {
		return gerror.New("双向机器人未配置管理群")
	}
	userId := fmt.Sprintf("%d", msg.From.ID)
	topic, err := s.ensureUserTopic(ctx, bot, row, msg.From)
	if err != nil {
		return err
	}
	target, err := bot.ForwardMessage(ctx, &tgbot.ForwardMessageParams{
		ChatID:          row.SupergroupId,
		MessageThreadID: int(topic.ThreadId),
		FromChatID:      msg.Chat.ID,
		MessageID:       msg.ID,
	})
	targetMessageId := 0
	if target != nil {
		targetMessageId = target.ID
	}
	if err != nil {
		copied, copyErr := bot.CopyMessage(ctx, &tgbot.CopyMessageParams{
			ChatID:          row.SupergroupId,
			MessageThreadID: int(topic.ThreadId),
			FromChatID:      msg.Chat.ID,
			MessageID:       msg.ID,
		})
		err = copyErr
		if copied != nil {
			targetMessageId = copied.ID
		}
	}
	status, message := "sent", ""
	if err != nil {
		status, message = "failed", err.Error()
	}
	_ = s.recordMessage(ctx, row, "in", userId, topic.ThreadId, fmt.Sprintf("%d", msg.Chat.ID), msg.ID, row.SupergroupId, targetMessageId, msg.MediaGroupID, status, message)
	if err != nil {
		return gerror.Wrap(err, "转发用户消息到话题失败")
	}
	return nil
}

func (s *sSysTwoWayBot) handleTopicMessage(ctx context.Context, bot *tgbot.Bot, row *entity.YoubanTwoWayBotBot, msg *models.Message) error {
	if msg.MessageThreadID <= 0 || msg.From == nil || msg.From.IsBot {
		return nil
	}
	userId := cachedThreadUser(ctx, row.Id, int64(msg.MessageThreadID))
	if userId == "" {
		topic, err := s.topicByThread(ctx, row, int64(msg.MessageThreadID))
		if err != nil {
			return err
		}
		if topic == nil || topic.TelegramUserId == "" {
			return nil
		}
		userId = topic.TelegramUserId
		cacheUserTopic(ctx, row.Id, userId, topic.ThreadId)
	}
	target, err := bot.CopyMessage(ctx, &tgbot.CopyMessageParams{
		ChatID:     userId,
		FromChatID: row.SupergroupId,
		MessageID:  msg.ID,
	})
	targetMessageId := 0
	if target != nil {
		targetMessageId = target.ID
	}
	status, message := "sent", ""
	if err != nil {
		status, message = "failed", err.Error()
	}
	_ = s.recordMessage(ctx, row, "out", userId, int64(msg.MessageThreadID), row.SupergroupId, msg.ID, userId, targetMessageId, msg.MediaGroupID, status, message)
	if err != nil {
		return gerror.Wrap(err, "发送话题回复到用户失败")
	}
	return nil
}

func (s *sSysTwoWayBot) ensureUserTopic(ctx context.Context, bot *tgbot.Bot, row *entity.YoubanTwoWayBotBot, user *models.User) (*entity.YoubanTwoWayBotTopic, error) {
	userId := fmt.Sprintf("%d", user.ID)
	if threadId := cachedUserThread(ctx, row.Id, userId); threadId > 0 {
		return &entity.YoubanTwoWayBotTopic{TenantId: row.TenantId, BotId: row.Id, TelegramUserId: userId, ThreadId: threadId}, nil
	}
	topic, err := s.topicByUser(ctx, row, userId)
	if err != nil {
		return nil, err
	}
	if topic != nil && topic.ThreadId > 0 {
		cacheUserTopic(ctx, row.Id, userId, topic.ThreadId)
		return topic, nil
	}
	title := truncateTopicTitle(fmt.Sprintf("%s %s", telegramUserTitle(user), userId))
	created, err := bot.CreateForumTopic(ctx, &tgbot.CreateForumTopicParams{ChatID: row.SupergroupId, Name: title})
	if err != nil {
		return nil, gerror.Wrap(err, "创建Telegram话题失败")
	}
	if created == nil || created.MessageThreadID <= 0 {
		return nil, gerror.New("创建Telegram话题失败")
	}
	now := gtime.Now()
	data := g.Map{
		"tenant_id":           row.TenantId,
		"bot_id":              row.Id,
		"telegram_user_id":    userId,
		"telegram_username":   strings.TrimPrefix(user.Username, "@"),
		"telegram_first_name": user.FirstName,
		"telegram_last_name":  user.LastName,
		"thread_id":           created.MessageThreadID,
		"title":               title,
		"closed":              0,
		"last_message_at":     now,
		"created_at":          now,
		"updated_at":          now,
	}
	id, err := twdao.YoubanTwoWayBotTopic.Ctx(ctx).Data(data).InsertAndGetId()
	if err != nil {
		return nil, gerror.Wrap(err, "保存Telegram话题失败")
	}
	topic = &entity.YoubanTwoWayBotTopic{Id: id, TenantId: row.TenantId, BotId: row.Id, TelegramUserId: userId, ThreadId: int64(created.MessageThreadID), Title: title}
	cacheUserTopic(ctx, row.Id, userId, topic.ThreadId)
	return topic, nil
}

func (s *sSysTwoWayBot) topicByUser(ctx context.Context, row *entity.YoubanTwoWayBotBot, userId string) (*entity.YoubanTwoWayBotTopic, error) {
	var topic *entity.YoubanTwoWayBotTopic
	err := twdao.YoubanTwoWayBotTopic.Ctx(ctx).
		Where("tenant_id", row.TenantId).
		Where("bot_id", row.Id).
		Where("telegram_user_id", userId).
		WhereNull("deleted_at").
		Scan(&topic)
	if err != nil {
		return nil, gerror.Wrap(err, "读取用户话题失败")
	}
	return topic, nil
}

func (s *sSysTwoWayBot) topicByThread(ctx context.Context, row *entity.YoubanTwoWayBotBot, threadId int64) (*entity.YoubanTwoWayBotTopic, error) {
	var topic *entity.YoubanTwoWayBotTopic
	err := twdao.YoubanTwoWayBotTopic.Ctx(ctx).
		Where("tenant_id", row.TenantId).
		Where("bot_id", row.Id).
		Where("thread_id", threadId).
		WhereNull("deleted_at").
		Scan(&topic)
	if err != nil {
		return nil, gerror.Wrap(err, "读取话题用户失败")
	}
	return topic, nil
}

func (s *sSysTwoWayBot) recordMessage(ctx context.Context, row *entity.YoubanTwoWayBotBot, direction string, userId string, threadId int64, sourceChatId string, sourceMessageId int, targetChatId string, targetMessageId int, mediaGroupId string, status string, message string) error {
	now := gtime.Now()
	_, err := twdao.YoubanTwoWayBotMessage.Ctx(ctx).Data(g.Map{
		"tenant_id":         row.TenantId,
		"bot_id":            row.Id,
		"direction":         direction,
		"telegram_user_id":  userId,
		"thread_id":         threadId,
		"source_chat_id":    sourceChatId,
		"source_message_id": sourceMessageId,
		"target_chat_id":    targetChatId,
		"target_message_id": targetMessageId,
		"media_group_id":    mediaGroupId,
		"status":            status,
		"error_message":     truncateText(message, 1000),
		"created_at":        now,
		"updated_at":        now,
	}).Insert()
	return err
}

func (s *sSysTwoWayBot) botByWebhookId(ctx context.Context, id int64) (*entity.YoubanTwoWayBotBot, error) {
	var row *entity.YoubanTwoWayBotBot
	err := twdao.YoubanTwoWayBotBot.Ctx(ctx).
		Where("id", id).
		WhereNull("deleted_at").
		Scan(&row)
	if err != nil {
		return nil, gerror.Wrap(err, "读取双向机器人失败")
	}
	if row == nil || row.Id <= 0 {
		return nil, gerror.New("双向机器人不存在")
	}
	return row, nil
}

func truncateTopicTitle(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "用户"
	}
	runes := []rune(text)
	if len(runes) > 60 {
		return string(runes[:60])
	}
	return text
}

func truncateText(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return string(runes)
}
