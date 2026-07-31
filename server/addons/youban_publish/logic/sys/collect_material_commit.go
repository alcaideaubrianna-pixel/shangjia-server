package sys

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	iservice "hotgo/internal/service"
)

const collectProfileSourceType = "youban_collect"

type collectPreparedMedia struct {
	EventMediaId      int64
	Purpose           string
	SortIndex         int
	MediaType         string
	FileId            string
	FileURL           string
	StoragePath       string
	PosterURL         string
	PosterStoragePath string
	PerceptualHash    string
	Size              int64
	MD5               string
	MetaJSON          string
}

type collectPreparedMaterial struct {
	Content *collectContentResult
	Media   []collectPreparedMedia
}

func (s *sSysPublish) commitCollectMaterial(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record, text string) (int64, error) {
	prepared, err := s.prepareCollectMaterial(ctx, event, content)
	if err != nil {
		return 0, err
	}
	return s.commitCollectPreparedProfile(ctx, event, prepared, rule, text)
}

func (s *sSysPublish) prepareCollectMaterial(ctx context.Context, event gdb.Record, content *collectContentResult) (*collectPreparedMaterial, error) {
	canonical, err := s.canonicalCollectProfileMedia(ctx, event, content)
	if err != nil {
		return nil, gerror.Wrap(err, "整理采集资料媒体失败")
	}
	return s.prepareCollectMaterialSnapshot(ctx, event, canonical)
}

func (s *sSysPublish) prepareCollectMaterialSnapshot(ctx context.Context, event gdb.Record, snapshot *collectContentResult) (*collectPreparedMaterial, error) {
	if err := validateCollectMaterialMedia(snapshot); err != nil {
		return nil, err
	}
	items := snapshot.Media
	prepared := &collectPreparedMaterial{Content: snapshot, Media: make([]collectPreparedMedia, 0, len(items))}
	displayItems, verifyItems := classifyCollectPublishMedia(event, items)
	for _, group := range []struct {
		purpose string
		items   []collectMediaItem
	}{
		{purpose: collectMaterialRoleDisplay, items: displayItems},
		{purpose: collectMaterialRoleVerify, items: verifyItems},
	} {
		purpose := group.purpose
		purposeItems := group.items
		for index, item := range purposeItems {
			item, err := s.prepareCollectMediaAsset(ctx, event, item)
			if err != nil {
				return nil, gerror.Wrap(err, "准备采集媒体失败")
			}
			mediaType := collectPublishMediaType(item.Type)
			if mediaType == "" {
				return nil, gerror.New("采集资料包含不支持的媒体类型")
			}
			storagePath := normalizeStoredMediaPath(item.StoragePath)
			fileURL := strings.TrimSpace(item.FileUrl)
			mediaSize := mediaStorageSize(storagePath)
			mediaMD5 := strings.TrimSpace(item.FileMd5)
			if fileURL == "" && storagePath != "" {
				attachment, uploadErr := s.uploadCollectMediaToStorage(ctx, mediaType, storagePath)
				if uploadErr != nil {
					return nil, gerror.Wrap(uploadErr, "保存采集媒体 CDN 资源失败")
				}
				fileURL = strings.TrimSpace(attachment.FileUrl)
				if attachment.Path != "" {
					storagePath = normalizeStoredMediaPath(attachment.Path)
				}
				if attachment.Size > 0 {
					mediaSize = attachment.Size
				}
				if strings.TrimSpace(attachment.Md5) != "" {
					mediaMD5 = strings.TrimSpace(attachment.Md5)
				}
				if fileURL == "" && storagePath == "" {
					return nil, gerror.New("采集媒体上传完成但没有可用地址")
				}
			}
			assets, assetErr := processMediaAssetMetadata(ctx, mediaType, storagePath, fileURL, item.PosterUrl, "")
			if assetErr != nil {
				return nil, gerror.Wrap(assetErr, "处理采集媒体指纹失败")
			}
			prepared.Media = append(prepared.Media, collectPreparedMedia{
				EventMediaId:      item.EventMediaId,
				Purpose:           purpose,
				SortIndex:         index + 1,
				MediaType:         mediaType,
				FileId:            strings.TrimSpace(item.FileId),
				FileURL:           fileURL,
				StoragePath:       storagePath,
				PosterURL:         firstNonEmpty(assetPosterURL(assets), strings.TrimSpace(item.PosterUrl)),
				PosterStoragePath: assetPosterStoragePath(assets),
				PerceptualHash:    assetPerceptualHash(assets),
				Size:              mediaSize,
				MD5:               mediaMD5,
				MetaJSON:          strings.TrimSpace(item.DebugMetaJson),
			})
		}
	}
	if len(prepared.Media) != snapshot.MediaCount {
		return nil, gerror.New("采集资料媒体准备数量不完整")
	}
	if err := s.persistPreparedCollectMedia(ctx, prepared.Media); err != nil {
		return nil, err
	}
	prepared.Content = collectPreparedContentSnapshot(prepared)
	return prepared, nil
}

func collectPreparedContentSnapshot(prepared *collectPreparedMaterial) *collectContentResult {
	if prepared == nil || prepared.Content == nil {
		return nil
	}
	content := *prepared.Content
	items := make([]collectMediaItem, 0, len(prepared.Media))
	for _, media := range prepared.Media {
		mediaType := strings.ToLower(strings.TrimSpace(media.MediaType))
		if mediaType == "image" {
			mediaType = "photo"
		}
		items = append(items, collectMediaItem{
			EventMediaId: media.EventMediaId,
			Type:         mediaType, Purpose: media.Purpose, FileId: media.FileId,
			FileUrl: media.FileURL, StoragePath: media.StoragePath,
			PosterUrl: media.PosterURL, FileMd5: media.MD5,
			FilePhash: media.PerceptualHash, DebugMetaJson: media.MetaJSON,
		})
	}
	content.Media = items
	content.MediaCount = len(items)
	content.DedupeKey = collectHash(content.NormalizedText + ":" + collectMediaSignature(content.Media))
	return &content
}

func (s *sSysPublish) persistPreparedCollectMedia(ctx context.Context, media []collectPreparedMedia) error {
	if len(media) == 0 {
		return nil
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, item := range media {
			if item.EventMediaId <= 0 {
				return gerror.New("采集媒体缺少稳定记录标识")
			}
			fileURL := normalizeMediaFileURL(item.FileURL, item.StoragePath)
			if fileURL == "" || isCollectMediaCachePath(item.StoragePath) {
				return gerror.Newf("采集媒体尚未持久化到统一存储 mediaId:%d", item.EventMediaId)
			}
			result, err := tx.Model(pdao.YoubanPublishCollectEventMedia.Table()).Ctx(ctx).
				Where("id", item.EventMediaId).
				Data(g.Map{
					"file_url":      fileURL,
					"storage_path":  normalizeStoredMediaPath(item.StoragePath),
					"poster_url":    normalizeMediaFileURL(item.PosterURL, item.PosterStoragePath),
					"file_md5":      strings.TrimSpace(item.MD5),
					"file_phash":    strings.TrimSpace(item.PerceptualHash),
					"cache_status":  collectMediaCacheReady,
					"error_message": "",
					"updated_at":    gtime.Now(),
				}).Update()
			if err != nil {
				return gerror.Wrapf(err, "回写采集媒体持久化地址失败 mediaId:%d", item.EventMediaId)
			}
			affected, err := result.RowsAffected()
			if err != nil {
				return gerror.Wrapf(err, "读取采集媒体回写结果失败 mediaId:%d", item.EventMediaId)
			}
			if affected != 1 {
				return gerror.Newf("采集媒体持久化回写记录不存在 mediaId:%d", item.EventMediaId)
			}
		}
		return nil
	})
}

func isCollectMediaCachePath(path string) bool {
	path = strings.TrimLeft(normalizeStoredMediaPath(path), "/")
	return strings.HasPrefix(path, "storage/cache/youban_publish/media/")
}

func validateCollectMaterialMedia(content *collectContentResult) error {
	if content == nil || content.MediaCount <= 0 {
		return nil
	}
	if len(content.Media) != content.MediaCount {
		return gerror.New("采集资料媒体快照数量不完整，等待媒体准备完成")
	}
	for _, item := range content.Media {
		if collectPublishMediaType(item.Type) == "" {
			return gerror.New("采集资料包含无效媒体，等待重新处理")
		}
		if strings.TrimSpace(item.StoragePath) == "" && strings.TrimSpace(item.FileUrl) == "" {
			return gerror.New("采集资料媒体尚未全部下载完成")
		}
	}
	return nil
}

func assetPosterURL(assets *mediaAssetMetadata) string {
	if assets == nil {
		return ""
	}
	return assets.PosterURL
}

func assetPosterStoragePath(assets *mediaAssetMetadata) string {
	if assets == nil {
		return ""
	}
	return assets.PosterStoragePath
}

func assetPerceptualHash(assets *mediaAssetMetadata) string {
	if assets == nil {
		return ""
	}
	return assets.PerceptualHash
}

func mediaStorageSize(path string) int64 {
	if info, err := os.Stat(path); err == nil {
		return info.Size()
	}
	return 0
}

func (s *sSysPublish) commitCollectPreparedProfile(ctx context.Context, event gdb.Record, content *collectPreparedMaterial, rule gdb.Record, text string) (int64, error) {
	if content == nil || content.Content == nil {
		return 0, gerror.New("采集资料媒体尚未准备完成")
	}
	text = strings.TrimSpace(text)
	title := collectTitle(text)
	if title == "" {
		return 0, gerror.New("采集资料标题为空")
	}
	tenantId := event["tenant_id"].Int64()
	accountId := event["account_id"].Int64()
	metadata, err := s.enrichProfileMetadata(ctx, text)
	if err != nil {
		return 0, err
	}
	sourceKey := collectPublishClientRequestId(event, rule)
	now := gtime.Now()
	imageCount, videoCount, hasVerificationVideo := collectPreparedMediaCounts(content.Media)
	var profileId int64
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		columns := dao.ContentProfile.Columns()
		existing, txErr := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).
			Fields(columns.Id).
			Where(columns.SourceKey, sourceKey).
			WhereNull(columns.DeletedAt).
			One()
		if txErr != nil {
			return gerror.Wrap(txErr, "读取采集资料失败")
		}
		data := g.Map{
			columns.SourceType: collectProfileSourceType, columns.SourceKey: sourceKey,
			columns.SourceTextHash: collectHash(text), columns.Title: title,
			columns.Summary: profileSummary(text), columns.PlainText: text,
			columns.Province: metadata.Province, columns.City: metadata.City,
			columns.CupSize: metadata.Tag, columns.Visibility: consts.ContentVisibilityPublic,
			columns.ReviewStatus: consts.ContentReviewApproved, columns.ImportStatus: "collect",
			columns.SourceUpdateBy: fmt.Sprintf("%d", accountId), columns.SourceUpdatedAt: now,
			columns.Status: 2, columns.PublishedAt: nil, columns.UpdatedAt: now, columns.DeletedAt: nil,
			columns.ImageCount: imageCount, columns.VideoCount: videoCount,
			columns.HasVerificationVideo: hasVerificationVideo,
		}
		if existing.IsEmpty() {
			data[columns.SourceNoteUuid] = newPublishProfileUUID()
			data[columns.SourceCreateBy] = fmt.Sprintf("%d", accountId)
			data[columns.SourceCreatedAt] = now
			data[columns.CreatedAt] = now
			for i := 0; i < 1000; i++ {
				profileNo, numberErr := s.nextAccountProfileNo(ctx, tx, tenantId, accountId)
				if numberErr != nil {
					return numberErr
				}
				data[columns.ProfileNo] = profileNo
				profileId, txErr = tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Data(data).InsertAndGetId()
				if txErr == nil {
					break
				}
				if !isProfileNoUniqueConstraintError(txErr) {
					return gerror.Wrap(txErr, "创建采集资料失败")
				}
			}
			if profileId <= 0 {
				return gerror.New("创建采集资料失败")
			}
		} else {
			profileId = existing[columns.Id].Int64()
			if _, txErr = tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(columns.Id, profileId).Data(data).Update(); txErr != nil {
				return gerror.Wrap(txErr, "更新采集资料失败")
			}
		}
		if txErr = s.upsertProfileStateTx(ctx, tx, profileId, tenantId, accountId, "", "", 0, nil); txErr != nil {
			return txErr
		}
		if _, txErr = tx.Model(publishMediaTable).Safe().Ctx(ctx).Where("profile_id", profileId).WhereNull("deleted_at").Data(g.Map{"deleted_at": now, "deleted_by": accountId, "updated_at": now}).Update(); txErr != nil {
			return gerror.Wrap(txErr, "清理采集旧媒体失败")
		}
		for _, media := range content.Media {
			_, txErr = tx.Model(publishMediaTable).Safe().Ctx(ctx).Data(g.Map{
				"tenant_id": tenantId, "merchant_id": tenantId, "account_id": accountId, "profile_id": profileId,
				"media_type": media.MediaType, "purpose": media.Purpose, "name": fmt.Sprintf("collect-%d-%s-%d", event["id"].Int64(), media.Purpose, media.SortIndex),
				"tg_file_id": media.FileId, "file_url": media.FileURL, "storage_path": media.StoragePath, "size": media.Size, "md5": media.MD5,
				"tg_cache_asset_hash": mediaAssetHash(media.StoragePath, media.FileURL), "tg_cache_status": tgCacheStatusValid,
				"poster_url": media.PosterURL, "poster_storage_path": media.PosterStoragePath, "perceptual_hash": media.PerceptualHash,
				"sort_index": media.SortIndex, "status": 1, "created_at": now, "updated_at": now,
				"created_by": accountId, "updated_by": accountId,
			}).InsertAndGetId()
			if txErr != nil {
				return gerror.Wrap(txErr, "写入采集资料媒体失败")
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if err = s.deleteMediaPHashBucketByProfileId(ctx, profileId); err != nil {
		return 0, err
	}
	var mediaRows []struct {
		Id int64 `json:"id"`
	}
	if err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Fields("id").Where("profile_id", profileId).WhereNull("deleted_at").
		Where("perceptual_hash IS NOT NULL").Where("perceptual_hash <> ''").
		Scan(&mediaRows); err != nil {
		return 0, gerror.Wrap(err, "读取采集资料媒体索引失败")
	}
	for _, mediaRow := range mediaRows {
		if err = s.syncMediaPHashBucketByMediaId(ctx, mediaRow.Id); err != nil {
			return 0, err
		}
	}
	if err = s.syncProfileNoteIndex(ctx, profileId); err != nil {
		return 0, err
	}
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	return profileId, nil
}

func collectPreparedMediaCounts(media []collectPreparedMedia) (imageCount int, videoCount int, hasVerificationVideo int) {
	for _, item := range media {
		switch strings.ToLower(strings.TrimSpace(item.MediaType)) {
		case "image":
			imageCount++
		case "video":
			videoCount++
			if strings.EqualFold(strings.TrimSpace(item.Purpose), collectMaterialRoleVerify) {
				hasVerificationVideo = 1
			}
		}
	}
	return
}
