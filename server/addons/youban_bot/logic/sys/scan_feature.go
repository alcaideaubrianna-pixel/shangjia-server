package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
	"hotgo/utility/scantrace"
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
	// A single message has no album members to wait for. Do not put it behind
	// the album quiet window, including when its fingerprint is already cached.
	if strings.TrimSpace(event.Msg.MediaGroupID) == "" {
		traceId, ackErr := bot.acknowledgeScanMedia(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.From.ID), event.Msg)
		if ackErr != nil {
			return true, ackErr
		}
		claimed, claimErr := bot.claimScanMediaGroupResolved(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.From.ID), fmt.Sprintf("message:%d", event.Msg.ID))
		if claimErr != nil {
			return true, claimErr
		}
		if !claimed {
			return true, nil
		}
		ctx = withScanTrace(ctx, traceId)
		finish := scantrace.Begin(ctx, "telegram.resolve")
		items, resolveErr := bot.resolveScanSearchItems(ctx, event.BotId, account, event.Msg)
		finish(resolveErr)
		if resolveErr != nil {
			return true, bot.replyBotError(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), "扫图搜索", resolveErr)
		}
		if len(items) == 0 {
			return true, bot.sendMessageOnly(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), "当前媒体没有可用的图片或视频预览图。")
		}
		return true, bot.searchScanMediaAndReply(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), account, items)
	}
	userId := fmt.Sprintf("%d", event.Msg.From.ID)
	batchId, err := bot.scanMediaBatch(ctx, event.BotId, userId)
	if err != nil {
		return true, err
	}
	traceId, err := bot.acknowledgeScanMediaGroup(ctx, event.BotId, userId, batchId, event.Msg)
	if err != nil {
		return true, err
	}
	if bot.scanMediaGroupResolved(ctx, event.BotId, userId, batchId) {
		return true, nil
	}
	ctx = withScanTrace(ctx, traceId)
	g.Log().Info(ctx, "Bot扫图请求开始", g.Map{"traceId": traceId, "botId": event.BotId, "userId": userId, "batchId": batchId, "groupId": strings.TrimSpace(event.Msg.MediaGroupID), "messageId": event.Msg.ID})
	if err = bot.beginScanMediaGroupItem(ctx, event.BotId, userId, batchId, event.Msg); err != nil {
		return true, err
	}
	resolveStartedAt := time.Now()
	finish := scantrace.Begin(ctx, "telegram.resolve")
	items, err := bot.resolveScanSearchItems(ctx, event.BotId, account, event.Msg)
	finish(err)
	observeScanStage(ctx, event.BotId, "telegram_resolve", resolveStartedAt, err)
	if err != nil {
		_ = bot.completeScanMediaGroupItem(ctx, event.BotId, userId, batchId, nil)
		observeScanRequest(ctx, event.BotId, "failed")
		return true, bot.replyBotError(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), "扫图搜索", err)
	}
	if len(items) == 0 {
		_ = bot.completeScanMediaGroupItem(ctx, event.BotId, userId, batchId, nil)
		return true, bot.sendMessageOnly(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), "当前媒体没有可用的图片或视频预览图。")
	}
	return true, bot.completeScanMediaGroupItem(ctx, event.BotId, userId, batchId, items)
}

// Keep the original stable file identity for the send-ledger shortcut, but
// resolve only the thumbnail for video image search.
func scanMediaPreviewMessage(msg *models.Message) (*models.Message, error) {
	if msg == nil || msg.Video == nil {
		return msg, nil
	}
	if msg.Video.Thumbnail == nil || strings.TrimSpace(msg.Video.Thumbnail.FileID) == "" {
		return nil, gerror.New("视频没有可用的预览图，请发送视频封面图片")
	}
	preview := *msg
	preview.Video = nil
	preview.Photo = []models.PhotoSize{*msg.Video.Thumbnail}
	return &preview, nil
}

func (s *sSysBot) resolveTelegramScanMedia(ctx context.Context, botId int64, token string, msg *models.Message) ([]*publishsysin.MessageTemplateMediaInp, error) {
	preview, err := scanMediaPreviewMessage(msg)
	if err != nil {
		return nil, err
	}
	uniqueId := telegramMessageFileUniqueId(msg)
	if publishService.SysPublish().BotMediaFingerprintCached(ctx, uniqueId) {
		mediaType := "image"
		if msg != nil && msg.Video != nil {
			mediaType = "video"
		}
		// Cached fingerprints do not need a new Telegram getFile request or URL.
		return []*publishsysin.MessageTemplateMediaInp{{MediaType: mediaType, TgFileUniqueId: uniqueId}}, nil
	}
	items, err := s.resolveTelegramMessageMedia(ctx, botId, token, preview)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		item.TgFileUniqueId = telegramMessageFileUniqueId(msg)
		if msg != nil && msg.Video != nil {
			item.MediaType = "video"
			item.PosterUrl = item.FileUrl
			item.TgThumbFileId = msg.Video.Thumbnail.FileID
		}
	}
	return items, nil
}

func scanResultLabel(err error, success string) string {
	if err != nil {
		return "failed"
	}
	return success
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
		if (url == "" && strings.TrimSpace(item.TgFileUniqueId) == "") || (mediaType != "image" && mediaType != "video") {
			continue
		}
		items = append(items, &publishsysin.BotMediaSearchItem{
			FileUrl: url, MediaType: mediaType, FileUniqueId: strings.TrimSpace(item.TgFileUniqueId),
		})
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
