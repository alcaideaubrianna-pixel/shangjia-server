package sys

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	twdao "hotgo/addons/youban_two_way_bot/internal/dao"
	"hotgo/addons/youban_two_way_bot/internal/model/entity"
	"hotgo/internal/library/cache"
)

const twoWayMediaGroupFlushDelay = 1200 * time.Millisecond

type twoWayMediaGroupItem struct {
	TenantId        int64  `json:"tenantId"`
	BotId           int64  `json:"botId"`
	Direction       string `json:"direction"`
	TelegramUserId  string `json:"telegramUserId"`
	ThreadId        int64  `json:"threadId"`
	SourceChatId    string `json:"sourceChatId"`
	SourceMessageId int    `json:"sourceMessageId"`
	TargetChatId    string `json:"targetChatId"`
	MediaGroupId    string `json:"mediaGroupId"`
}

func contactUserMarkup(userId string) *models.InlineKeyboardMarkup {
	userId = strings.TrimSpace(userId)
	if userId == "" {
		return nil
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		{Text: "联系用户", URL: "tg://user?id=" + userId},
	}}}
}

func (s *sSysTwoWayBot) sendNewTelegramMessage(ctx context.Context, bot *tgbot.Bot, chatId string, threadId int, msg *models.Message, markup models.ReplyMarkup) (*models.Message, error) {
	if msg == nil {
		return nil, gerror.New("Telegram消息为空")
	}
	caption := strings.TrimSpace(msg.Caption)
	if msg.Text != "" {
		return bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: chatId, MessageThreadID: threadId, Text: msg.Text, ReplyMarkup: markup})
	}
	if len(msg.Photo) > 0 {
		return bot.SendPhoto(ctx, &tgbot.SendPhotoParams{ChatID: chatId, MessageThreadID: threadId, Photo: inputFile(msg.Photo[len(msg.Photo)-1].FileID), Caption: caption, ReplyMarkup: markup})
	}
	if msg.Video != nil {
		return bot.SendVideo(ctx, &tgbot.SendVideoParams{ChatID: chatId, MessageThreadID: threadId, Video: inputFile(msg.Video.FileID), Duration: msg.Video.Duration, Width: msg.Video.Width, Height: msg.Video.Height, Caption: caption, SupportsStreaming: true, ReplyMarkup: markup})
	}
	if msg.Animation != nil {
		return bot.SendAnimation(ctx, &tgbot.SendAnimationParams{ChatID: chatId, MessageThreadID: threadId, Animation: inputFile(msg.Animation.FileID), Duration: msg.Animation.Duration, Width: msg.Animation.Width, Height: msg.Animation.Height, Caption: caption, ReplyMarkup: markup})
	}
	if msg.Document != nil {
		return bot.SendDocument(ctx, &tgbot.SendDocumentParams{ChatID: chatId, MessageThreadID: threadId, Document: inputFile(msg.Document.FileID), Caption: caption, ReplyMarkup: markup})
	}
	if msg.Audio != nil {
		return bot.SendAudio(ctx, &tgbot.SendAudioParams{ChatID: chatId, MessageThreadID: threadId, Audio: inputFile(msg.Audio.FileID), Duration: msg.Audio.Duration, Performer: msg.Audio.Performer, Title: msg.Audio.Title, Caption: caption, ReplyMarkup: markup})
	}
	if msg.Voice != nil {
		return bot.SendVoice(ctx, &tgbot.SendVoiceParams{ChatID: chatId, MessageThreadID: threadId, Voice: inputFile(msg.Voice.FileID), Duration: msg.Voice.Duration, Caption: caption, ReplyMarkup: markup})
	}
	return bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: chatId, MessageThreadID: threadId, Text: caption, ReplyMarkup: markup})
}

func inputFile(fileId string) models.InputFile {
	return &models.InputFileString{Data: strings.TrimSpace(fileId)}
}

func (s *sSysTwoWayBot) enqueueMediaGroup(ctx context.Context, bot *tgbot.Bot, row *entity.YoubanTwoWayBotBot, item twoWayMediaGroupItem, msg *models.Message) error {
	if msg == nil {
		return gerror.New("Telegram媒体组消息为空")
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return gerror.Wrap(err, "缓存Telegram媒体组消息失败")
	}
	messageKey := mediaGroupMessageCacheKey(row.Id, item.Direction, item.MediaGroupId, item.SourceChatId, item.SourceMessageId)
	if err = cache.Instance().Set(ctx, messageKey, string(payload), twoWayMediaGroupCacheTTL); err != nil {
		return gerror.Wrap(err, "缓存Telegram媒体文件失败")
	}
	_ = s.recordMessage(ctx, row, item.Direction, item.TelegramUserId, item.ThreadId, item.SourceChatId, item.SourceMessageId, item.TargetChatId, 0, item.MediaGroupId, "queued", "")
	flushKey := mediaGroupFlushCacheKey(row.Id, item.Direction, item.MediaGroupId)
	ok, err := cache.Instance().SetIfNotExist(ctx, flushKey, 1, twoWayMediaGroupCacheTTL)
	if err != nil || !ok {
		return err
	}
	go func() {
		flushCtx := context.WithoutCancel(ctx)
		time.Sleep(twoWayMediaGroupFlushDelay)
		if err := s.flushMediaGroup(flushCtx, bot, row, item.Direction, item.MediaGroupId); err != nil {
			g.Log().Warningf(flushCtx, "重新发送双向机器人媒体组失败 botId:%d mediaGroupId:%s err:%+v", row.Id, item.MediaGroupId, err)
		}
	}()
	return nil
}

func (s *sSysTwoWayBot) flushMediaGroup(ctx context.Context, bot *tgbot.Bot, row *entity.YoubanTwoWayBotBot, direction string, groupId string) error {
	_, _ = cache.Instance().Remove(ctx, mediaGroupFlushCacheKey(row.Id, direction, groupId))
	var rows []*entity.YoubanTwoWayBotMessage
	err := twdao.YoubanTwoWayBotMessage.Ctx(ctx).
		Where("tenant_id", row.TenantId).Where("bot_id", row.Id).
		Where("direction", direction).Where("media_group_id", groupId).
		Where("status", "queued").OrderAsc("source_message_id").Scan(&rows)
	if err != nil || len(rows) == 0 {
		return err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].SourceMessageId < rows[j].SourceMessageId })
	messages := make([]*models.Message, 0, len(rows))
	for _, queued := range rows {
		message, loadErr := s.cachedMediaGroupMessage(ctx, row.Id, direction, groupId, queued.SourceChatId, queued.SourceMessageId)
		if loadErr != nil {
			return loadErr
		}
		messages = append(messages, message)
	}
	targetThreadId := 0
	if direction == "in" {
		targetThreadId = int(rows[0].ThreadId)
	}
	targetIds, err := sendNewTelegramMediaGroup(ctx, bot, rows[0].TargetChatId, targetThreadId, messages)
	status, errorMessage := "sent", ""
	if err != nil {
		status, errorMessage = "failed", err.Error()
	}
	for index, queued := range rows {
		targetMessageId := 0
		if index < len(targetIds) && targetIds[index] != nil {
			targetMessageId = targetIds[index].ID
		}
		_, _ = twdao.YoubanTwoWayBotMessage.Ctx(ctx).WherePri(queued.Id).Data(g.Map{
			"target_message_id": targetMessageId, "status": status,
			"error_message": truncateText(errorMessage, 1000), "updated_at": gtime.Now(),
		}).Update()
	}
	if err != nil {
		return err
	}
	if direction == "in" {
		if topic, topicErr := s.topicByThread(ctx, row, rows[0].ThreadId); topicErr == nil && topic != nil {
			_ = s.touchTopic(ctx, topic.Id)
		}
		_, _ = bot.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: rows[0].TargetChatId, MessageThreadID: targetThreadId,
			Text: "发送人：", ReplyMarkup: contactUserMarkup(rows[0].TelegramUserId),
		})
	}
	return nil
}

func (s *sSysTwoWayBot) cachedMediaGroupMessage(ctx context.Context, botId int64, direction, groupId, sourceChatId string, sourceMessageId int) (*models.Message, error) {
	key := mediaGroupMessageCacheKey(botId, direction, groupId, sourceChatId, sourceMessageId)
	value, err := cache.Instance().Get(ctx, key)
	if err != nil || value == nil || value.IsNil() {
		return nil, gerror.New("Telegram媒体组缓存已过期，请重新发送")
	}
	message := &models.Message{}
	if err = json.Unmarshal([]byte(value.String()), message); err != nil {
		return nil, gerror.Wrap(err, "解析Telegram媒体组缓存失败")
	}
	return message, nil
}

func sendNewTelegramMediaGroup(ctx context.Context, bot *tgbot.Bot, chatId string, threadId int, messages []*models.Message) ([]*models.Message, error) {
	media := make([]models.InputMedia, 0, len(messages))
	for _, message := range messages {
		input, err := inputMediaFromMessage(message)
		if err != nil {
			return nil, err
		}
		media = append(media, input)
	}
	if len(media) == 1 {
		message, err := sendNewTelegramMessageWithoutMarkup(ctx, bot, chatId, threadId, messages[0])
		if err != nil {
			return nil, err
		}
		return []*models.Message{message}, nil
	}
	return bot.SendMediaGroup(ctx, &tgbot.SendMediaGroupParams{ChatID: chatId, MessageThreadID: threadId, Media: media})
}

func sendNewTelegramMessageWithoutMarkup(ctx context.Context, bot *tgbot.Bot, chatId string, threadId int, message *models.Message) (*models.Message, error) {
	return (&sSysTwoWayBot{}).sendNewTelegramMessage(ctx, bot, chatId, threadId, message, nil)
}

func inputMediaFromMessage(message *models.Message) (models.InputMedia, error) {
	if message == nil {
		return nil, gerror.New("Telegram媒体消息为空")
	}
	caption := strings.TrimSpace(message.Caption)
	if len(message.Photo) > 0 {
		return &models.InputMediaPhoto{Media: message.Photo[len(message.Photo)-1].FileID, Caption: caption}, nil
	}
	if message.Video != nil {
		return &models.InputMediaVideo{Media: message.Video.FileID, Caption: caption, Width: message.Video.Width, Height: message.Video.Height, Duration: message.Video.Duration, SupportsStreaming: true}, nil
	}
	if message.Document != nil {
		return &models.InputMediaDocument{Media: message.Document.FileID, Caption: caption}, nil
	}
	if message.Audio != nil {
		return &models.InputMediaAudio{Media: message.Audio.FileID, Caption: caption, Duration: message.Audio.Duration, Performer: message.Audio.Performer, Title: message.Audio.Title}, nil
	}
	return nil, gerror.New("当前媒体类型不支持媒体组重新发送")
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
