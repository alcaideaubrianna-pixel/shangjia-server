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
	"hotgo/internal/library/cache"
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
	if handled, err := s.handleCooperationCallback(ctx, bot, row, update.CallbackQuery); handled || err != nil {
		return err
	}
	msg := update.Message
	if msg == nil {
		msg = update.EditedMessage
	}
	if msg == nil || msg.Chat.ID == 0 {
		return nil
	}
	if fmt.Sprintf("%d", msg.Chat.ID) == strings.TrimSpace(row.SupergroupId) {
		if msg.ForumTopicClosed != nil && msg.MessageThreadID > 0 {
			return s.setTopicClosed(ctx, row, int64(msg.MessageThreadID), true)
		}
		if msg.ForumTopicReopened != nil && msg.MessageThreadID > 0 {
			return s.setTopicClosed(ctx, row, int64(msg.MessageThreadID), false)
		}
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
	if handled, err := s.handleCooperationPrivateMessage(ctx, bot, row, msg); handled || err != nil {
		return err
	}
	if strings.TrimSpace(row.SupergroupId) == "" {
		return gerror.New("双向机器人未配置管理群")
	}
	userId := fmt.Sprintf("%d", msg.From.ID)
	if s.isUserBanned(ctx, row, userId) {
		return nil
	}
	topic, err := s.ensureUserTopic(ctx, bot, row, msg.From)
	if err != nil {
		return err
	}
	if topic.Closed == 1 {
		_, _ = bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: msg.Chat.ID, Text: "当前对话已关闭。"})
		return nil
	}
	if msg.MediaGroupID != "" {
		return s.enqueueMediaGroup(ctx, bot, row, twoWayMediaGroupItem{
			TenantId:        row.TenantId,
			BotId:           row.Id,
			Direction:       "in",
			TelegramUserId:  userId,
			ThreadId:        topic.ThreadId,
			SourceChatId:    fmt.Sprintf("%d", msg.Chat.ID),
			SourceMessageId: msg.ID,
			TargetChatId:    row.SupergroupId,
			MediaGroupId:    msg.MediaGroupID,
		}, msg)
	}
	target, err := s.sendNewTelegramMessage(ctx, bot, row.SupergroupId, int(topic.ThreadId), msg, contactUserMarkup(userId))
	targetMessageId := 0
	if target != nil {
		targetMessageId = target.ID
	}
	status, message := "sent", ""
	if err != nil {
		status, message = "failed", err.Error()
	}
	_ = s.recordMessage(ctx, row, "in", userId, topic.ThreadId, fmt.Sprintf("%d", msg.Chat.ID), msg.ID, row.SupergroupId, targetMessageId, msg.MediaGroupID, status, message)
	if err != nil {
		if isTelegramTopicMissing(err) {
			removeCachedUserTopic(ctx, row.Id, userId, topic.ThreadId)
		}
		return gerror.Wrap(err, "转发用户消息到话题失败")
	}
	_ = s.touchTopic(ctx, topic.Id)
	return nil
}

func (s *sSysTwoWayBot) handleTopicMessage(ctx context.Context, bot *tgbot.Bot, row *entity.YoubanTwoWayBotBot, msg *models.Message) error {
	if msg.MessageThreadID <= 0 || msg.From == nil || msg.From.IsBot {
		return nil
	}
	threadId := int64(msg.MessageThreadID)
	userId := cachedThreadUser(ctx, row.Id, threadId)
	var topic *entity.YoubanTwoWayBotTopic
	if userId == "" {
		var err error
		topic, err = s.topicByThread(ctx, row, threadId)
		if err != nil {
			return err
		}
		if topic == nil || topic.TelegramUserId == "" {
			return nil
		}
		userId = topic.TelegramUserId
		cacheUserTopic(ctx, row.Id, userId, topic.ThreadId)
	} else {
		var err error
		topic, err = s.topicByThread(ctx, row, threadId)
		if err != nil {
			return err
		}
	}
	if s.isAdminCommand(msg) {
		ok, err := s.isGroupAdmin(ctx, bot, row, msg.From.ID)
		if err != nil || !ok {
			if err != nil {
				g.Log().Warningf(ctx, "校验双向机器人管理员失败 botId:%d userId:%d err:%+v", row.Id, msg.From.ID, err)
			}
			return nil
		}
		return s.handleAdminCommand(ctx, bot, row, topic, msg, userId)
	}
	if topic != nil && topic.Closed == 1 {
		return nil
	}
	if msg.MediaGroupID != "" {
		return s.enqueueMediaGroup(ctx, bot, row, twoWayMediaGroupItem{
			TenantId:        row.TenantId,
			BotId:           row.Id,
			Direction:       "out",
			TelegramUserId:  userId,
			ThreadId:        threadId,
			SourceChatId:    row.SupergroupId,
			SourceMessageId: msg.ID,
			TargetChatId:    userId,
			MediaGroupId:    msg.MediaGroupID,
		}, msg)
	}
	target, err := s.sendNewTelegramMessage(ctx, bot, userId, 0, msg, nil)
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
	if topic != nil {
		_ = s.touchTopic(ctx, topic.Id)
	}
	return nil
}

func (s *sSysTwoWayBot) ensureUserTopic(ctx context.Context, bot *tgbot.Bot, row *entity.YoubanTwoWayBotBot, user *models.User) (*entity.YoubanTwoWayBotTopic, error) {
	userId := fmt.Sprintf("%d", user.ID)
	if threadId := cachedUserThread(ctx, row.Id, userId); threadId > 0 {
		if topic, err := s.topicByUser(ctx, row, userId); err != nil {
			return nil, err
		} else if topic != nil && topic.ThreadId > 0 {
			if topic.ThreadId != threadId {
				cacheUserTopic(ctx, row.Id, userId, topic.ThreadId)
			}
			return topic, nil
		}
		removeCachedUserTopic(ctx, row.Id, userId, threadId)
	}
	topic, err := s.topicByUser(ctx, row, userId)
	if err != nil {
		return nil, err
	}
	if topic != nil && topic.ThreadId > 0 {
		cacheUserTopic(ctx, row.Id, userId, topic.ThreadId)
		return topic, nil
	}
	title := truncateTopicTitle(telegramUserTitle(user))
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

func (s *sSysTwoWayBot) handleAdminCommand(ctx context.Context, bot *tgbot.Bot, row *entity.YoubanTwoWayBotBot, topic *entity.YoubanTwoWayBotTopic, msg *models.Message, userId string) error {
	command := normalizeBotCommand(msg.Text)
	threadId := int64(msg.MessageThreadID)
	switch command {
	case "/close":
		if err := s.setTopicClosed(ctx, row, threadId, true); err != nil {
			return err
		}
		_ = s.sendTopicNotice(ctx, bot, row, msg.MessageThreadID, "对话已关闭。")
		_, _ = bot.CloseForumTopic(ctx, &tgbot.CloseForumTopicParams{ChatID: row.SupergroupId, MessageThreadID: msg.MessageThreadID})
		return nil
	case "/open":
		if err := s.setTopicClosed(ctx, row, threadId, false); err != nil {
			return err
		}
		_, _ = bot.ReopenForumTopic(ctx, &tgbot.ReopenForumTopicParams{ChatID: row.SupergroupId, MessageThreadID: msg.MessageThreadID})
		return s.sendTopicNotice(ctx, bot, row, msg.MessageThreadID, "对话已恢复。")
	case "/ban":
		_ = cache.Instance().Set(ctx, bannedUserCacheKey(row.Id, userId), 1, twoWayStateCacheTTL)
		return s.sendTopicNotice(ctx, bot, row, msg.MessageThreadID, "用户已封禁。")
	case "/unban":
		_, _ = cache.Instance().Remove(ctx, bannedUserCacheKey(row.Id, userId))
		return s.sendTopicNotice(ctx, bot, row, msg.MessageThreadID, "用户已解封。")
	case "/trust":
		_ = cache.Instance().Set(ctx, trustedUserCacheKey(row.Id, userId), 1, twoWayStateCacheTTL)
		_, _ = cache.Instance().Remove(ctx, bannedUserCacheKey(row.Id, userId))
		return s.sendTopicNotice(ctx, bot, row, msg.MessageThreadID, "用户已标记为信任。")
	case "/reset":
		removeCachedUserTopic(ctx, row.Id, userId, threadId)
		return s.sendTopicNotice(ctx, bot, row, msg.MessageThreadID, "话题缓存已重置。")
	case "/info":
		title := ""
		closed := 0
		if topic != nil {
			title = topic.Title
			closed = topic.Closed
		}
		text := fmt.Sprintf("用户ID：%s\n话题ID：%d\n话题标题：%s\n关闭状态：%d\n封禁状态：%t", userId, threadId, title, closed, s.isUserBanned(ctx, row, userId))
		return s.sendTopicNotice(ctx, bot, row, msg.MessageThreadID, text)
	case "/cleanup":
		removeCachedUserTopic(ctx, row.Id, userId, threadId)
		return s.sendTopicNotice(ctx, bot, row, msg.MessageThreadID, "当前话题缓存已清理。")
	default:
		return nil
	}
}

func (s *sSysTwoWayBot) isAdminCommand(msg *models.Message) bool {
	return normalizeBotCommand(msg.Text) != ""
}

func normalizeBotCommand(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || !strings.HasPrefix(text, "/") {
		return ""
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	command := strings.ToLower(fields[0])
	if i := strings.Index(command, "@"); i >= 0 {
		command = command[:i]
	}
	switch command {
	case "/cleanup", "/close", "/open", "/reset", "/trust", "/ban", "/unban", "/info":
		return command
	default:
		return ""
	}
}

func (s *sSysTwoWayBot) sendTopicNotice(ctx context.Context, bot *tgbot.Bot, row *entity.YoubanTwoWayBotBot, threadId int, text string) error {
	_, err := bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: row.SupergroupId, MessageThreadID: threadId, Text: text})
	return err
}

func (s *sSysTwoWayBot) isGroupAdmin(ctx context.Context, bot *tgbot.Bot, row *entity.YoubanTwoWayBotBot, userId int64) (bool, error) {
	if userId <= 0 {
		return false, nil
	}
	key := adminUserCacheKey(row.Id, userId)
	if value, err := cache.Instance().Get(ctx, key); err == nil && value != nil {
		if value.Bool() {
			return true, nil
		}
		_, _ = cache.Instance().Remove(ctx, key)
	}
	member, memberErr := bot.GetChatMember(ctx, &tgbot.GetChatMemberParams{ChatID: row.SupergroupId, UserID: userId})
	ok := memberErr == nil && member != nil && (member.Type == models.ChatMemberTypeOwner || member.Type == models.ChatMemberTypeAdministrator)
	if !ok {
		administrators, administratorsErr := bot.GetChatAdministrators(ctx, &tgbot.GetChatAdministratorsParams{ChatID: row.SupergroupId})
		if administratorsErr == nil {
			for _, administrator := range administrators {
				administratorUserId := int64(0)
				switch administrator.Type {
				case models.ChatMemberTypeOwner:
					if administrator.Owner != nil && administrator.Owner.User != nil {
						administratorUserId = administrator.Owner.User.ID
					}
				case models.ChatMemberTypeAdministrator:
					if administrator.Administrator != nil {
						administratorUserId = administrator.Administrator.User.ID
					}
				}
				if administratorUserId == userId {
					ok = true
					break
				}
			}
		} else if memberErr != nil {
			return false, gerror.Wrapf(administratorsErr, "读取管理群管理员失败，GetChatMember错误：%v", memberErr)
		}
	}
	if ok {
		_ = cache.Instance().Set(ctx, key, true, twoWayAdminCacheTTL)
	}
	return ok, nil
}

func (s *sSysTwoWayBot) isUserBanned(ctx context.Context, row *entity.YoubanTwoWayBotBot, userId string) bool {
	value, err := cache.Instance().Get(ctx, bannedUserCacheKey(row.Id, userId))
	return err == nil && value != nil && value.Bool()
}

func (s *sSysTwoWayBot) setTopicClosed(ctx context.Context, row *entity.YoubanTwoWayBotBot, threadId int64, closed bool) error {
	closedValue := 0
	if closed {
		closedValue = 1
	}
	_, err := twdao.YoubanTwoWayBotTopic.Ctx(ctx).
		Where("tenant_id", row.TenantId).
		Where("bot_id", row.Id).
		Where("thread_id", threadId).
		WhereNull("deleted_at").
		Data(g.Map{"closed": closedValue, "updated_at": gtime.Now()}).
		Update()
	return err
}

func (s *sSysTwoWayBot) touchTopic(ctx context.Context, topicId int64) error {
	if topicId <= 0 {
		return nil
	}
	now := gtime.Now()
	_, err := twdao.YoubanTwoWayBotTopic.Ctx(ctx).WherePri(topicId).Data(g.Map{"last_message_at": now, "updated_at": now}).Update()
	return err
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

func isTelegramTopicMissing(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "message thread not found") ||
		strings.Contains(text, "topic not found") ||
		strings.Contains(text, "message thread invalid") ||
		strings.Contains(text, "forum topic") && strings.Contains(text, "not found")
}
