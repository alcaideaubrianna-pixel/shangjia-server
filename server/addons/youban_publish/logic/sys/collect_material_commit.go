package sys

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
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

type telegramCollectorMediaCacheState struct {
	Fingerprint string
	MD5         string
	MimeType    string
	Size        int64
	Claimed     bool
	Entry       *collectorin.MediaCacheEntry
}

func telegramCollectorMediaCacheURL(entry *collectorin.MediaCacheEntry) string {
	if entry == nil {
		return ""
	}
	if value := strings.TrimSpace(entry.FileURL); strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	if value := strings.TrimSpace(entry.StoragePath); strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	return ""
}

func calculateTelegramMediaFingerprint(localPath string, mediaType string, mimeType string) (string, int64, string, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return "", 0, "", gerror.Wrap(err, "读取采集媒体文件信息失败")
	}
	if info.IsDir() {
		return "", 0, "", gerror.New("采集媒体缓存路径不能是目录")
	}
	md5Value, err := fileMD5(localPath)
	if err != nil {
		return "", 0, "", gerror.Wrap(err, "计算采集媒体MD5失败")
	}
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(localPath)))
	}
	return md5Value, info.Size(), mimeType, nil
}

func lookupTelegramCollectorMediaCache(ctx context.Context, mediaType string, md5Value string, size int64, mimeType string) (*collectorin.MediaCacheEntry, bool, error) {
	md5Value = strings.TrimSpace(md5Value)
	mimeType = strings.TrimSpace(mimeType)
	if md5Value == "" || size <= 0 {
		return nil, false, nil
	}
	fingerprint := collectorservice.BuildMediaFingerprint(md5Value, size, telegramCollectorMediaKind(mediaType), mimeType)
	entry, ready, err := collectorservice.Collector().MediaCache(ctx, fingerprint)
	if err != nil || !ready || entry == nil || telegramCollectorMediaCacheURL(entry) == "" {
		return entry, false, err
	}
	return entry, true, nil
}

func acquireTelegramCollectorMediaCache(ctx context.Context, mediaType, storagePath, md5Value string, size int64, mimeType string) (*telegramCollectorMediaCacheState, error) {
	if !collectorservice.Collector().Enabled(ctx) {
		return nil, nil
	}
	md5Value = strings.TrimSpace(md5Value)
	mimeType = strings.TrimSpace(mimeType)
	localPath := ""
	if strings.TrimSpace(storagePath) != "" {
		resolved, err := resolveMediaLocalPath(storagePath)
		if err == nil && fileExists(resolved) {
			localPath = resolved
		}
	}
	if localPath != "" {
		if size <= 0 {
			if info, err := os.Stat(localPath); err == nil {
				size = info.Size()
			}
		}
		if md5Value == "" {
			value, err := fileMD5(localPath)
			if err != nil {
				return nil, gerror.Wrap(err, "计算采集媒体MD5失败")
			}
			md5Value = value
		}
		if mimeType == "" {
			mimeType = mime.TypeByExtension(strings.ToLower(filepath.Ext(localPath)))
		}
	}
	if md5Value == "" || size <= 0 {
		return nil, nil
	}
	fingerprint := collectorservice.BuildMediaFingerprint(md5Value, size, telegramCollectorMediaKind(mediaType), mimeType)
	entry, ready, err := collectorservice.Collector().MediaCache(ctx, fingerprint)
	if err != nil {
		return nil, err
	}
	state := &telegramCollectorMediaCacheState{Fingerprint: fingerprint, MD5: md5Value, MimeType: mimeType, Size: size}
	if ready && entry != nil && telegramCollectorMediaCacheURL(entry) != "" {
		state.Entry = entry
		return state, nil
	}
	claimed, err := collectorservice.Collector().ClaimMediaProcessing(ctx, fingerprint, 30*time.Minute)
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, newCollectMediaRetryError("相同Telegram媒体正在由其他Worker处理，等待复用结果", 5*time.Second)
	}
	state.Claimed = true
	return state, nil
}

func releaseTelegramCollectorMediaCache(ctx context.Context, state *telegramCollectorMediaCacheState, cause error) {
	if state == nil || !state.Claimed || strings.TrimSpace(state.Fingerprint) == "" {
		return
	}
	if err := collectorservice.Collector().ReleaseMediaProcessing(ctx, state.Fingerprint, cause); err != nil {
		g.Log().Warningf(ctx, "释放Telegram采集媒体处理租约失败 fingerprint:%s err:%+v", state.Fingerprint, err)
	}
}

func telegramCollectorMediaKind(mediaType string) string {
	if strings.EqualFold(strings.TrimSpace(mediaType), "video") {
		return collectorin.MediaKindVideo
	}
	return collectorin.MediaKindPhoto
}

func (s *sSysPublish) commitCollectMaterial(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record, text string) (int64, error) {
	text = s.normalizeCollectMaterialText(ctx, event, rule, text)
	prepared, err := s.prepareCollectMaterial(ctx, event, content)
	if err != nil {
		return 0, err
	}
	return s.commitCollectPreparedProfile(ctx, event, prepared, rule, text)
}

// normalizeCollectMaterialText is the final text boundary before a collected
// profile is persisted. Upstream rule evaluation normally performs this work,
// but repeated events and retry/update paths must not be able to write raw text
// back over the cleaned profile text.
func (s *sSysPublish) normalizeCollectMaterialText(ctx context.Context, event gdb.Record, rule gdb.Record, text string) string {
	if !rule["truncate_intro_fee_enabled"].Bool() {
		return text
	}
	// The rule engine appends user-configured footer text after cleaning the
	// source body. When a footer is configured, do not re-truncate the final
	// caption at this boundary; retry/merge records may not carry the footer
	// field and would otherwise delete the user's appended text.
	if strings.TrimSpace(rule["footer_markdown"].String()) != "" {
		return text
	}
	// Footer/追加文案 is intentionally user-authored and may itself contain
	// the keyword. Only normalize the source body before the exact footer.
	footer := strings.TrimSpace(rule["footer_markdown"].String())
	body := text
	footerSuffix := ""
	if footer != "" {
		candidate := "\n" + footer
		if strings.HasSuffix(text, candidate) {
			body = strings.TrimSuffix(text, candidate)
			footerSuffix = candidate
		}
	}
	cleaned := applyCollectIntroFeeTruncate(body) + footerSuffix
	if cleaned != text {
		g.Log().Infof(ctx, "采集资料提交前执行介绍费兜底清洗 eventId:%d ruleId:%d beforeLen:%d afterLen:%d", event["id"].Int64(), rule["id"].Int64(), len([]rune(text)), len([]rune(cleaned)))
	}
	return cleaned
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
			if mediaSize <= 0 {
				mediaSize = item.SourceSize
			}
			mediaMD5 := strings.TrimSpace(item.FileMd5)
			cacheState, cacheErr := acquireTelegramCollectorMediaCache(ctx, mediaType, storagePath, mediaMD5, mediaSize, item.SourceMimeType)
			if cacheErr != nil {
				return nil, cacheErr
			}
			if cacheState != nil {
				mediaMD5 = cacheState.MD5
				mediaSize = cacheState.Size
			}
			var assets *mediaAssetMetadata
			if cacheState != nil && cacheState.Entry != nil {
				cachedPath := strings.TrimSpace(cacheState.Entry.StoragePath)
				cachedURL := telegramCollectorMediaCacheURL(cacheState.Entry)
				if cachedURL != "" {
					fileURL = cachedURL
					if strings.HasPrefix(cachedPath, "http://") || strings.HasPrefix(cachedPath, "https://") {
						storagePath = ""
					} else {
						storagePath = strings.TrimSpace(cachedPath)
					}
				} else if cachedPath != "" {
					storagePath = normalizeStoredMediaPath(cachedPath)
				}
				assets = &mediaAssetMetadata{
					PerceptualHash:    cacheState.Entry.PHash,
					PosterURL:         firstNonEmpty(cacheState.Entry.PosterURL, cacheState.Entry.PosterStoragePath),
					PosterStoragePath: cacheState.Entry.PosterStoragePath,
				}
			}
			if assets == nil && fileURL == "" && storagePath != "" {
				attachment, uploadErr := s.uploadCollectMediaToStorage(ctx, mediaType, storagePath)
				if uploadErr != nil {
					releaseTelegramCollectorMediaCache(ctx, cacheState, uploadErr)
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
					emptyErr := gerror.New("采集媒体上传完成但没有可用地址")
					releaseTelegramCollectorMediaCache(ctx, cacheState, emptyErr)
					return nil, emptyErr
				}
			}
			if assets == nil {
				var assetErr error
				assets, assetErr = processMediaAssetMetadata(ctx, mediaType, storagePath, fileURL, item.PosterUrl, "")
				if assetErr != nil {
					releaseTelegramCollectorMediaCache(ctx, cacheState, assetErr)
					return nil, gerror.Wrap(assetErr, "处理采集媒体指纹失败")
				}
				if cacheState != nil && cacheState.Claimed {
					entry := &collectorin.MediaCacheEntry{
						Fingerprint:       cacheState.Fingerprint,
						FileURL:           fileURL,
						StoragePath:       storagePath,
						PosterURL:         assetPosterURL(assets),
						PosterStoragePath: assetPosterStoragePath(assets),
						PHash:             assetPerceptualHash(assets),
						Kind:              telegramCollectorMediaKind(mediaType),
						MimeType:          cacheState.MimeType,
						Size:              mediaSize,
					}
					if saveErr := collectorservice.Collector().SaveMediaReady(ctx, entry, 0); saveErr != nil {
						releaseTelegramCollectorMediaCache(ctx, cacheState, saveErr)
						return nil, gerror.Wrap(saveErr, "保存Telegram采集媒体复用索引失败")
					}
				}
			}
			preparedMedia := collectPreparedMedia{
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
			}
			if err = validatePreparedCollectMedia(preparedMedia); err != nil {
				return nil, err
			}
			prepared.Media = append(prepared.Media, preparedMedia)
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

func validatePreparedCollectMedia(media collectPreparedMedia) error {
	if !strings.EqualFold(strings.TrimSpace(media.MediaType), "video") {
		return nil
	}
	if strings.TrimSpace(media.PosterURL) == "" && strings.TrimSpace(media.PosterStoragePath) == "" {
		return gerror.New("采集视频预览图尚未生成，等待媒体处理完成")
	}
	return nil
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
			Fields(columns.Id, columns.SourceKey, columns.HasVerificationVideo, columns.ImageCount, columns.VideoCount).
			Where(columns.SourceKey, sourceKey).
			WhereNull(columns.DeletedAt).
			One()
		if txErr != nil {
			return gerror.Wrap(txErr, "读取采集资料失败")
		}
		if existing.IsEmpty() {
			existing, txErr = existingLegacyCollectProfile(ctx, tx, event)
			if txErr != nil {
				return txErr
			}
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
			if err = replaceProfileChannelMappings(ctx, tx, tenantId, accountId, profileId, collectRuleTargetChannelIds(rule)); err != nil {
				return err
			}
		} else {
			profileId = existing[columns.Id].Int64()
			if !collectProfileMaterialShouldReplace(existing, imageCount, videoCount, hasVerificationVideo) {
				if existing[columns.SourceKey].String() != sourceKey {
					if _, txErr = tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(columns.Id, profileId).Data(g.Map{columns.SourceKey: sourceKey, columns.UpdatedAt: now}).Update(); txErr != nil {
						return gerror.Wrap(txErr, "升级采集资料身份键失败")
					}
				}
				return nil
			}
			if _, txErr = tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(columns.Id, profileId).Data(data).Update(); txErr != nil {
				return gerror.Wrap(txErr, "更新采集资料失败")
			}
		}
		if txErr = s.upsertProfileStateTx(ctx, tx, profileId, tenantId, accountId, "", 0, nil); txErr != nil {
			return txErr
		}
		if _, txErr = tx.Model(publishMediaTable).Safe().Ctx(ctx).Where("profile_id", profileId).WhereNull("deleted_at").Data(g.Map{"deleted_at": now, "deleted_by": accountId, "updated_at": now}).Update(); txErr != nil {
			return gerror.Wrap(txErr, "清理采集旧媒体失败")
		}
		for _, media := range content.Media {
			media.PerceptualHash = strings.TrimSpace(media.PerceptualHash)
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

func collectProfileMaterialShouldReplace(existing gdb.Record, imageCount, videoCount, hasVerificationVideo int) bool {
	if existing.IsEmpty() {
		return true
	}
	if existing["has_verification_video"].Int() == 1 && hasVerificationVideo != 1 {
		return false
	}
	if existing["has_verification_video"].Int() != 1 && hasVerificationVideo == 1 {
		return true
	}
	return imageCount+videoCount >= existing["image_count"].Int()+existing["video_count"].Int()
}

func existingLegacyCollectProfile(ctx context.Context, tx gdb.TX, event gdb.Record) (gdb.Record, error) {
	tenantID := event["tenant_id"].Int64()
	accountID := event["account_id"].Int64()
	groupedID := strings.TrimSpace(event["source_grouped_id"].String())
	chatID := normalizeTelegramChannelChatID(event["source_chat_id"].String())
	if tenantID <= 0 || accountID <= 0 || groupedID == "" || chatID == "" {
		return nil, nil
	}
	chatIDs := []string{chatID}
	if strings.HasPrefix(chatID, "-100") {
		chatIDs = append(chatIDs, strings.TrimPrefix(chatID, "-100"))
	}
	conditions := make([]string, 0, len(chatIDs))
	args := make([]interface{}, 0, len(chatIDs))
	for _, candidateChatID := range chatIDs {
		conditions = append(conditions, "p.source_key LIKE ?")
		args = append(args, "%:"+candidateChatID+":group:"+groupedID+":%")
	}
	row, err := tx.Model(dao.ContentProfile.Table()+" p").Ctx(ctx).
		Fields("p.id,p.source_key,p.has_verification_video,p.image_count,p.video_count").
		WhereNull("p.deleted_at").
		Where("("+strings.Join(conditions, " OR ")+")", args[:len(chatIDs)]...).
		Where("EXISTS (SELECT 1 FROM hg_youban_publish_profile_state ps WHERE ps.profile_id=p.id AND ps.tenant_id=? AND ps.account_id=? AND ps.deleted_at IS NULL)", tenantID, accountID).
		OrderDesc("p.has_verification_video").OrderDesc("p.id").Limit(1).One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取旧版采集资料失败")
	}
	return row, nil
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
