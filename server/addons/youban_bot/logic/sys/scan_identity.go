package sys

import (
	"context"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"golang.org/x/sync/errgroup"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
	"hotgo/internal/library/telegrammedia"
	"hotgo/utility/scantrace"
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
		if msg.Video.Thumbnail != nil {
			item.PreviewFileId = msg.Video.Thumbnail.FileID
			item.PreviewSize = int64(msg.Video.Thumbnail.FileSize)
		}
		return item
	}
	photo := scanPhotoSize(msg.Photo)
	item.PreviewFileId, item.PreviewSize = photo.FileID, int64(photo.FileSize)
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
	// Album intake only captures metadata. Resolve the whole batch once after
	// the quiet window; do not query permissions/getFile once per webhook.
	return []*publishsysin.BotMediaSearchItem{item}, nil
}

func (s *sSysBot) resolveScanBatch(ctx context.Context, botId int64, account *botProfileAccount, items []*publishsysin.BotMediaSearchItem) ([]*publishsysin.BotMediaSearchItem, error) {
	started := time.Now()
	known := make([]bool, len(items))
	unchecked := make([]*publishsysin.BotMediaSearchItem, 0, len(items))
	indices := make([]int, 0, len(items))
	for i, item := range items {
		if item == nil {
			continue
		}
		if item.FileUrl != "" || publishService.SysPublish().BotMediaFingerprintCached(ctx, item.FileUniqueId) {
			known[i] = true
		} else {
			unchecked = append(unchecked, item)
			indices = append(indices, i)
		}
	}
	var err error
	if len(unchecked) > 0 {
		var hits []bool
		finish := scantrace.Begin(ctx, "telegram.identity_batch")
		hits, err = publishService.SysPublish().BotMediaSearchKnownItems(ctx, &publishsysin.BotMediaSearchInp{
			TenantId: account.TenantId, AccountId: account.AccountId, AccountType: account.AccountType, Items: unchecked,
		})
		finish(err)
		if err == nil && len(hits) == len(unchecked) {
			for i, hit := range hits {
				known[indices[i]] = hit
			}
		}
	}
	knownCount := 0
	for _, hit := range known {
		if hit {
			knownCount++
		}
	}
	scantrace.Record(ctx, "telegram.identity_batch.summary", g.Map{"botId": botId, "durationMs": time.Since(started).Milliseconds(), "inputItems": len(items), "preflightItems": len(unchecked), "knownItems": knownCount}, err)
	if err != nil {
		// The optimisation must not disable the existing fingerprint path.
		g.Log().Warning(ctx, "扫图媒体标识预查询失败，继续原搜索流程", g.Map{"botId": botId, "err": err})
	}
	results := make([]*publishsysin.BotMediaSearchItem, len(items))
	errors := make([]error, len(items))
	group := new(errgroup.Group)
	group.SetLimit(4)
	for i, item := range items {
		i, item := i, item
		group.Go(func() error {
			if item == nil {
				return nil
			}
			copy := *item
			if known[i] {
				results[i] = &copy
				return nil
			}
			if item.PreviewFileId == "" {
				errors[i] = gerror.New("当前媒体没有可用的图片或视频预览图")
				return nil
			}
			itemCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			itemCtx = scantrace.With(itemCtx, g.Map{"itemIndex": i})
			finish := scantrace.Begin(itemCtx, "telegram.get_file")
			url, resolveErr := s.resolveTelegramMediaURL(itemCtx, botId, botTokenForEvent(itemCtx, s, botId), telegrammedia.Request{FileID: item.PreviewFileId, MediaType: "image", Size: item.PreviewSize})
			finish(resolveErr)
			if resolveErr != nil {
				errors[i] = resolveErr
				return nil
			}
			copy.FileUrl = url
			results[i] = &copy
			return nil
		})
	}
	_ = group.Wait()
	resolved := make([]*publishsysin.BotMediaSearchItem, 0, len(items))
	var firstErr error
	for i, item := range results {
		if item != nil {
			resolved = append(resolved, item)
		}
		if errors[i] != nil {
			if firstErr == nil {
				firstErr = errors[i]
			}
			g.Log().Warning(ctx, "扫图预览解析失败", g.Map{"itemIndex": i, "err": errors[i]})
		}
	}
	scantrace.Record(ctx, "telegram.resolve_batch.summary", g.Map{"durationMs": time.Since(started).Milliseconds(), "inputItems": len(items), "resolvedItems": len(resolved), "failedItems": len(items) - len(resolved), "incomplete": firstErr != nil}, firstErr)
	return resolved, firstErr
}
