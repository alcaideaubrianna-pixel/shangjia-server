package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type publishCollectorDeliveryHandler struct {
	publish *sSysPublish
}

type publishCollectorAccountTaskHandler struct{ publish *sSysPublish }

type publishCollectorAccountMediaProvider struct{ publish *sSysPublish }

func (h *publishCollectorAccountTaskHandler) HandleAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) (*collectorin.AccountMediaDownloadResult, error) {
	if h == nil || h.publish == nil || task == nil {
		return nil, gerror.New("Telegram账号任务处理参数无效")
	}
	switch task.TaskType {
	case collectorin.AccountTaskTypeHistoryPage:
		return nil, h.publish.handleCollectHistoryAccountTask(ctx, client, task.HistoryTaskID)
	case collectorin.AccountTaskTypeMaterialImportHistoryPage:
		const prefix = "material-import:"
		parts := strings.Split(strings.TrimPrefix(task.TaskKey, prefix), ":")
		taskID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || taskID <= 0 || !strings.HasPrefix(task.TaskKey, prefix) {
			return nil, gerror.New("资料导入账号任务参数无效")
		}
		importTask, err := h.publish.materialImportTaskByPrimary(ctx, taskID)
		if err != nil {
			return nil, err
		}
		cache, err := h.publish.tgChannelCacheByChannelId(ctx, importTask.TenantId, importTask.TgAccountId, importTask.SourceChatId)
		if err != nil {
			return nil, err
		}
		peer, err := collectInputPeerChannel(cache)
		if err != nil {
			return nil, err
		}
		if _, err = client.Self(ctx); err != nil {
			return nil, err
		}
		return nil, h.publish.pullMaterialImportPages(ctx, client, importTask, peer, cache)
	case collectorin.AccountTaskTypeDialogCacheRefresh:
		return nil, h.publish.handleDialogCacheRefreshAccountTask(ctx, client, task)
	default:
		return nil, gerror.Newf("不支持的Telegram账号任务类型：%s", task.TaskType)
	}
}

func (p *publishCollectorAccountMediaProvider) ResolvePeer(ctx context.Context, tenantID, accountID int64, chatID string, client *telegram.Client) (tg.InputPeerClass, error) {
	if p == nil || p.publish == nil {
		return nil, gerror.New("Telegram账号媒体 Peer 解析器未初始化")
	}
	return p.publish.collectMediaInputPeer(ctx, tenantID, accountID, client, chatID)
}

func (p *publishCollectorAccountMediaProvider) StoreMedia(ctx context.Context, task *collectorin.AccountTask, localPath string) (*collectorin.AccountMediaDownloadResult, error) {
	if p == nil || p.publish == nil || task == nil {
		return nil, gerror.New("Telegram账号媒体存储参数无效")
	}
	ctx = collectMediaRuntimeContext(ctx, task.MediaOwnerAccountID)
	md5Value, mediaSize, mimeType, err := calculateTelegramMediaFingerprint(localPath, task.Media.Type, task.Media.SourceMimeType)
	if err != nil {
		return nil, err
	}
	if cached, ok, cacheErr := lookupTelegramCollectorMediaCache(ctx, task.Media.Type, md5Value, mediaSize, mimeType); cacheErr != nil {
		return nil, cacheErr
	} else if ok {
		task.Media.FileURL = telegramCollectorMediaCacheURL(cached)
		task.Media.StoragePath = cached.StoragePath
		task.Media.FileMD5 = md5Value
		task.Media.SourceSize = mediaSize
		task.Media.FilePHash = cached.PHash
		task.Media.PosterURL = firstNonEmpty(cached.PosterURL, cached.PosterStoragePath)
		return &collectorin.AccountMediaDownloadResult{FileURL: task.Media.FileURL, StoragePath: task.Media.StoragePath, Media: task.Media}, nil
	}
	attachment, err := p.publish.uploadCollectMediaToStorage(ctx, task.Media.Type, localPath)
	if err != nil {
		return nil, err
	}
	item := task.Media
	item.FileURL = strings.TrimSpace(attachment.FileUrl)
	item.StoragePath = normalizeStoredMediaPath(attachment.Path)
	item.FileMD5 = md5Value
	item.SourceSize = mediaSize
	item.SourceMimeType = mimeType
	return &collectorin.AccountMediaDownloadResult{
		AttachmentID: attachment.Id, FileURL: item.FileURL, StoragePath: item.StoragePath, Media: item,
	}, nil
}

func collectorMediaItemFromCollect(item collectMediaItem) collectorin.CollectorMediaItem {
	return collectorin.CollectorMediaItem{
		Type: item.Type, Purpose: item.Purpose, FileID: item.FileId, FileURL: item.FileUrl,
		StoragePath: item.StoragePath, PosterURL: item.PosterUrl, FileMD5: item.FileMd5,
		FilePHash: item.FilePhash, SourceKind: item.SourceKind, SourceMediaID: item.SourceMediaId,
		SourceAccessHash: item.SourceAccessHash, SourceFileReference: append([]byte(nil), item.SourceFileReference...),
		SourceThumbSize: item.SourceThumbSize, SourceMimeType: item.SourceMimeType,
		SourceDCID: item.SourceDCId, SourceSize: item.SourceSize, DebugMetaJSON: item.DebugMetaJson,
	}
}

func collectMediaItemFromCollector(item collectorin.CollectorMediaItem) collectMediaItem {
	return collectMediaItem{
		Type: item.Type, Purpose: item.Purpose, FileId: item.FileID, FileUrl: item.FileURL,
		StoragePath: item.StoragePath, PosterUrl: item.PosterURL, FileMd5: item.FileMD5,
		FilePhash: item.FilePHash, SourceKind: item.SourceKind, SourceMediaId: item.SourceMediaID,
		SourceAccessHash: item.SourceAccessHash, SourceFileReference: append([]byte(nil), item.SourceFileReference...),
		SourceThumbSize: item.SourceThumbSize, SourceMimeType: item.SourceMimeType,
		SourceDCId: item.SourceDCID, SourceSize: item.SourceSize, DebugMetaJson: item.DebugMetaJSON,
	}
}

func (h *publishCollectorDeliveryHandler) HandleCollectorDelivery(ctx context.Context, delivery *collectorin.CollectorDelivery) error {
	if h == nil || h.publish == nil || delivery == nil {
		return gerror.New("Telegram采集交付处理参数无效")
	}
	switch delivery.SourceType {
	case collectorin.SourceTypeBot:
		return h.publish.ingestCollectorBotDelivery(ctx, delivery)
	case collectorin.SourceTypeAccount:
		return h.publish.ingestCollectorAccountDelivery(ctx, delivery)
	default:
		return gerror.Newf("暂不支持的Telegram采集来源类型：%s", delivery.SourceType)
	}
}

func (s *sSysPublish) ingestCollectorBotDelivery(ctx context.Context, delivery *collectorin.CollectorDelivery) error {
	if !s.collectGlobalEnabled(ctx) {
		return nil
	}
	source, err := g.DB().Model(publishCollectSourceTable+" s").Safe().Ctx(ctx).
		InnerJoin("hg_youban_publish_tenant_vip vip", "vip.tenant_id=s.tenant_id AND vip.status=1 AND vip.level>0 AND vip.deleted_at IS NULL").
		Fields("s.id,s.tenant_id,s.account_id").
		Where("s.id", delivery.SourceID).
		Where("s.source_type", sysin.CollectSourceTypeBot).
		Where("s.collect_enabled", 1).
		Where("s.status", 1).
		Where("(vip.expired_at IS NULL OR vip.expired_at>?)", gtime.Now()).
		WhereNull("s.deleted_at").
		One()
	if err != nil {
		return gerror.Wrap(err, "读取Bot采集源失败")
	}
	if source.IsEmpty() {
		return nil
	}
	blocked, err := s.botCollectMessageFromPublishChannel(ctx, source["tenant_id"].Int64(), g.NewVar(delivery.SourceChatID).Int64())
	if err != nil {
		return gerror.Wrap(err, "检查Bot采集上架频道过滤失败")
	}
	if blocked {
		return nil
	}
	message := collectorDeliveryMessage(delivery, source["tenant_id"].Int64(), source["account_id"].Int64(), source["id"].Int64(), sysin.CollectSourceTypeBot)
	_, err = s.ingestAndProcessCollectMessage(ctx, message)
	return err
}

func (s *sSysPublish) ingestCollectorAccountDelivery(ctx context.Context, delivery *collectorin.CollectorDelivery) error {
	message := collectorDeliveryMessage(delivery, delivery.TenantID, delivery.AccountID, delivery.SourceID, sysin.CollectSourceTypeAccount)
	message.TgAccountId = delivery.TgAccountID
	if groupedID := strings.TrimSpace(delivery.SourceGroupedID); groupedID != "" {
		message.SourceUniqueKey = accountCollectMaterialGroupKey(delivery, groupedID)
	}
	_, err := s.ingestAndProcessCollectMessage(ctx, message)
	return err
}

func collectorDeliveryMessage(delivery *collectorin.CollectorDelivery, tenantID, accountID, sourceID int64, sourceType string) *CollectMessage {
	message := &CollectMessage{
		TenantId: tenantID, AccountId: accountID, SourceId: sourceID, SourceType: sourceType,
		SourceChatId: delivery.SourceChatID, SourceMessageId: delivery.SourceMessageID,
		SourceGroupedId: delivery.SourceGroupedID, SourceUniqueKey: delivery.SourceUniqueKey, RawText: delivery.RawText,
	}
	if !delivery.ReceivedAt.IsZero() {
		message.ReceivedAt = gtime.New(delivery.ReceivedAt)
	}
	message.Media = make([]collectMediaItem, 0, len(delivery.Media))
	for _, item := range delivery.Media {
		message.Media = append(message.Media, collectMediaItem{
			Type: item.Type, Purpose: item.Purpose, FileId: item.FileID, FileUrl: item.FileURL,
			StoragePath: item.StoragePath, PosterUrl: item.PosterURL, FileMd5: item.FileMD5,
			FilePhash: item.FilePHash, SourceKind: item.SourceKind, SourceMediaId: item.SourceMediaID,
			SourceAccessHash: item.SourceAccessHash, SourceFileReference: append([]byte(nil), item.SourceFileReference...),
			SourceThumbSize: item.SourceThumbSize, SourceMimeType: item.SourceMimeType,
			SourceDCId: item.SourceDCID, SourceSize: item.SourceSize, DebugMetaJson: item.DebugMetaJSON,
		})
	}
	return message
}

func accountCollectMaterialGroupKey(delivery *collectorin.CollectorDelivery, groupedID string) string {
	if delivery == nil {
		return ""
	}
	groupedID = strings.TrimSpace(groupedID)
	if groupedID == "" {
		return strings.TrimSpace(delivery.SourceUniqueKey)
	}
	return fmt.Sprintf(
		"account:%d:source:%d:%s:group:%s",
		delivery.TgAccountID,
		delivery.SourceID,
		strings.TrimSpace(delivery.SourceChatID),
		groupedID,
	)
}
