package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"

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
	userId := fmt.Sprintf("%d", event.Msg.From.ID)
	groupId := strings.TrimSpace(event.Msg.MediaGroupID)
	if groupId != "" {
		if err = bot.acknowledgeScanMediaGroup(ctx, event.BotId, userId, event.Msg); err != nil {
			return true, err
		}
		if bot.scanMediaGroupResolved(ctx, event.BotId, userId, groupId) {
			return true, nil
		}
	}
	lookupStartedAt := time.Now()
	note, _, lookupErr := bot.lookupForwardedScanProfile(ctx, event.BotId, account, event.Msg)
	if lookupErr != nil {
		g.Log().Warning(ctx, "Bot转发资料快捷查询失败，降级扫图", g.Map{
			"botId": event.BotId, "tenantId": account.TenantId, "accountId": account.AccountId, "err": lookupErr,
		})
	}
	if note != nil && note.Id > 0 {
		if groupId != "" {
			claimed, claimErr := bot.claimScanMediaGroupResolved(ctx, event.BotId, userId, groupId)
			if claimErr != nil {
				return true, claimErr
			}
			if !claimed {
				return true, nil
			}
		}
		err = bot.sendProfileCard(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), note, profileCardPurpose(account, note, "view"))
		observeScanStage(ctx, event.BotId, "direct_reply", lookupStartedAt, err)
		observeScanRequest(ctx, event.BotId, scanResultLabel(err, "direct"))
		return true, err
	}
	resolveStartedAt := time.Now()
	media, err := bot.resolveTelegramMessageMedia(ctx, botTokenForEvent(ctx, bot, event.BotId), event.Msg)
	observeScanStage(ctx, event.BotId, "telegram_resolve", resolveStartedAt, err)
	if err != nil {
		observeScanRequest(ctx, event.BotId, "failed")
		return true, bot.replyBotError(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), "扫图搜索", err)
	}
	items := scanSearchItems(media)
	if len(items) == 0 {
		return true, bot.sendMessageOnly(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), "当前媒体没有可用的图片或视频预览图。")
	}
	if groupId != "" {
		return true, bot.collectScanMediaGroup(ctx, event.BotId, userId, event.Msg, items)
	}
	err = bot.searchScanMediaAndReply(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), account, items)
	observeScanRequest(ctx, event.BotId, scanResultLabel(err, "fingerprint"))
	return true, err
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
		if url == "" || (mediaType != "image" && mediaType != "video") {
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
