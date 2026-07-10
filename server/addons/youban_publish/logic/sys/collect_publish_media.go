package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) rebuildCollectPublishMedia(ctx context.Context, event gdb.Record, content *collectContentResult, taskId int64) error {
	if err := ensureMediaEditColumns(ctx); err != nil {
		return err
	}
	items := make([]collectMediaItem, 0)
	if content != nil && strings.TrimSpace(content.MediaJSON) != "" {
		_ = json.Unmarshal([]byte(content.MediaJSON), &items)
	}
	if len(items) == 0 {
		rows, err := s.collectEventMediaRows(ctx, event["id"].Int64())
		if err != nil {
			return err
		}
		items = collectMediaRowsToItems(rows)
	}
	now := gtime.Now()
	if _, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		WhereNull("deleted_at").
		Data(g.Map{
			"deleted_at": now,
			"deleted_by": event["account_id"].Int64(),
			"updated_at": now,
		}).
		Update(); err != nil {
		return gerror.Wrap(err, "清理采集旧媒体失败")
	}
	return s.insertCollectPublishMediaRows(ctx, event, taskId, "display", items)
}

func (s *sSysPublish) insertCollectPublishMediaRows(ctx context.Context, event gdb.Record, taskId int64, purpose string, items []collectMediaItem) error {
	sortIndex := 1
	for _, item := range items {
		if collectMediaSourceKey(item) == "" {
			continue
		}
		if err := s.insertCollectPublishMediaRow(ctx, event, taskId, purpose, sortIndex, item); err != nil {
			return err
		}
		sortIndex++
	}
	return nil
}

func (s *sSysPublish) insertCollectPublishMediaRow(ctx context.Context, event gdb.Record, taskId int64, purpose string, sortIndex int, item collectMediaItem) error {
	item, err := s.prepareCollectMediaAsset(ctx, event, item)
	if err != nil {
		return err
	}
	mediaType := collectPublishMediaType(item.Type)
	if mediaType == "" {
		return nil
	}
	storagePath := strings.TrimSpace(item.StoragePath)
	fileUrl := strings.TrimSpace(item.FileUrl)
	cacheStatus := tgCacheStatusInvalid
	if strings.TrimSpace(item.FileId) != "" {
		cacheStatus = tgCacheStatusValid
	}
	now := gtime.Now()
	_, err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id":           event["tenant_id"].Int64(),
		"merchant_id":         event["tenant_id"].Int64(),
		"account_id":          event["account_id"].Int64(),
		"task_id":             taskId,
		"media_type":          mediaType,
		"purpose":             purpose,
		"name":                fmt.Sprintf("collect-%d-%s-%d", event["id"].Int64(), purpose, sortIndex),
		"tg_file_id":          strings.TrimSpace(item.FileId),
		"file_url":            fileUrl,
		"storage_path":        storagePath,
		"tg_cache_asset_hash": mediaAssetHash(storagePath, fileUrl),
		"tg_cache_status":     cacheStatus,
		"poster_url":          strings.TrimSpace(item.PosterUrl),
		"sort_index":          sortIndex,
		"status":              1,
		"created_at":          now,
		"updated_at":          now,
		"created_by":          event["account_id"].Int64(),
		"updated_by":          event["account_id"].Int64(),
	}).Insert()
	return gerror.Wrap(err, "创建采集媒体失败")
}
