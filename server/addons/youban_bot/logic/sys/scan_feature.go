package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gctx"

	botsysin "hotgo/addons/youban_bot/model/input/sysin"
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

func (scanMediaMessageHandler) Handle(ctx context.Context, bot *sSysBot, event *botMessageEvent) (handled bool, resultErr error) {
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
			g.Log().Warningf(ctx, "扫图确认失败 botId:%d messageId:%d err:%+v", event.BotId, event.Msg.ID, ackErr)
			return true, ackErr
		}
		ctx = scanRequestContext(ctx, traceId, event)
		finishRequest := scantrace.Begin(ctx, "request.single")
		defer func() { finishRequest(resultErr) }()
		claimed, claimErr := bot.claimScanMediaGroupResolved(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.From.ID), fmt.Sprintf("message:%d", event.Msg.ID))
		if claimErr != nil {
			return true, claimErr
		}
		if !claimed {
			return true, nil
		}
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
		g.Log().Warningf(ctx, "创建扫图批次失败 botId:%d messageId:%d err:%+v", event.BotId, event.Msg.ID, err)
		return true, err
	}
	traceId, err := bot.acknowledgeScanMediaGroup(ctx, event.BotId, userId, batchId, event.Msg)
	if err != nil {
		g.Log().Warningf(ctx, "扫图批次确认失败 botId:%d messageId:%d batchId:%s err:%+v", event.BotId, event.Msg.ID, batchId, err)
		return true, err
	}
	ctx = scanRequestContext(ctx, traceId, event)
	ctx = scantrace.With(ctx, g.Map{"batchId": batchId})
	finishRequest := scantrace.Begin(ctx, "request.album_item")
	defer func() { finishRequest(resultErr) }()
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
		if completeErr := bot.completeScanMediaGroupItem(ctx, event.BotId, userId, batchId, nil); completeErr != nil {
			g.Log().Warningf(ctx, "完成扫图批次项失败 traceId:%s batchId:%s err:%+v", traceId, batchId, completeErr)
		}
		observeScanRequest(ctx, event.BotId, "failed")
		return true, bot.replyBotError(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), "扫图搜索", err)
	}
	if len(items) == 0 {
		if err = bot.completeScanMediaGroupItem(ctx, event.BotId, userId, batchId, nil); err != nil {
			return true, err
		}
		return true, bot.sendMessageOnly(ctx, event.BotId, fmt.Sprintf("%d", event.Msg.Chat.ID), "当前媒体没有可用的图片或视频预览图。")
	}
	return true, bot.completeScanMediaGroupItem(ctx, event.BotId, userId, batchId, items)
}

func scanRequestContext(ctx context.Context, traceId string, event *botMessageEvent) context.Context {
	return scantrace.With(withScanTrace(ctx, traceId), g.Map{
		"requestId": gctx.CtxId(ctx),
		"botId":     event.BotId, "userId": event.Msg.From.ID,
		"chatId": event.Msg.Chat.ID, "messageId": event.Msg.ID,
		"groupId": event.Msg.MediaGroupID,
	})
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

func botTokenForEvent(ctx context.Context, bot *sSysBot, botId int64) string {
	row, err := bot.botById(ctx, botId)
	if err != nil || row == nil {
		return ""
	}
	return row.BotToken
}
