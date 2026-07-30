package sys

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/entity"
)

const (
	collectMediaCachePending     = "pending"
	collectMediaCacheForwarding  = "forwarding"
	collectMediaCacheDownloading = "downloading"
	collectMediaCacheReady       = "ready"
	collectMediaCacheFailed      = "failed"
	collectMediaNextRetryAt      = "next_retry_at"
)

func (s *sSysPublish) appendCollectEventLog(ctx context.Context, eventId int64, stage string, status string, message string, meta string) {
	if eventId <= 0 {
		return
	}
	if skipCollectEventLog(stage, status, message) {
		return
	}
	cols := pdao.YoubanPublishCollectEventLog.Columns()
	eventCols := pdao.YoubanPublishCollectEvent.Columns()
	tenantId := int64(0)
	accountId := int64(0)
	event, _ := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields(eventCols.TenantId, eventCols.AccountId).
		Where(eventCols.Id, eventId).
		One()
	if !event.IsEmpty() {
		tenantId = event[eventCols.TenantId].Int64()
		accountId = event[eventCols.AccountId].Int64()
	}
	_, _ = pdao.YoubanPublishCollectEventLog.Ctx(ctx).Data(g.Map{
		cols.TenantId:  tenantId,
		cols.AccountId: accountId,
		cols.EventId:   eventId,
		cols.Stage:     strings.TrimSpace(stage),
		cols.Status:    strings.TrimSpace(status),
		cols.Message:   strings.TrimSpace(message),
		cols.MetaText:  strings.TrimSpace(meta),
		cols.CreatedAt: gtime.Now(),
	}).Insert()
}

func (s *sSysPublish) appendCollectEventLogForRecord(ctx context.Context, event gdb.Record, stage string, status string, message string, meta string) {
	if event.IsEmpty() || event["id"].Int64() <= 0 {
		return
	}
	if skipCollectEventLog(stage, status, message) {
		return
	}
	s.appendCollectEventLogWithOwner(ctx, event["id"].Int64(), event["tenant_id"].Int64(), event["account_id"].Int64(), stage, status, message, meta)
}

func (s *sSysPublish) appendCollectEventLogWithOwner(ctx context.Context, eventId int64, tenantId int64, accountId int64, stage string, status string, message string, meta string) {
	if eventId <= 0 {
		return
	}
	if skipCollectEventLog(stage, status, message) {
		return
	}
	cols := pdao.YoubanPublishCollectEventLog.Columns()
	_, _ = pdao.YoubanPublishCollectEventLog.Ctx(ctx).Data(g.Map{
		cols.TenantId:  tenantId,
		cols.AccountId: accountId,
		cols.EventId:   eventId,
		cols.Stage:     strings.TrimSpace(stage),
		cols.Status:    strings.TrimSpace(status),
		cols.Message:   strings.TrimSpace(message),
		cols.MetaText:  strings.TrimSpace(meta),
		cols.CreatedAt: gtime.Now(),
	}).Insert()
}

func skipCollectEventLog(stage string, status string, message string) bool {
	stage = strings.TrimSpace(stage)
	status = strings.TrimSpace(status)
	message = strings.TrimSpace(message)
	if stage != "media" {
		return false
	}
	switch status + ":" + message {
	case
		"pending:媒体等待缓存",
		"running:媒体缓存任务开始执行",
		"checking:开始检查媒体缓存方式",
		"downloading:账号采集媒体使用下载缓存，保证带文案媒体组可原格式发送",
		"ready:媒体缓存任务处理完成",
		"ready:媒体已就绪",
		"forwarded:媒体已转存到备份频道",
		"forwarded:账号采集运行时已完成媒体转存",
		"forwarded:下载媒体组已推送到备份频道":
		return true
	default:
		return false
	}
}

func (s *sSysPublish) upsertCollectEventMedia(ctx context.Context, event gdb.Record, media []collectMediaItem) error {
	if event.IsEmpty() || event["id"].Int64() <= 0 {
		return nil
	}
	eventId := event["id"].Int64()
	sortIndex := 1
	for _, item := range media {
		item = normalizeCollectMediaItem(item)
		sourceKey := collectMediaSourceKey(item)
		if sourceKey == "" || collectPublishMediaType(item.Type) == "" {
			continue
		}
		mediaKey := strings.TrimSpace(item.Type) + ":" + sourceKey
		status := collectMediaCacheReady
		if strings.HasPrefix(strings.TrimSpace(item.FileId), "gotd:") && item.StoragePath == "" && item.FileUrl == "" {
			status = collectMediaCachePending
		}
		mediaCols := pdao.YoubanPublishCollectEventMedia.Columns()
		existing, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
			Fields(mediaCols.Id, mediaCols.SortIndex, mediaCols.CacheStatus).
			Where(mediaCols.EventId, eventId).
			Where(mediaCols.SourceMediaKey, mediaKey).
			One()
		if err != nil {
			return gerror.Wrap(err, "读取采集事件媒体失败")
		}
		data := g.Map{
			mediaCols.TenantId:         event["tenant_id"].Int64(),
			mediaCols.AccountId:        event["account_id"].Int64(),
			mediaCols.SourceId:         event["source_id"].Int64(),
			mediaCols.SourceType:       event["source_type"].String(),
			mediaCols.SourceChatId:     event["source_chat_id"].String(),
			mediaCols.SourceMessageId:  collectMediaSourceMessageId(item, event["source_message_id"].Int64()),
			mediaCols.SourceGroupedId:  event["source_grouped_id"].String(),
			mediaCols.SourceMediaKey:   mediaKey,
			mediaCols.MediaType:        strings.TrimSpace(item.Type),
			mediaCols.SourceRefType:    collectMediaRefType(item),
			mediaCols.SourceFileId:     strings.TrimSpace(item.FileId),
			mediaCols.SourceMessageRef: collectMediaMessageRef(item),
			mediaCols.FileUrl:          strings.TrimSpace(item.FileUrl),
			mediaCols.StoragePath:      normalizeStoredMediaPath(item.StoragePath),
			mediaCols.PosterUrl:        strings.TrimSpace(item.PosterUrl),
			mediaCols.MetaJson:         strings.TrimSpace(item.MetaJson),
			mediaCols.UpdatedAt:        gtime.Now(),
		}
		if !existing.IsEmpty() && existing[mediaCols.CacheStatus].String() == collectMediaCacheReady {
			if strings.TrimSpace(item.FileUrl) == "" {
				delete(data, mediaCols.FileUrl)
			}
			if strings.TrimSpace(item.StoragePath) == "" {
				delete(data, mediaCols.StoragePath)
			}
			if strings.TrimSpace(item.PosterUrl) == "" {
				delete(data, mediaCols.PosterUrl)
			}
		}
		if existing.IsEmpty() {
			data[mediaCols.EventId] = eventId
			data[mediaCols.SortIndex] = collectMediaSortIndex(item, sortIndex)
			data[mediaCols.CacheStatus] = status
			data[mediaCols.CreatedAt] = gtime.Now()
			if _, err = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Data(data).Insert(); err != nil {
				return gerror.Wrap(err, "创建采集事件媒体失败")
			}
		} else {
			data[mediaCols.SortIndex] = collectMediaSortIndex(item, existing[mediaCols.SortIndex].Int())
			if existing[mediaCols.CacheStatus].String() == collectMediaCacheReady {
				delete(data, mediaCols.CacheStatus)
			} else {
				data[mediaCols.CacheStatus] = status
			}
			if _, err = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where(mediaCols.Id, existing[mediaCols.Id].Int64()).Data(data).Update(); err != nil {
				return gerror.Wrap(err, "更新采集事件媒体失败")
			}
		}
		sortIndex++
	}
	return s.syncCollectEventMediaSnapshot(ctx, eventId)
}

func (s *sSysPublish) collectEventMediaRows(ctx context.Context, eventId int64) ([]*entity.YoubanPublishCollectEventMedia, error) {
	rows := make([]*entity.YoubanPublishCollectEventMedia, 0)
	if eventId <= 0 {
		return rows, nil
	}
	mediaCols := pdao.YoubanPublishCollectEventMedia.Columns()
	err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Where(mediaCols.EventId, eventId).
		OrderAsc(mediaCols.SortIndex).
		OrderAsc(mediaCols.Id).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集事件媒体失败")
	}
	sortCollectEventMediaRows(rows)
	return rows, nil
}

func (s *sSysPublish) syncCollectEventMediaSnapshot(ctx context.Context, eventId int64) error {
	rows, err := s.collectEventMediaRows(ctx, eventId)
	if err != nil {
		return err
	}
	event, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", eventId).One()
	if err != nil || event.IsEmpty() {
		return err
	}
	items := collectMediaRowsToItems(rows, event["material_role"].String())
	mediaJSON, mediaCount := collectMessageMediaJSON(items)
	eventCols := pdao.YoubanPublishCollectEvent.Columns()
	_, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).Where(eventCols.Id, eventId).Data(g.Map{
		eventCols.MediaCount: mediaCount,
		eventCols.MediaJson:  mediaJSON,
		eventCols.DedupeKey:  collectHash(fmt.Sprintf("%s:%s", normalizeCollectText(event[eventCols.RawText].String()), collectMediaSignature(mediaJSON))),
		eventCols.UpdatedAt:  gtime.Now(),
	}).Update()
	return gerror.Wrap(err, "同步采集事件媒体快照失败")
}

func collectMediaRowsToItems(rows []*entity.YoubanPublishCollectEventMedia, purpose string) []collectMediaItem {
	items := make([]collectMediaItem, 0, len(rows))
	purpose = strings.TrimSpace(purpose)
	for _, row := range rows {
		if row == nil {
			continue
		}
		fileId := strings.TrimSpace(row.SourceFileId)
		if row.BackupChatId != "" && row.BackupMessageId > 0 {
			fileId = telegramCopyMediaFileId(row.BackupChatId, int(row.BackupMessageId))
		}
		items = append(items, collectMediaItem{
			Type:        row.MediaType,
			Purpose:     purpose,
			FileId:      fileId,
			FileUrl:     row.FileUrl,
			StoragePath: row.StoragePath,
			PosterUrl:   row.PosterUrl,
			MetaJson:    row.MetaJson,
		})
	}
	return items
}

func normalizeCollectMediaItem(item collectMediaItem) collectMediaItem {
	item.Type = strings.TrimSpace(item.Type)
	item.Purpose = strings.TrimSpace(item.Purpose)
	item.FileId = strings.TrimSpace(item.FileId)
	item.FileUrl = strings.TrimSpace(item.FileUrl)
	item.StoragePath = strings.TrimSpace(item.StoragePath)
	item.PosterUrl = strings.TrimSpace(item.PosterUrl)
	item.FileMd5 = strings.TrimSpace(item.FileMd5)
	item.FilePhash = strings.TrimSpace(item.FilePhash)
	item.MetaJson = strings.TrimSpace(item.MetaJson)
	return item
}

func collectMediaRefType(item collectMediaItem) string {
	fileId := strings.TrimSpace(item.FileId)
	switch {
	case strings.HasPrefix(fileId, "gotd:"):
		return "gotd"
	case strings.HasPrefix(fileId, "copy:"):
		return "copy"
	case fileId != "":
		return "bot_file"
	case strings.TrimSpace(item.StoragePath) != "":
		return "download"
	case strings.TrimSpace(item.FileUrl) != "":
		return "url"
	default:
		return ""
	}
}

func collectMediaMessageRef(item collectMediaItem) string {
	if strings.HasPrefix(strings.TrimSpace(item.FileId), "gotd:") {
		return strings.TrimSpace(item.FileId)
	}
	return ""
}

func collectMediaSourceMessageId(item collectMediaItem, fallback int64) int64 {
	_, messageId, ok := parseGotdCollectFileId(item.FileId)
	if ok && messageId > 0 {
		return int64(messageId)
	}
	return fallback
}

func collectMediaSortIndex(item collectMediaItem, fallback int) int {
	if _, messageId, ok := parseGotdCollectFileId(item.FileId); ok {
		return messageId
	}
	if fallback > 0 {
		return fallback
	}
	return 1
}

func sortCollectEventMediaRows(rows []*entity.YoubanPublishCollectEventMedia) {
	sort.SliceStable(rows, func(i, j int) bool {
		left := collectMediaRowOrder(rows[i])
		right := collectMediaRowOrder(rows[j])
		if left != right {
			return left < right
		}
		if rows[i] == nil || rows[j] == nil {
			return rows[j] != nil
		}
		return rows[i].Id < rows[j].Id
	})
}

func collectMediaRowOrder(row *entity.YoubanPublishCollectEventMedia) int {
	if row == nil {
		return 0
	}
	if _, messageId, ok := parseGotdCollectFileId(row.SourceMessageRef); ok {
		return messageId
	}
	if _, messageId, ok := parseGotdCollectFileId(row.SourceFileId); ok {
		return messageId
	}
	if row.SortIndex > 0 {
		return row.SortIndex
	}
	return int(row.Id)
}
