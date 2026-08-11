package sys

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type publishCollectorDeliveryHandler struct {
	publish *sSysPublish
}

type publishCollectorAccountTaskHandler struct{ publish *sSysPublish }

func (h *publishCollectorAccountTaskHandler) HandleAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) (json.RawMessage, error) {
	if h == nil || h.publish == nil || task == nil {
		return nil, gerror.New("Telegram账号任务处理参数无效")
	}
	switch task.TaskType {
	case collectorin.AccountTaskTypeHistoryPage:
		var payload collectHistoryAccountTaskPayload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			return nil, gerror.Wrap(err, "解析历史采集账号任务失败")
		}
		return nil, h.publish.handleCollectHistoryAccountTask(ctx, client, payload.TaskID)
	case collectorin.AccountTaskTypeMediaDownload:
		return h.publish.handleAccountMediaDownloadTask(ctx, client, task)
	default:
		return nil, gerror.Newf("不支持的Telegram账号任务类型：%s", task.TaskType)
	}
}

func (s *sSysPublish) handleAccountMediaDownloadTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) (json.RawMessage, error) {
	var payload collectorin.AccountMediaDownloadPayload
	if err := json.Unmarshal(task.Payload, &payload); err != nil {
		return nil, gerror.Wrap(err, "解析账号媒体下载任务失败")
	}
	item := collectMediaItemFromCollector(payload.Media)
	ctx = collectMediaRuntimeContext(ctx, payload.AccountID)
	downloaded, err := s.downloadTelegramMediaWithRefresh(ctx, payload.TenantID, task.AccountID, item, client)
	if err != nil {
		if collectMediaSourceGone(err) {
			return json.Marshal(&collectorin.AccountMediaDownloadResult{
				Media: collectorMediaItemFromCollect(item), ErrorCode: "source_gone", ErrorMessage: err.Error(),
			})
		}
		return nil, err
	}
	if downloaded == nil || strings.TrimSpace(downloaded.Path) == "" {
		return nil, gerror.New("账号媒体下载完成但未返回本地缓存文件")
	}
	attachment, err := s.uploadCollectMediaToStorage(ctx, item.Type, downloaded.Path)
	if err != nil {
		return nil, err
	}
	resultItem := downloaded.Item
	if strings.TrimSpace(resultItem.FileId) == "" {
		resultItem = item
	}
	resultItem.FileUrl = strings.TrimSpace(attachment.FileUrl)
	resultItem.StoragePath = normalizeStoredMediaPath(attachment.Path)
	resultItem.DebugMetaJson = firstNonEmpty(downloaded.MetaJson, resultItem.DebugMetaJson)
	return json.Marshal(&collectorin.AccountMediaDownloadResult{
		AttachmentID: attachment.Id,
		FileURL:      resultItem.FileUrl,
		StoragePath:  resultItem.StoragePath,
		Media:        collectorMediaItemFromCollect(resultItem),
	})
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
		var update models.Update
		if err := json.Unmarshal(delivery.RawUpdate, &update); err != nil {
			return gerror.Wrap(err, "解析Telegram采集交付原始消息失败")
		}
		message, _ := telegramUpdateMessage(&update)
		if message == nil {
			return nil
		}
		return h.publish.collectBotMessage(ctx, delivery.SourceID, message)
	case collectorin.SourceTypeAccount:
		return h.publish.ingestCollectorAccountDelivery(ctx, delivery)
	default:
		return gerror.Newf("暂不支持的Telegram采集来源类型：%s", delivery.SourceType)
	}
}

func (s *sSysPublish) ingestCollectorAccountDelivery(ctx context.Context, delivery *collectorin.CollectorDelivery) error {
	message := &CollectMessage{
		TenantId: delivery.TenantID, AccountId: delivery.AccountID, SourceId: delivery.SourceID,
		SourceType: sysin.CollectSourceTypeAccount, TgAccountId: delivery.TgAccountID,
		SourceChatId: delivery.SourceChatID, SourceMessageId: delivery.SourceMessageID,
		SourceGroupedId: delivery.SourceGroupedID, SourceUniqueKey: delivery.SourceUniqueKey,
		RawText: delivery.RawText,
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
	_, err := s.ingestAndProcessCollectMessage(ctx, message)
	return err
}
