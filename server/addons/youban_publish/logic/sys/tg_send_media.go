package sys

import (
	"context"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
)

func (s *sSysPublish) sendTelegramMediaSet(ctx context.Context, bot *tgbot.Bot, chatId string, purpose string, caption string, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
	if len(media) == 0 {
		return nil, nil
	}
	if len(media) == 1 {
		return s.sendTelegramSingleMedia(ctx, bot, chatId, purpose, caption, media[0])
	}
	group, mediaIds := s.telegramInputMediaGroup(media, caption)
	if len(group) == 0 {
		return nil, nil
	}
	msgs, err := bot.SendMediaGroup(ctx, &tgbot.SendMediaGroupParams{
		ChatID: chatId,
		Media:  group,
	})
	if err != nil {
		return nil, err
	}
	return telegramSentMessagesFromGroup(msgs, purpose, mediaIds), nil
}

func (s *sSysPublish) sendTelegramSingleMedia(ctx context.Context, bot *tgbot.Bot, chatId string, purpose string, caption string, media *telegramMediaItem) ([]*telegramSentMessage, error) {
	source := telegramMediaSource(media)
	if source == "" {
		return nil, gerror.New("媒体文件地址为空")
	}
	switch media.MediaType {
	case "video":
		msg, err := bot.SendVideo(ctx, &tgbot.SendVideoParams{
			ChatID:  chatId,
			Video:   &models.InputFileString{Data: source},
			Caption: caption,
		})
		if err != nil {
			return nil, err
		}
		return telegramSentMessagesFromSingle(msg, purpose, media.Id)
	default:
		msg, err := bot.SendPhoto(ctx, &tgbot.SendPhotoParams{
			ChatID:  chatId,
			Photo:   &models.InputFileString{Data: source},
			Caption: caption,
		})
		if err != nil {
			return nil, err
		}
		return telegramSentMessagesFromSingle(msg, purpose, media.Id)
	}
}

func (s *sSysPublish) telegramInputMediaGroup(media []*telegramMediaItem, caption string) ([]models.InputMedia, []int64) {
	group := make([]models.InputMedia, 0, len(media))
	mediaIds := make([]int64, 0, len(media))
	for _, item := range media {
		source := telegramMediaSource(item)
		if source == "" {
			continue
		}
		itemCaption := ""
		if len(group) == 0 {
			itemCaption = caption
		}
		if item.MediaType == "video" {
			group = append(group, &models.InputMediaVideo{Media: source, Caption: itemCaption})
		} else {
			group = append(group, &models.InputMediaPhoto{Media: source, Caption: itemCaption})
		}
		mediaIds = append(mediaIds, item.Id)
	}
	return group, mediaIds
}

func telegramMediaSource(media *telegramMediaItem) string {
	if media == nil {
		return ""
	}
	if source := strings.TrimSpace(media.TgFileId); source != "" {
		return source
	}
	return strings.TrimSpace(media.FileUrl)
}

func telegramSentMessagesFromGroup(msgs []*models.Message, purpose string, mediaIds []int64) []*telegramSentMessage {
	list := make([]*telegramSentMessage, 0, len(msgs))
	for i, msg := range msgs {
		if msg == nil {
			continue
		}
		mediaId := int64(0)
		if i < len(mediaIds) {
			mediaId = mediaIds[i]
		}
		list = append(list, &telegramSentMessage{
			MessageId:    int64(msg.ID),
			MediaGroupId: msg.MediaGroupID,
			Purpose:      purpose,
			MediaId:      mediaId,
			TgFileId:     telegramMessageFileId(msg),
		})
	}
	return list
}

func telegramSentMessagesFromSingle(msg *models.Message, purpose string, mediaId int64) ([]*telegramSentMessage, error) {
	if msg == nil {
		return nil, nil
	}
	return []*telegramSentMessage{{
		MessageId:    int64(msg.ID),
		MediaGroupId: msg.MediaGroupID,
		Purpose:      purpose,
		MediaId:      mediaId,
		TgFileId:     telegramMessageFileId(msg),
	}}, nil
}
