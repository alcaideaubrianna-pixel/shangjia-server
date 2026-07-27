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

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) rebuildCollectPublishMedia(ctx context.Context, event gdb.Record, content *collectContentResult, taskId int64) error {
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
	task, err := s.getTaskByTenant(ctx, taskId, event["tenant_id"].Int64())
	if err != nil {
		return err
	}
	profileId := task["profile_id"].Int64()
	if profileId <= 0 {
		return gerror.New("采集发布资料不存在")
	}
	if _, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).
		WhereNull("deleted_at").
		Data(g.Map{
			"deleted_at": now,
			"deleted_by": event["account_id"].Int64(),
			"updated_at": now,
		}).
		Update(); err != nil {
		return gerror.Wrap(err, "清理采集旧媒体失败")
	}
	_ = s.deleteMediaPHashBucketByProfileId(ctx, profileId)
	displayItems, verifyItems := splitCollectPublishMediaItems(event, items)
	if err := s.insertCollectPublishMediaRows(ctx, event, taskId, "display", displayItems); err != nil {
		return err
	}
	return s.insertCollectPublishMediaRows(ctx, event, taskId, "verify", verifyItems)
}

func splitCollectPublishMediaItems(event gdb.Record, items []collectMediaItem) ([]collectMediaItem, []collectMediaItem) {
	if !event.IsEmpty() && strings.TrimSpace(event["source_grouped_id"].String()) != "" {
		return items, nil
	}
	if len(items) <= 1 {
		return items, nil
	}
	last := items[len(items)-1]
	if strings.TrimSpace(last.Type) != "video" {
		return items, nil
	}
	display := make([]collectMediaItem, len(items)-1)
	copy(display, items[:len(items)-1])
	return display, []collectMediaItem{last}
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
	task, err := s.getTaskByTenant(ctx, taskId, event["tenant_id"].Int64())
	if err != nil {
		return err
	}
	profileId := task["profile_id"].Int64()
	if profileId <= 0 {
		return gerror.New("采集发布资料不存在")
	}
	item, err = s.prepareCollectMediaAsset(ctx, event, item)
	if err != nil {
		return err
	}
	mediaType := collectPublishMediaType(item.Type)
	if mediaType == "" {
		return nil
	}
	storagePath := strings.TrimSpace(item.StoragePath)
	fileUrl := strings.TrimSpace(item.FileUrl)
	assets, err := s.ProcessStoredMediaAssets(ctx, &sysin.StoredMediaAssetsInp{
		MediaType: mediaType,
		LocalPath: storagePath,
	})
	if err != nil {
		return err
	}
	cacheStatus := tgCacheStatusInvalid
	if strings.TrimSpace(item.FileId) != "" {
		cacheStatus = tgCacheStatusValid
	}
	now := gtime.Now()
	posterURL := strings.TrimSpace(item.PosterUrl)
	posterStoragePath := ""
	perceptualHash := ""
	if assets != nil && assets.Processed {
		perceptualHash = assets.PerceptualHash
		posterURL = firstNonEmpty(assets.PosterUrl, posterURL)
		posterStoragePath = assets.PosterStoragePath
	}
	if perceptualHash == "" {
		remoteAssets, remoteErr := s.ProcessRemoteMediaAssets(ctx, &sysin.RemoteMediaAssetsInp{
			MediaType: mediaType,
			FileURL:   fileUrl,
			PosterURL: posterURL,
		})
		if remoteErr != nil {
			g.Log().Warning(ctx, "计算采集媒体哈希失败", g.Map{"eventId": event["id"].Int64(), "mediaType": mediaType, "err": remoteErr})
		} else if remoteAssets != nil && remoteAssets.Processed {
			perceptualHash = remoteAssets.PerceptualHash
		}
	}
	mediaId, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id":           event["tenant_id"].Int64(),
		"merchant_id":         event["tenant_id"].Int64(),
		"account_id":          event["account_id"].Int64(),
		"profile_id":          profileId,
		"media_type":          mediaType,
		"purpose":             purpose,
		"name":                fmt.Sprintf("collect-%d-%s-%d", event["id"].Int64(), purpose, sortIndex),
		"tg_file_id":          strings.TrimSpace(item.FileId),
		"file_url":            fileUrl,
		"storage_path":        storagePath,
		"tg_cache_asset_hash": mediaAssetHash(storagePath, fileUrl),
		"tg_cache_status":     cacheStatus,
		"poster_url":          posterURL,
		"poster_storage_path": posterStoragePath,
		"perceptual_hash":     perceptualHash,
		"sort_index":          sortIndex,
		"status":              1,
		"created_at":          now,
		"updated_at":          now,
		"created_by":          event["account_id"].Int64(),
		"updated_by":          event["account_id"].Int64(),
	}).InsertAndGetId()
	if err != nil {
		return gerror.Wrap(err, "创建采集媒体失败")
	}
	return s.syncMediaPHashBucketByMediaId(ctx, mediaId)
}
