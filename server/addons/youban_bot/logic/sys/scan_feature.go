package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"

	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
)

type scanFeature struct{}

func (scanFeature) Key() string         { return "image_scan" }
func (scanFeature) Command() string     { return "scan" }
func (scanFeature) Description() string { return "扫图搜索" }
func (scanFeature) ConfigSchema() []*botsysin.FeatureConfigSchema {
	return []*botsysin.FeatureConfigSchema{{Field: "replyText", Label: "使用说明", Component: "textarea", Default: "请直接发送图片、视频或媒体组进行扫图搜索。"}}
}
func (scanFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	if featureCtx == nil || featureCtx.Msg == nil {
		return true, nil
	}
	text := bot.featureConfigValue(ctx, scanFeature{}.Key(), "replyText")
	return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), text)
}

type scanMediaMessageHandler struct{}

func (scanMediaMessageHandler) Handle(ctx context.Context, bot *sSysBot, event *botMessageEvent) (bool, error) {
	if event == nil || event.Msg == nil || event.Msg.From == nil || !hasScanMedia(event.Msg) {
		return false, nil
	}
	if !isTelegramPrivateChat(event.Msg) {
		return false, nil
	}
	if _, enabled := bot.featureConfig(ctx, scanFeature{}.Key()); !enabled {
		return false, nil
	}
	account, err := bot.boundProfileAccountByUser(ctx, event.Msg.From.ID)
	if err != nil {
		return true, bot.replyBotError(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), "扫图搜索", err)
	}
	if err = publishService.SysPublish().EnsureBotMediaSearchAccess(ctx, account.TenantId); err != nil {
		return true, bot.replyBotError(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), "扫图搜索", err)
	}
	media, err := bot.resolveTelegramMessageMedia(ctx, botTokenForEvent(ctx, bot, event.BotId), event.Msg)
	if err != nil {
		return true, bot.replyBotError(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), "扫图搜索", err)
	}
	items := scanSearchItems(media)
	if len(items) == 0 {
		return true, bot.sendMessageOnly(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), "当前媒体没有可用的图片或视频预览图。")
	}
	userId := fmt.Sprintf("%d", event.Msg.From.ID)
	if groupId := strings.TrimSpace(event.Msg.MediaGroupID); groupId != "" {
		return true, bot.collectScanMediaGroup(ctx, event.BotId, userId, event.Msg, items)
	}
	return true, bot.searchScanMediaAndReply(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), account, items)
}

func isTelegramPrivateChat(msg *models.Message) bool {
	return msg != nil && strings.EqualFold(strings.TrimSpace(string(msg.Chat.Type)), "private")
}

func hasScanMedia(msg *models.Message) bool {
	return msg != nil && (len(msg.Photo) > 0 || msg.Video != nil)
}

func scanSearchItems(media []*publishsysin.MessageTemplateMediaInp) []*publishsysin.BotMediaSearchItem {
	items := make([]*publishsysin.BotMediaSearchItem, 0, len(media))
	for _, item := range media {
		if item == nil {
			continue
		}
		url := strings.TrimSpace(item.FileUrl)
		mediaType := strings.ToLower(strings.TrimSpace(item.MediaType))
		if mediaType == "video" {
			url = strings.TrimSpace(item.PosterUrl)
		}
		if url == "" || (mediaType != "image" && mediaType != "video") {
			continue
		}
		items = append(items, &publishsysin.BotMediaSearchItem{FileUrl: url, MediaType: mediaType})
	}
	return items
}

func botTokenForEvent(ctx context.Context, bot *sSysBot, botId int64) string {
	row, err := bot.botById(ctx, botId)
	if err != nil || row == nil {
		return ""
	}
	return row.BotToken
}
