package sys

import (
	"context"
	"fmt"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/tg"

	botService "hotgo/addons/youban_bot/service"
)

func (s *sSysPublish) listenerNotifyMedia(ctx context.Context, plan accountListenPlanRuntime, notifyChatId string, sourceChatId string, messages []*tg.Message, caption string, buttonLabel string, buttonURL string) (bool, error) {
	messages = listenerNonNilMessages(messages)
	if len(messages) == 0 {
		return false, nil
	}
	media, err := s.listenerMessageMediaItems(ctx, plan, sourceChatId, messages)
	if err != nil {
		return false, err
	}
	if len(media) == 0 {
		g.Log().Warningf(ctx, "监听命中无可推送媒体 plan:%d sourceChat:%s messages:%d", plan.Id, sourceChatId, len(messages))
		return false, nil
	}
	botToken, err := botService.SysBot().OfficialBotToken(ctx)
	if err != nil {
		return true, err
	}
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return true, err
	}
	chatId := normalizeTelegramChannelChatID(notifyChatId)
	caption = listenerMediaCaption(caption, buttonLabel, buttonURL, len(media) > 1)
	if len(media) == 1 {
		_, err = s.sendTelegramSingleMediaWithMarkup(ctx, bot, chatId, "message_listen", caption, media[0], listenerInlineKeyboard(buttonLabel, buttonURL))
	} else {
		_, err = s.sendTelegramMediaSet(ctx, bot, chatId, "message_listen", caption, media)
	}
	if err != nil {
		return true, gerror.Wrap(err, "推送监听媒体失败")
	}
	g.Log().Infof(ctx, "监听媒体推送成功 plan:%d chat:%s media:%d", plan.Id, notifyChatId, len(media))
	return true, nil
}

func (s *sSysPublish) listenerMessageMediaItems(ctx context.Context, plan accountListenPlanRuntime, sourceChatId string, messages []*tg.Message) ([]*telegramMediaItem, error) {
	media := make([]*telegramMediaItem, 0, len(messages))
	for _, msg := range messages {
		items := gotdCollectMedia(msg, sourceChatId)
		if len(items) == 0 {
			g.Log().Debugf(ctx, "监听消息不包含可下载媒体 plan:%d message:%d", plan.Id, msg.ID)
			continue
		}
		item := items[0]
		downloaded, err := s.cachedGotdCollectMediaFile(ctx, plan.TenantId, plan.TgAccountId, item)
		if err != nil {
			g.Log().Warningf(ctx, "下载监听媒体失败 plan:%d sourceChat:%s message:%d type:%s fileId:%s err:%+v", plan.Id, sourceChatId, msg.ID, item.Type, item.FileId, err)
			return nil, gerror.Wrapf(err, "下载监听媒体失败 message:%d", msg.ID)
		}
		g.Log().Debugf(ctx, "监听媒体缓存完成 plan:%d message:%d type:%s path:%s", plan.Id, msg.ID, item.Type, downloaded.Path)
		media = append(media, &telegramMediaItem{
			Id:          int64(msg.ID),
			MediaType:   listenerTelegramMediaType(item.Type),
			Purpose:     "message_listen",
			StoragePath: downloaded.Path,
			SortIndex:   len(media),
			AssetHash:   fmt.Sprintf("listen:%d:%s", msg.ID, strings.TrimSpace(item.Type)),
		})
	}
	return media, nil
}

func listenerTelegramMediaType(value string) string {
	if strings.TrimSpace(value) == "video" {
		return "video"
	}
	return "photo"
}

func (s *sSysPublish) sendTelegramSingleMediaWithMarkup(ctx context.Context, bot *tgbot.Bot, chatId string, purpose string, caption string, media *telegramMediaItem, replyMarkup models.ReplyMarkup) ([]*telegramSentMessage, error) {
	if media == nil {
		return nil, nil
	}
	input, closer, err := telegramInputFile(ctx, media)
	if err != nil {
		return nil, err
	}
	if closer != nil {
		defer closer.Close()
	}
	switch media.MediaType {
	case "video":
		thumbnail, thumbnailCloser, err := telegramVideoThumbnail(ctx, media)
		if err != nil {
			return nil, err
		}
		if thumbnailCloser != nil {
			defer thumbnailCloser.Close()
		}
		params := &tgbot.SendVideoParams{
			ChatID:            chatId,
			Video:             input,
			Thumbnail:         thumbnail,
			Caption:           caption,
			ParseMode:         telegramMediaParseMode(caption),
			SupportsStreaming: true,
			ReplyMarkup:       replyMarkup,
		}
		applyTelegramSendVideoMeta(params, s.telegramVideoMeta(ctx, media))
		msg, err := bot.SendVideo(ctx, params)
		if err != nil {
			return nil, err
		}
		return telegramSentMessagesFromSingle(msg, purpose, media.Id, media.AssetHash)
	default:
		msg, err := bot.SendPhoto(ctx, &tgbot.SendPhotoParams{
			ChatID:      chatId,
			Photo:       input,
			Caption:     caption,
			ParseMode:   telegramMediaParseMode(caption),
			ReplyMarkup: replyMarkup,
		})
		if err != nil {
			return nil, err
		}
		return telegramSentMessagesFromSingle(msg, purpose, media.Id, media.AssetHash)
	}
}

func listenerInlineKeyboard(label string, url string) models.ReplyMarkup {
	label = strings.TrimSpace(label)
	url = strings.TrimSpace(url)
	if label == "" || url == "" {
		return nil
	}
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: label, URL: url}},
		},
	}
}

func listenerMediaCaption(caption string, buttonLabel string, buttonURL string, appendLink bool) string {
	caption = strings.TrimSpace(caption)
	if appendLink && strings.TrimSpace(buttonURL) != "" {
		label := strings.TrimSpace(buttonLabel)
		if label == "" {
			label = "查看用户"
		}
		caption = strings.TrimSpace(caption + "\n\n" + `<a href="` + telegramEscapeText(buttonURL) + `">` + telegramEscapeText(label) + `</a>`)
	}
	return truncateTelegramCaption(caption)
}

func truncateTelegramCaption(value string) string {
	const maxCaptionRunes = 1000
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maxCaptionRunes {
		return string(runes)
	}
	return string(runes[:maxCaptionRunes]) + "..."
}
