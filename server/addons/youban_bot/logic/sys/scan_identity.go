package sys

import (
	"context"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
)

func scanMessageIdentity(botId int64, msg *models.Message) *publishsysin.BotMediaSearchItem {
	item := &publishsysin.BotMediaSearchItem{BotId: botId, MediaType: "image", FileUniqueId: telegramMessageFileUniqueId(msg)}
	if msg == nil {
		return item
	}
	chatId, messageId := forwardedChannelMessageRef(msg)
	item.SourceChatId = chatId
	item.SourceMessageId = int64(messageId)
	if msg.Video != nil {
		item.MediaType = "video"
		item.FileId = msg.Video.FileID
		return item
	}
	for _, photo := range msg.Photo {
		if id := strings.TrimSpace(photo.FileUniqueID); id != "" {
			item.FileUniqueIds = append(item.FileUniqueIds, id)
		}
		if photo.FileUniqueID == item.FileUniqueId {
			item.FileId = photo.FileID
		}
	}
	return item
}

func (s *sSysBot) resolveScanSearchItems(ctx context.Context, botId int64, account *botProfileAccount, msg *models.Message) ([]*publishsysin.BotMediaSearchItem, error) {
	item := scanMessageIdentity(botId, msg)
	// A cache hit needs neither DB preflight nor Telegram getFile.
	if publishService.SysPublish().BotMediaFingerprintCached(ctx, item.FileUniqueId) {
		return []*publishsysin.BotMediaSearchItem{item}, nil
	}
	started := time.Now()
	known, err := publishService.SysPublish().BotMediaSearchKnownItems(ctx, &publishsysin.BotMediaSearchInp{
		TenantId: account.TenantId, AccountId: account.AccountId, AccountType: account.AccountType, Items: []*publishsysin.BotMediaSearchItem{item},
	})
	g.Log().Info(ctx, "扫图下载前标识查询完成", g.Map{"botId": botId, "durationMs": time.Since(started).Milliseconds(), "known": err == nil && len(known) == 1 && known[0], "aliases": len(item.FileUniqueIds), "hasForward": item.SourceMessageId > 0})
	if err == nil && len(known) == 1 && known[0] {
		return []*publishsysin.BotMediaSearchItem{item}, nil
	}
	if err != nil {
		// The optimisation must not disable the existing fingerprint path.
		g.Log().Warning(ctx, "扫图媒体标识预查询失败，继续原搜索流程", g.Map{"botId": botId, "err": err})
	}
	media, err := s.resolveTelegramScanMedia(ctx, botId, botTokenForEvent(ctx, s, botId), msg)
	if err != nil {
		return nil, err
	}
	items := scanSearchItems(media)
	for _, resolved := range items {
		resolved.FileUniqueIds = item.FileUniqueIds
		resolved.BotId, resolved.FileId = item.BotId, item.FileId
		resolved.SourceChatId, resolved.SourceMessageId = item.SourceChatId, item.SourceMessageId
	}
	return items, nil
}
