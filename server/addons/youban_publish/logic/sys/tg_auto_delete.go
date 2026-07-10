package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/tg"

	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

func telegramAllowedUpdateNames() []string {
	return []string{
		models.AllowedUpdateMessage,
		models.AllowedUpdateEditedMessage,
		models.AllowedUpdateChannelPost,
		models.AllowedUpdateEditedChannelPost,
	}
}

func telegramAllowedUpdates() tgbot.AllowedUpdates {
	return tgbot.AllowedUpdates(telegramAllowedUpdateNames())
}

func telegramUpdateMessage(update *models.Update) (*models.Message, string) {
	if update == nil {
		return nil, ""
	}
	switch {
	case update.ChannelPost != nil:
		return update.ChannelPost, models.AllowedUpdateChannelPost
	case update.EditedChannelPost != nil:
		return update.EditedChannelPost, models.AllowedUpdateEditedChannelPost
	case update.Message != nil:
		return update.Message, models.AllowedUpdateMessage
	case update.EditedMessage != nil:
		return update.EditedMessage, models.AllowedUpdateEditedMessage
	default:
		return nil, ""
	}
}

func telegramMessageText(msg *models.Message) string {
	if msg == nil {
		return ""
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		text = strings.TrimSpace(msg.Caption)
	}
	return text
}

func (s *sSysPublish) handleTelegramAutoDelete(ctx context.Context, botId int64, msg *models.Message, text string) {
	if msg == nil || msg.ID <= 0 || strings.TrimSpace(text) == "" {
		return
	}
	conf, err := service.SysConfig().AutoDeleteConfigView(ctx, &sysin.AutoDeleteConfigViewInp{})
	if err != nil || conf == nil || conf.AutoDeleteConfig == nil {
		g.Log().Warningf(ctx, "读取频道自动删除配置失败：%+v", err)
		return
	}
	if conf.Enabled != 1 {
		return
	}
	keyword := matchedAutoDeleteKeyword(text, conf.Keywords)
	if keyword == "" {
		return
	}
	channel, err := s.autoDeleteChannel(ctx, msg.Chat)
	if err != nil || channel == nil || channel.Id <= 0 {
		g.Log().Debugf(ctx, "频道自动删除跳过，频道未配置 chat:%d err:%+v", msg.Chat.ID, err)
		return
	}
	botItem, err := s.autoDeleteBot(ctx, botId, channel, conf.AutoDeleteConfig)
	if err != nil || botItem == nil || botItem.Id <= 0 {
		g.Log().Debugf(ctx, "频道自动删除跳过，Bot未配置 channel:%d bot:%d err:%+v", channel.Id, botId, err)
		return
	}
	if err = s.deleteMatchedTelegramMessage(ctx, botItem.BotToken, msg.Chat.ID, msg.ID); err != nil {
		s.appendAutoDeleteLog(ctx, channel, botItem.Id, msg, keyword, "failed", err.Error())
		g.Log().Warningf(ctx, "频道自动删除失败 channel:%d bot:%d message:%d err:%+v", channel.Id, botItem.Id, msg.ID, err)
		return
	}
	s.appendAutoDeleteLog(ctx, channel, botItem.Id, msg, keyword, "success", "频道消息命中关键词，已自动删除")
}

func (s *sSysPublish) handleGotdAutoDelete(ctx context.Context, msg *tg.Message) {
	if msg == nil || msg.ID <= 0 || strings.TrimSpace(msg.Message) == "" {
		return
	}
	conf, err := service.SysConfig().AutoDeleteConfigView(ctx, &sysin.AutoDeleteConfigViewInp{})
	if err != nil || conf == nil || conf.AutoDeleteConfig == nil {
		g.Log().Warningf(ctx, "读取频道自动删除配置失败：%+v", err)
		return
	}
	if conf.Enabled != 1 {
		return
	}
	keyword := matchedAutoDeleteKeyword(msg.Message, conf.Keywords)
	if keyword == "" {
		return
	}
	chatIds := gotdMessageChatIds(msg)
	channel, err := s.autoDeleteChannelByChatIds(ctx, chatIds)
	if err != nil || channel == nil || channel.Id <= 0 {
		g.Log().Debugf(ctx, "频道自动删除跳过，账号监听频道未配置 chats:%s err:%+v", strings.Join(chatIds, ","), err)
		return
	}
	botItem, err := s.autoDeleteBot(ctx, 0, channel, conf.AutoDeleteConfig)
	if err != nil || botItem == nil || botItem.Id <= 0 {
		g.Log().Debugf(ctx, "频道自动删除跳过，Bot未配置 channel:%d err:%+v", channel.Id, err)
		return
	}
	chatId := normalizeTelegramChannelChatID(channel.TargetChatId)
	if chatId == "" && len(chatIds) > 0 {
		chatId = normalizeTelegramChannelChatID(chatIds[0])
	}
	if err = s.deleteMatchedTelegramMessageByChat(ctx, botItem.BotToken, chatId, msg.ID); err != nil {
		if isTelegramMessageAlreadyDeletedError(err) {
			s.appendAutoDeleteLogByValues(ctx, channel, botItem.Id, msg.ID, keyword, "skipped", "频道消息命中关键词，TG消息已不存在")
			return
		}
		s.appendAutoDeleteLogByValues(ctx, channel, botItem.Id, msg.ID, keyword, "failed", err.Error())
		g.Log().Warningf(ctx, "频道自动删除失败 channel:%d bot:%d message:%d err:%+v", channel.Id, botItem.Id, msg.ID, err)
		return
	}
	s.appendAutoDeleteLogByValues(ctx, channel, botItem.Id, msg.ID, keyword, "success", "账号监听频道消息命中关键词，已自动删除")
}

func matchedAutoDeleteKeyword(text string, keywords []string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return ""
	}
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		if strings.Contains(text, strings.ToLower(keyword)) {
			return keyword
		}
	}
	return ""
}

func (s *sSysPublish) autoDeleteChannel(ctx context.Context, chat models.Chat) (*autoDeleteChannel, error) {
	chatId := strconv.FormatInt(chat.ID, 10)
	positiveChatId := strings.TrimPrefix(chatId, "-100")
	mod := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("status", 1).
		WhereNull("deleted_at")
	if username := strings.TrimPrefix(strings.TrimSpace(chat.Username), "@"); username != "" {
		mod = mod.Where("(target_chat_id IN(?, ?) OR channel_username = ?)", chatId, positiveChatId, username)
	} else {
		mod = mod.Where("target_chat_id IN(?, ?)", chatId, positiveChatId)
	}
	var channel *autoDeleteChannel
	if err := mod.Fields("id,tenant_id,bot_id_json,channel_title,target_chat_id").OrderDesc("id").Scan(&channel); err != nil {
		return nil, err
	}
	return channel, nil
}

func (s *sSysPublish) autoDeleteChannelByChatIds(ctx context.Context, chatIds []string) (*autoDeleteChannel, error) {
	values := make([]string, 0, len(chatIds)*2)
	for _, chatId := range chatIds {
		chatId = strings.TrimSpace(chatId)
		if chatId == "" {
			continue
		}
		values = append(values, chatId)
		if strings.HasPrefix(chatId, "-100") {
			values = append(values, strings.TrimPrefix(chatId, "-100"))
		}
	}
	values = uniqueStrings(values)
	if len(values) == 0 {
		return nil, nil
	}
	var channel *autoDeleteChannel
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("status", 1).
		WhereNull("deleted_at").
		WhereIn("target_chat_id", values).
		Fields("id,tenant_id,bot_id_json,channel_title,target_chat_id").
		OrderDesc("id").
		Scan(&channel); err != nil {
		return nil, err
	}
	return channel, nil
}

func (s *sSysPublish) autoDeleteBot(ctx context.Context, botId int64, channel *autoDeleteChannel, conf *model.AutoDeleteConfig) (*sysin.BotModel, error) {
	if botId > 0 {
		if !autoDeleteBotAllowed(botId, conf.BotIds) {
			return nil, nil
		}
		return s.getBotById(ctx, botId, channel.TenantId)
	}
	for _, id := range decodeBotIds(channel.BotIdJson) {
		if !autoDeleteBotAllowed(id, conf.BotIds) {
			continue
		}
		return s.getBotById(ctx, id, channel.TenantId)
	}
	for _, id := range conf.BotIds {
		return s.getBotById(ctx, id, channel.TenantId)
	}
	return nil, nil
}

func autoDeleteBotAllowed(botId int64, allowed []int64) bool {
	if botId <= 0 {
		return false
	}
	if len(allowed) == 0 {
		return true
	}
	for _, id := range allowed {
		if id == botId {
			return true
		}
	}
	return false
}

func (s *sSysPublish) deleteMatchedTelegramMessage(ctx context.Context, botToken string, chatId int64, messageId int) error {
	return s.deleteMatchedTelegramMessageByChat(ctx, botToken, normalizeTelegramChannelChatID(strconv.FormatInt(chatId, 10)), messageId)
}

func (s *sSysPublish) deleteMatchedTelegramMessageByChat(ctx context.Context, botToken string, chatId string, messageId int) error {
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	_, err = bot.DeleteMessage(ctx, &tgbot.DeleteMessageParams{
		ChatID:    normalizeTelegramChannelChatID(chatId),
		MessageID: messageId,
	})
	return err
}

func (s *sSysPublish) appendAutoDeleteLog(ctx context.Context, channel *autoDeleteChannel, botId int64, msg *models.Message, keyword string, status string, message string) {
	_, _ = g.DB().Model(publishTgJobLogTable).Safe().Ctx(ctx).Data(g.Map{
		"job_id":     0,
		"task_id":    0,
		"tenant_id":  channel.TenantId,
		"account_id": 0,
		"profile_id": 0,
		"bot_id":     botId,
		"action":     "auto_delete",
		"status":     status,
		"message":    fmt.Sprintf("%s；频道:%s(%s)；消息:%d；关键词:%s", message, channel.ChannelTitle, channel.TargetChatId, msg.ID, keyword),
		"created_at": gtime.Now(),
	}).Insert()
}

func (s *sSysPublish) appendAutoDeleteLogByValues(ctx context.Context, channel *autoDeleteChannel, botId int64, messageId int, keyword string, status string, message string) {
	_, _ = g.DB().Model(publishTgJobLogTable).Safe().Ctx(ctx).Data(g.Map{
		"job_id":     0,
		"task_id":    0,
		"tenant_id":  channel.TenantId,
		"account_id": 0,
		"profile_id": 0,
		"bot_id":     botId,
		"action":     "auto_delete",
		"status":     status,
		"message":    fmt.Sprintf("%s；频道:%s(%s)；消息:%d；关键词:%s", message, channel.ChannelTitle, channel.TargetChatId, messageId, keyword),
		"created_at": gtime.Now(),
	}).Insert()
}

type autoDeleteChannel struct {
	Id           int64  `json:"id"`
	TenantId     int64  `json:"tenantId"`
	BotIdJson    string `json:"botIdJson"`
	ChannelTitle string `json:"channelTitle"`
	TargetChatId string `json:"targetChatId"`
}
