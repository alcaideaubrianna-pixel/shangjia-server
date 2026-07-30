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

	"hotgo/internal/dao"
)

type collectPublishMediaOwner struct {
	ProfileId int64
}

func (s *sSysPublish) rebuildCollectProfileMedia(ctx context.Context, event gdb.Record, content *collectContentResult, profileId int64) error {
	if err := s.rebuildCollectOwnedMedia(ctx, event, content, collectPublishMediaOwner{ProfileId: profileId}); err != nil {
		return err
	}
	columns := dao.ContentProfile.Columns()
	imageCount, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).Where("media_type", "image").WhereNull("deleted_at").Count()
	if err != nil {
		return gerror.Wrap(err, "统计采集图片失败")
	}
	videoCount, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).Where("media_type", "video").WhereNull("deleted_at").Count()
	if err != nil {
		return gerror.Wrap(err, "统计采集视频失败")
	}
	_, err = dao.ContentProfile.Ctx(ctx).Where(columns.Id, profileId).Data(g.Map{
		columns.ImageCount: imageCount, columns.VideoCount: videoCount, columns.UpdatedAt: gtime.Now(),
	}).Update()
	return gerror.Wrap(err, "更新采集媒体数量失败")
}

func (s *sSysPublish) canonicalCollectProfileMedia(ctx context.Context, event gdb.Record, content *collectContentResult) (*collectContentResult, error) {
	if content == nil {
		content = &collectContentResult{}
	}
	resolved := *content
	contentItems := collectMediaRowsToItemsFromJSON(content.MediaJSON)
	displayItems := make([]collectMediaItem, 0, len(contentItems))
	for _, item := range contentItems {
		if strings.EqualFold(strings.TrimSpace(item.Purpose), collectMaterialRoleVerify) {
			continue
		}
		item.Purpose = collectMaterialRoleDisplay
		displayItems = append(displayItems, item)
	}

	if eventID := event["id"].Int64(); eventID > 0 {
		rows, err := s.collectEventMediaRows(ctx, eventID)
		if err != nil {
			return nil, err
		}
		if len(rows) > 0 {
			displayItems = collectMediaRowsToItems(rows, collectMaterialRoleDisplay)
		}
	}

	verifyItems := make([]collectMediaItem, 0)
	verify, err := s.pairedCollectVerifyEvent(ctx, event["id"].Int64())
	if err != nil {
		return nil, err
	}
	if !verify.IsEmpty() {
		rows, rowErr := s.collectEventMediaRows(ctx, verify["id"].Int64())
		if rowErr != nil {
			return nil, rowErr
		}
		if len(rows) > 0 {
			verifyItems = collectMediaRowsToItems(rows, collectMaterialRoleVerify)
		} else {
			verifyItems = collectMediaJSONWithPurposeItems(verify["media_json"].String(), collectMaterialRoleVerify)
		}
	}
	if len(verifyItems) == 0 {
		verifyItems = collectMediaJSONWithPurposeItems(content.MediaJSON, collectMaterialRoleVerify)
	}

	mediaJSON, mediaCount := mergeCollectMediaJSON(
		collectMediaItemsJSON(displayItems),
		collectMediaItemsJSON(verifyItems),
	)
	resolved.MediaJSON = mediaJSON
	resolved.MediaCount = mediaCount
	resolved.DedupeKey = collectHash(resolved.NormalizedText + ":" + collectMediaSignature(mediaJSON))
	return &resolved, nil
}

func collectMediaJSONWithPurposeItems(mediaJSON string, purpose string) []collectMediaItem {
	data := collectMediaJSONWithPurpose(mediaJSON, purpose)
	return collectMediaRowsToItemsFromJSON(data)
}

func collectMediaItemsJSON(items []collectMediaItem) string {
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func (s *sSysPublish) rebuildCollectOwnedMedia(ctx context.Context, event gdb.Record, content *collectContentResult, owner collectPublishMediaOwner) error {
	if owner.ProfileId <= 0 {
		return gerror.New("采集发布资料不存在")
	}
	canonical, err := s.canonicalCollectProfileMedia(ctx, event, content)
	if err != nil {
		return err
	}
	return s.withProfileMediaSyncLock(ctx, owner.ProfileId, func(ctx context.Context, _ gdb.TX) error {
		items := collectMediaRowsToItemsFromJSON(canonical.MediaJSON)
		now := gtime.Now()
		if _, deleteErr := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
			Where("profile_id", owner.ProfileId).
			WhereNull("deleted_at").
			Data(g.Map{
				"deleted_at": now,
				"deleted_by": event["account_id"].Int64(),
				"updated_at": now,
			}).
			Update(); deleteErr != nil {
			return gerror.Wrap(deleteErr, "清理采集旧媒体失败")
		}
		_ = s.deleteMediaPHashBucketByProfileId(ctx, owner.ProfileId)
		displayItems, verifyItems := classifyCollectPublishMedia(event, items)
		if insertErr := s.insertCollectOwnedMediaRows(ctx, event, owner, "display", displayItems); insertErr != nil {
			return insertErr
		}
		return s.insertCollectOwnedMediaRows(ctx, event, owner, "verify", verifyItems)
	})
}

func classifyCollectPublishMedia(event gdb.Record, items []collectMediaItem) ([]collectMediaItem, []collectMediaItem) {
	text := ""
	if !event.IsEmpty() {
		text = event["raw_text"].String()
	}
	displayItems := make([]collectMediaItem, 0, len(items))
	verifyItems := make([]collectMediaItem, 0)
	unknownItems := make([]collectMediaItem, 0)
	for _, item := range items {
		switch strings.ToLower(strings.TrimSpace(item.Purpose)) {
		case "verify":
			verifyItems = append(verifyItems, item)
		case "display":
			displayItems = append(displayItems, item)
		default:
			unknownItems = append(unknownItems, item)
		}
	}
	if len(verifyItems) > 0 {
		displayItems = append(displayItems, unknownItems...)
		return displayItems, verifyItems
	}
	classification := classifyProfileMessage(text, unknownItems)
	if classification.Kind == profileMessageKindVerify {
		return displayItems, append(verifyItems, unknownItems...)
	}
	return append(displayItems, unknownItems...), nil
}

func (s *sSysPublish) insertCollectOwnedMediaRows(ctx context.Context, event gdb.Record, owner collectPublishMediaOwner, purpose string, items []collectMediaItem) error {
	sortIndex := 1
	for _, item := range items {
		if collectMediaSourceKey(item) == "" {
			continue
		}
		if err := s.insertCollectOwnedMediaRow(ctx, event, owner, purpose, sortIndex, item); err != nil {
			return err
		}
		sortIndex++
	}
	return nil
}

func (s *sSysPublish) insertCollectOwnedMediaRow(ctx context.Context, event gdb.Record, owner collectPublishMediaOwner, purpose string, sortIndex int, item collectMediaItem) error {
	if owner.ProfileId <= 0 {
		return gerror.New("采集发布资料不存在")
	}
	item, err := s.prepareCollectMediaAsset(ctx, event, item)
	if err != nil {
		return err
	}
	mediaType := collectPublishMediaType(item.Type)
	if mediaType == "" {
		return nil
	}
	storagePath := normalizeStoredMediaPath(item.StoragePath)
	fileUrl := strings.TrimSpace(item.FileUrl)
	cacheStatus := tgCacheStatusInvalid
	if strings.TrimSpace(item.FileId) != "" {
		cacheStatus = tgCacheStatusValid
	}
	now := gtime.Now()
	assets, err := processMediaAssetMetadata(ctx, mediaType, storagePath, fileUrl, item.PosterUrl, "")
	if err != nil {
		g.Log().Warning(ctx, "计算采集媒体哈希失败", g.Map{"eventId": event["id"].Int64(), "mediaType": mediaType, "err": err})
	}
	posterURL := strings.TrimSpace(item.PosterUrl)
	posterStoragePath := ""
	perceptualHash := ""
	if assets != nil {
		perceptualHash = assets.PerceptualHash
		posterURL = firstNonEmpty(assets.PosterURL, posterURL)
		posterStoragePath = assets.PosterStoragePath
	}
	_, err = s.saveMediaRecordAndIndex(ctx, g.Map{
		"tenant_id":           event["tenant_id"].Int64(),
		"merchant_id":         event["tenant_id"].Int64(),
		"account_id":          event["account_id"].Int64(),
		"profile_id":          owner.ProfileId,
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
	}, "创建采集媒体失败")
	return err
}
