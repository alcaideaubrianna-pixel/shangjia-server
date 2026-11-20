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
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/profileextractor"
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

func lookupTelegramCollectorMediaIdentityCache(ctx context.Context, mediaType, fileUniqueID string, mediaID int64, dcID int, variant string) (*collectorin.MediaCacheEntry, bool, error) {
	if strings.TrimSpace(fileUniqueID) == "" && mediaID <= 0 {
		return nil, false, nil
	}
	fingerprint := collectorservice.BuildMediaIdentityFingerprint(fileUniqueID, mediaID, dcID, telegramCollectorMediaKind(mediaType), variant)
	entry, ready, err := collectorservice.Collector().MediaCache(ctx, fingerprint)
	if err != nil || !ready || entry == nil || telegramCollectorMediaCacheURL(entry) == "" {
		return entry, false, err
	}
	return entry, true, nil
}

func acquireTelegramCollectorMediaCache(ctx context.Context, mediaType, storagePath, fileURL, md5Value string, size int64, mimeType string) (*telegramCollectorMediaCacheState, error) {
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
	if md5Value == "" {
		value, err := mediaSourceMD5(ctx, storagePath, fileURL, mediaType)
		if err != nil {
			return nil, err
		}
		md5Value = value
	}
	if !collectorservice.Collector().Enabled(ctx) || size <= 0 {
		return &telegramCollectorMediaCacheState{MD5: md5Value, Size: size, MimeType: mimeType}, nil
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
		// Another worker may have completed between the initial read and claim.
		entry, ready, err = collectorservice.Collector().MediaCache(ctx, fingerprint)
		if err != nil {
			return nil, err
		}
		if ready && entry != nil {
			if telegramCollectorMediaCacheURL(entry) == "" {
				return nil, gerror.New("Telegram媒体索引已完成但缺少有效资源地址")
			}
			state.Entry = entry
			return state, nil
		}
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
	// Reject unchanged invalid input/config before costly media preparation.
	if collectTitle(text) == "" {
		return 0, gerror.New("采集资料标题为空")
	}
	channelIDs := collectRuleTargetChannelIds(rule)
	if len(channelIDs) == 0 {
		return 0, gerror.New("采集规则未配置目标频道")
	}
	if err := s.ensureActiveProfileChannels(ctx, channelIDs, event["tenant_id"].Int64()); err != nil {
		return 0, err
	}
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
	rawText := authoritativeCollectRawText(event["raw_text"].String())
	preserveNonFeeLines := func(candidate string) string {
		preserved := preserveCollectNonFeeLines(rawText, candidate)
		if preserved != candidate {
			g.Log().Warningf(ctx, "采集资料费用窗口外原文保护已触发 eventId:%d ruleId:%d beforeLen:%d afterLen:%d", event["id"].Int64(), rule["id"].Int64(), len([]rune(candidate)), len([]rune(preserved)))
		}
		return preserved
	}
	footer := strings.TrimSpace(rule["footer_markdown"].String())
	footerSuffix := ""
	if footer != "" {
		candidate := "\n" + footer
		if strings.HasSuffix(text, candidate) {
			text = strings.TrimSuffix(text, candidate)
			footerSuffix = candidate
		}
	}
	// Re-apply all source rules at the persistence boundary. This protects
	// retries/imports/merge paths that may provide the original caption.
	if footerSuffix != "" && rule["truncate_intro_fee_enabled"].Bool() {
		text = normalizeCollectRuleBodyWithoutTruncate(text, rule)
	} else {
		text = normalizeCollectRuleBody(text, rule)
	}
	text += footerSuffix
	if suffix := strings.TrimSpace(rule["intro_fee_suffix"].String()); suffix != "" {
		amountText := normalizeCollectRuleBodyWithoutTruncate(rawText, rule)
		text = applyCollectIntroFeeSuffix(text, amountText, suffix)
		return preserveNonFeeLines(text)
	}
	if !rule["truncate_intro_fee_enabled"].Bool() {
		return preserveNonFeeLines(text)
	}
	if footerSuffix != "" {
		return preserveNonFeeLines(text)
	}
	// The rule engine appends user-configured footer text after cleaning the
	// source body. When a footer is configured, do not re-truncate the final
	// caption at this boundary; retry/merge records may not carry the footer
	// field and would otherwise delete the user's appended text.
	// Footer/追加文案 is intentionally user-authored and may itself contain
	// the keyword. Only normalize the source body before the exact footer.
	body := text
	if footerSuffix != "" {
		body = strings.TrimSuffix(text, footerSuffix)
	}
	cleaned := applyCollectIntroFeeTruncate(body) + footerSuffix
	if cleaned != text {
		g.Log().Infof(ctx, "采集资料提交前执行介绍费兜底清洗 eventId:%d ruleId:%d beforeLen:%d afterLen:%d", event["id"].Int64(), rule["id"].Int64(), len([]rune(text)), len([]rune(cleaned)))
	}
	return preserveNonFeeLines(cleaned)
}

func (s *sSysPublish) prepareCollectMaterial(ctx context.Context, event gdb.Record, content *collectContentResult) (*collectPreparedMaterial, error) {
	ready, err := s.ensureCollectPairedVerifyReady(ctx, event)
	if err != nil {
		return nil, err
	}
	if !ready {
		return nil, newCollectProcessRetryError(collectMaterialWaitingVerifyRetryDelay, "采集准入等待验证视频匹配并写入")
	}
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
			item, err := s.prepareCollectMediaAsset(ctx, event, item, index+1)
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
			assets := reusedPublishedCollectMetadata(item)
			var cacheState *telegramCollectorMediaCacheState
			if assets == nil {
				var cacheErr error
				cacheState, cacheErr = acquireTelegramCollectorMediaCache(ctx, mediaType, storagePath, fileURL, mediaMD5, mediaSize, item.SourceMimeType)
				if cacheErr != nil {
					return nil, cacheErr
				}
			}
			if cacheState != nil {
				mediaMD5 = cacheState.MD5
				mediaSize = cacheState.Size
			}
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
					PosterURL:         cacheState.Entry.PosterURL,
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
				assets, assetErr = processMediaAssetMetadata(ctx, mediaType, storagePath, fileURL, item.PosterUrl, "", mediaMD5)
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
			if mediaMD5 == "" && assets != nil {
				mediaMD5 = assets.MD5
			}
			if mediaMD5 == "" {
				return nil, gerror.New("采集媒体MD5为空，等待重新处理")
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
		return newCollectProcessRetryError(collectMaterialWaitingVerifyRetryDelay, "采集资料等待展示和验证媒体")
	}
	if len(content.Media) != content.MediaCount {
		return gerror.New("采集资料媒体快照数量不完整，等待媒体准备完成")
	}
	hasVerificationVideo := false
	for _, item := range content.Media {
		if item.Type == "video" && strings.EqualFold(strings.TrimSpace(item.Purpose), collectMaterialRoleVerify) {
			hasVerificationVideo = true
		}
		if collectPublishMediaType(item.Type) == "" {
			return gerror.New("采集资料包含无效媒体，等待重新处理")
		}
		if strings.TrimSpace(item.StoragePath) == "" && strings.TrimSpace(item.FileUrl) == "" {
			return gerror.New("采集资料媒体尚未全部下载完成")
		}
	}
	if !hasVerificationVideo {
		return newCollectProcessRetryError(collectMaterialWaitingVerifyRetryDelay, "采集准入等待已匹配验证视频，禁止仅展示入库")
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
	_, _, verification := collectPreparedMediaCounts(content.Media)
	if verification == 0 {
		return 0, newCollectProcessRetryError(collectMaterialWaitingVerifyRetryDelay, "采集提交等待已准备完成的验证视频")
	}
	// Approval of an old review can enter here without processing the event
	// again. Never let that path create a profile from a publish target.
	ctx, blocked, guardErr := s.checkCollectEventPublishTarget(ctx, event)
	if guardErr != nil {
		return 0, guardErr
	}
	if blocked {
		return 0, gerror.New("采集来源是发布目标频道，禁止创建资料")
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
	extracted := profileextractor.Parse(text)
	sourceKey := collectPublishClientRequestId(event, rule)
	if sourceKey == "" || collectRuleSourceID(event, rule) != event["source_id"].Int64() {
		return 0, gerror.New("采集资料身份缺少租户、账户、来源或规则归属")
	}
	now := gtime.Now()
	imageCount, videoCount, hasVerificationVideo := collectPreparedMediaCounts(content.Media)
	fingerprintMedia := make([]collectMediaItem, 0, len(content.Media))
	for _, media := range content.Media {
		fingerprintMedia = append(fingerprintMedia, collectMediaItem{
			Type: media.MediaType, FileMd5: media.MD5, FilePhash: media.PerceptualHash,
		})
	}
	fingerprints := scopeCollectProfileFingerprints(sourceKey, buildProfileFingerprints(collectRuleTargetChannelIds(rule), text, fingerprintMedia))
	cacheFingerprints := fingerprints
	storedChannelIds, err := s.storedProfileChannelIds(ctx, collectRuleTargetChannelIds(rule), event["tenant_id"].Int64())
	if err != nil {
		return 0, err
	}
	var profileId int64
	wasExisting := false
	// This is observational state only; legacy_sent deliberately retains the
	// existing wasExisting control-flow semantics.
	profileCreated := false
	database := g.DB()
	topLevelTransaction := gdb.TXFromCtx(ctx, database.GetGroup()) == nil &&
		gdb.TXFromCtx(database.GetCtx(), database.GetGroup()) == nil
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) (resultErr error) {
		// Keep collected media and their search projections in one transaction.
		// This also repairs indexes when an idempotent retry reuses the profile.
		defer func() {
			if resultErr != nil || profileId <= 0 {
				return
			}
			if resultErr = s.deleteMediaPHashBucketByProfileId(ctx, profileId); resultErr != nil {
				return
			}
			if resultErr = s.syncMediaPHashBucketsByProfileId(ctx, profileId); resultErr != nil {
				return
			}
			resultErr = s.syncProfileNoteIndex(ctx, profileId)
		}()

		columns := dao.ContentProfile.Columns()
		existing, txErr := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).
			Fields(columns.Id, columns.SourceKey, columns.HasVerificationVideo, columns.ImageCount, columns.VideoCount,
				columns.Age, columns.IsVirgin, columns.Height, columns.Weight, columns.CupSize).
			Where(columns.SourceKey, sourceKey).
			WhereNull(columns.DeletedAt).Limit(1).
			One()
		if txErr != nil {
			return gerror.Wrap(txErr, "读取采集资料失败")
		}
		if existing.IsEmpty() {
			existing, txErr = existingLegacyCollectProfile(ctx, tx, event, rule)
			if txErr != nil {
				return txErr
			}
		}
		if !existing.IsEmpty() && existing["legacy_sent"].Bool() {
			// A confirmed branch is immutable here. Dispatch resumption owns
			// terminal handling; never reset its profile/media/publish state.
			profileId = existing[columns.Id].Int64()
			return nil
		}
		if !existing.IsEmpty() {
			wasExisting = true
			extracted = profileextractor.Refresh(text, profileextractor.Fields{
				Age:    existing[columns.Age].Int(),
				Virgin: existing[columns.IsVirgin].Int(),
				Height: existing[columns.Height].Int(),
				Weight: existing[columns.Weight].Int(),
				Cup:    existing[columns.CupSize].String(),
			})
		}
		data := g.Map{
			columns.SourceType: collectProfileSourceType, columns.SourceKey: sourceKey,
			columns.SourceTextHash: collectHash(text), columns.Title: title,
			columns.Summary: profileSummary(text), columns.PlainText: text,
			columns.Province: metadata.Province, columns.City: metadata.City,
			columns.Age:      extracted.Age,
			columns.IsVirgin: extracted.Virgin,
			columns.Height:   extracted.Height, columns.Weight: extracted.Weight,
			columns.CupSize: extracted.Cup, columns.Visibility: consts.ContentVisibilityPublic,
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
			profileCreated = true
		} else {
			profileId = existing[columns.Id].Int64()
			if !collectProfileMaterialShouldReplace(existing, imageCount, videoCount, hasVerificationVideo) {
				if existing[columns.SourceKey].String() != sourceKey {
					if _, txErr = tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(columns.Id, profileId).Data(g.Map{columns.SourceKey: sourceKey, columns.UpdatedAt: now}).Update(); txErr != nil {
						return gerror.Wrap(txErr, "升级采集资料身份键失败")
					}
				}
				if mappingErr := replaceProfileChannelMappings(ctx, tx, tenantId, accountId, profileId, storedChannelIds); mappingErr != nil {
					return mappingErr
				}
				return saveCollectMaterialOriginTx(ctx, tx, profileId, event, rule)
			}
			if _, txErr = tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(columns.Id, profileId).Data(data).Update(); txErr != nil {
				return gerror.Wrap(txErr, "更新采集资料失败")
			}
		}
		if originErr := saveCollectMaterialOriginTx(ctx, tx, profileId, event, rule); originErr != nil {
			return originErr
		}
		if mappingErr := replaceProfileChannelMappings(ctx, tx, tenantId, accountId, profileId, storedChannelIds); mappingErr != nil {
			return mappingErr
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
		if collectRuleDedupeEnabled(rule) && !wasExisting {
			if txErr = attachProfileFingerprintsTx(ctx, tx, tenantId, accountId, profileId, fingerprints); txErr != nil {
				return txErr
			}
		} else {
			if _, cacheFingerprints, txErr = s.replaceProfileFingerprintProjectionTx(ctx, tx, tenantId, accountId, profileId, collectRuleTargetChannelIds(rule)); txErr != nil {
				return txErr
			}
		}
		return nil
	})
	observeCollectProfilePersistence(
		collectBusinessEventContext(ctx, event, nil, rule["id"].Int64()),
		profileId, profileCreated, topLevelTransaction, err,
	)
	if err != nil {
		return 0, err
	}
	warmProfileFingerprintCache(ctx, tenantId, accountId, profileId, cacheFingerprints)
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

func existingLegacyCollectProfile(ctx context.Context, tx gdb.TX, event, rule gdb.Record) (gdb.Record, error) {
	tenantID, accountID := event["tenant_id"].Int64(), event["account_id"].Int64()
	sourceID, ruleID := event["source_id"].Int64(), rule["id"].Int64()
	if tenantID <= 0 || accountID <= 0 || sourceID <= 0 || ruleID <= 0 || collectRuleSourceID(event, rule) != sourceID {
		return nil, gerror.New("旧版采集资料身份归属不完整")
	}
	accountScope := collectEventAccountScope(event)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	previousCtx := tx.GetCtx()
	defer tx.Ctx(previousCtx)
	const candidateLimit = 128
	e := pdao.YoubanPublishCollectEvent.Columns()
	query := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).
		Fields(e.Id, e.SourceUniqueKey, e.TenantId, e.AccountId, e.SourceId, e.SourceType, e.BotId,
			e.SourceChatId, e.SourceGroupedId, e.SourceMessageId).
		Where(e.TenantId, tenantID).WhereIn(e.AccountId, accountScope).Where(e.SourceId, sourceID)
	chatID := normalizeTelegramChannelChatID(event["source_chat_id"].String())
	if chatID != "" && strings.TrimSpace(event["source_grouped_id"].String()) != "" {
		query = query.WhereIn(e.SourceChatId, tgChannelCacheLookupIds(chatID)).Where(e.SourceGroupedId, strings.TrimSpace(event["source_grouped_id"].String()))
	} else if chatID != "" && event["source_message_id"].Int64() > 0 {
		query = query.WhereIn(e.SourceChatId, tgChannelCacheLookupIds(chatID)).Where(e.SourceGroupedId, "").Where(e.SourceMessageId, event["source_message_id"].Int64())
	} else if event["id"].Int64() > 0 {
		query = query.Where(e.Id, event["id"].Int64())
	} else {
		return nil, gerror.New("旧版采集资料缺少精确消息身份")
	}
	events, err := query.OrderAsc(e.Id).Limit(candidateLimit + 1).All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取旧版采集资料身份事件失败")
	}
	if len(events) > candidateLimit {
		return nil, gerror.New("旧版采集资料身份事件超过安全上限，拒绝创建可能重复的资料")
	}
	eventIDs := []int64{}
	keys := legacyCollectProfileKeys(event, ruleID)
	storageEvent := make(gdb.Record, len(event))
	for key, value := range event {
		storageEvent[key] = value
	}
	storageEvent["account_id"] = g.NewVar(collectEventStorageAccountID(event))
	keys = append(keys, legacyCollectProfileKeys(storageEvent, ruleID)...)
	for _, identity := range events {
		eventIDs = append(eventIDs, identity[e.Id].Int64())
		keys = append(keys, legacyCollectProfileKeys(identity, ruleID)...)
	}
	if event["id"].Int64() > 0 {
		eventIDs = append(eventIDs, event["id"].Int64())
	}
	eventIDs = uniqueIds(eventIDs)
	keys = uniqueStrings(keys)
	if len(eventIDs) == 0 {
		return nil, gerror.New("旧版采集资料缺少可验证来源，拒绝盲目新建")
	}

	// Old v2/v3 keys locate candidates only. Read exact branch dispatches
	// first so the profile lookup is an indexed key/primary-key lookup, not a
	// correlated scan of the entire profile table.
	d := pdao.YoubanPublishCollectDispatch.Columns()
	dispatches, err := tx.Model(pdao.YoubanPublishCollectDispatch.Table()).Ctx(ctx).
		Fields(d.ProfileId, d.Status).Where(d.TenantId, tenantID).WhereIn(d.AccountId, accountScope).
		Where(d.SourceId, sourceID).Where(d.RuleId, ruleID).WhereIn(d.EventId, eventIDs).
		WhereGT(d.ProfileId, 0).OrderAsc(d.Id).Limit(candidateLimit + 1).All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取旧版采集资料分支映射失败")
	}
	if len(dispatches) > candidateLimit {
		return nil, gerror.New("旧版采集资料分发超过安全上限")
	}
	mapped := make(map[int64]bool)
	sent := make(map[int64]bool)
	for _, dispatch := range dispatches {
		id := dispatch[d.ProfileId].Int64()
		mapped[id] = true
		if dispatch[d.Status].String() == sysin.CollectDispatchStatusSent {
			sent[id] = true
		}
	}
	mappedIDs := make([]int64, 0, len(mapped))
	for id := range mapped {
		mappedIDs = append(mappedIDs, id)
	}
	if len(mappedIDs) > candidateLimit {
		return nil, gerror.New("旧版采集资料映射超过安全上限")
	}
	if len(mappedIDs) == 0 && len(keys) == 0 {
		return nil, gerror.New("旧版采集资料缺少可验证来源，拒绝盲目新建")
	}
	model := tx.Model(dao.ContentProfile.Table() + " p").Ctx(ctx).Unscoped().
		Fields("p.id,p.source_key,p.deleted_at,p.has_verification_video,p.image_count,p.video_count,p.age,p.is_virgin,p.height,p.weight,p.cup_size")
	if len(keys) == 0 {
		model = model.WhereIn("p.id", mappedIDs)
	} else if len(mappedIDs) > 0 {
		model = model.Where("(p.source_key IN (?) OR p.id IN (?))", keys, mappedIDs)
	} else {
		model = model.WhereIn("p.source_key", keys)
	}
	rows, err := model.OrderAsc("p.id").Limit(candidateLimit + 1).All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取旧版采集资料失败")
	}
	if len(rows) > candidateLimit {
		return nil, gerror.New("旧版采集资料候选超过安全上限")
	}
	if len(rows) == 0 {
		if len(mappedIDs) > 0 {
			return nil, gerror.New("旧版采集分支映射指向缺失或异账户资料，拒绝重复创建")
		}
		return nil, nil
	}
	profileIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		profileIDs = append(profileIDs, row["id"].Int64())
	}
	// Resolve profile ownership AFTER exact keys/PKs, using only the real
	// business account. A legacy account-zero profile must raise an error,
	// not disappear from candidates and cause a duplicate profile insertion.
	states, stateErr := tx.Model(publishProfileStateTable).Ctx(ctx).
		Fields("profile_id").WhereIn("profile_id", profileIDs).
		Where("tenant_id", tenantID).Where("account_id", accountID).WhereNull("deleted_at").
		Limit(candidateLimit + 1).All()
	if stateErr != nil {
		return nil, gerror.Wrap(stateErr, "校验旧版采集资料真实账户归属失败")
	}
	ownedProfiles := make(map[int64]bool, len(states))
	for _, state := range states {
		ownedProfiles[state["profile_id"].Int64()] = true
	}
	for _, id := range profileIDs {
		if !ownedProfiles[id] {
			return nil, gerror.New("旧版采集资料真实账户归属冲突或已删除，拒绝迁移或重复创建")
		}
	}
	// Every dispatch reference must resolve in this ownership scope, even
	// when unrelated exact-key candidates happen to exist as well.
	candidateIDs := make(map[int64]bool, len(profileIDs))
	for _, id := range profileIDs {
		candidateIDs[id] = true
	}
	for _, id := range mappedIDs {
		if !candidateIDs[id] {
			return nil, gerror.New("旧版采集分支存在缺失资料映射，拒绝重复创建")
		}
	}
	originEnabled, err := profileOriginEnabled(ctx)
	if err != nil {
		return nil, err
	}
	if originEnabled {
		// Resolve origin only AFTER exact-key/dispatch candidates are bounded.
		// profile_id is the leading lookup index; source/event alone are not.
		// Origin has no rule_id, so the channel-profile mapping must also
		// prove the exact source AND rule.
		origins, readErr := tx.Model(pdao.YoubanPublishProfileOrigin.Table()+" o").Ctx(ctx).
			Fields("o.profile_id").WhereIn("o.profile_id", profileIDs).Where("o.tenant_id", tenantID).WhereIn("o.account_id", accountScope).
			Where("o.source_id", sourceID).WhereIn("o.event_id", eventIDs).Where("o.origin_kind", "collect").
			Where("EXISTS (SELECT 1 FROM "+publishCollectChannelProfileTable+" cp WHERE cp.profile_id=o.profile_id AND cp.tenant_id=? AND cp.account_id IN (?) AND cp.source_id=? AND cp.rule_id=?)", tenantID, accountScope, sourceID, ruleID).
			Group("o.profile_id").OrderAsc("o.profile_id").Limit(candidateLimit + 1).All()
		if readErr != nil {
			return nil, gerror.Wrap(readErr, "读取旧版采集永久来源映射失败")
		}
		if len(origins) > candidateLimit {
			return nil, gerror.New("旧版采集永久来源映射超过安全上限")
		}
		for _, origin := range origins {
			if id := origin["profile_id"].Int64(); id > 0 {
				mapped[id] = true
			}
		}
	}
	// Do not rewrite profiles shared by historical branches, even if our
	// branch also has a valid mapping. Include tombstones as ownership evidence.
	foreign := make(map[int64]bool)
	for _, table := range []string{pdao.YoubanPublishCollectDispatch.Table(), publishCollectChannelProfileTable} {
		conflicts, readErr := tx.Model(table).Ctx(ctx).Unscoped().
			Fields("profile_id").WhereIn("profile_id", profileIDs).
			Where("(tenant_id<>? OR account_id NOT IN (?) OR source_id<>? OR rule_id<>?)", tenantID, accountScope, sourceID, ruleID).
			Group("profile_id").Limit(candidateLimit + 1).All()
		if readErr != nil {
			return nil, gerror.Wrap(readErr, "检查旧版采集资料共享分支失败")
		}
		if len(conflicts) > candidateLimit {
			return nil, gerror.New("旧版采集资料共享分支超过安全上限")
		}
		for _, conflict := range conflicts {
			foreign[conflict["profile_id"].Int64()] = true
		}
	}
	if originEnabled {
		conflicts, readErr := tx.Model(pdao.YoubanPublishProfileOrigin.Table()).Ctx(ctx).
			Fields("profile_id").WhereIn("profile_id", profileIDs).
			Where("(tenant_id<>? OR account_id NOT IN (?) OR source_id<>?)", tenantID, accountScope, sourceID).
			Group("profile_id").Limit(candidateLimit + 1).All()
		if readErr != nil {
			return nil, gerror.Wrap(readErr, "检查旧版采集永久来源归属失败")
		}
		if len(conflicts) > candidateLimit {
			return nil, gerror.New("旧版采集永久来源归属超过安全上限")
		}
		for _, conflict := range conflicts {
			foreign[conflict["profile_id"].Int64()] = true
		}
	}
	foundMapped := 0
	for _, row := range rows {
		id := row["id"].Int64()
		if mapped[id] {
			foundMapped++
			if key := row["source_key"].String(); strings.HasPrefix(key, "collect:v4:") && key != collectPublishClientRequestId(event, rule) {
				return nil, gerror.New("旧版映射与新版资料分支身份冲突，拒绝覆盖")
			}
		}
		row["legacy_mapped"], row["legacy_foreign"], row["legacy_sent"] = g.NewVar(mapped[id]), g.NewVar(foreign[id]), g.NewVar(sent[id])
	}
	if foundMapped != len(mapped) {
		return nil, gerror.New("旧版采集分支存在缺失资料映射，拒绝重复创建")
	}
	return selectLegacyCollectProfile(rows)
}

// Only a single explicitly mapped, unshared profile can be upgraded to v4.
// Unknown candidates stop creation; foreign-only candidates belong to another
// branch and must not prevent this source/rule from creating its own profile.
func selectLegacyCollectProfile(rows []gdb.Record) (gdb.Record, error) {
	var selected gdb.Record
	for _, row := range rows {
		if !row["legacy_mapped"].Bool() {
			if row["legacy_foreign"].Bool() {
				continue
			}
			return nil, gerror.New("旧版采集资料缺少同来源同规则映射，拒绝重复创建")
		}
		if row["legacy_foreign"].Bool() {
			return nil, gerror.New("旧版采集资料被多个来源或规则共享，拒绝覆盖历史分支")
		}
		if !row["deleted_at"].IsNil() {
			return nil, gerror.New("旧版采集资料已删除，拒绝自动恢复")
		}
		if !selected.IsEmpty() {
			return nil, gerror.New("同来源同规则存在多个旧版采集资料，拒绝自动归并")
		}
		selected = row
	}
	return selected, nil
}

func legacyCollectProfileKeys(event gdb.Record, ruleID int64) []string {
	keys := []string{}
	if key := legacyCollectMaterialSourceKey(event["source_unique_key"].String(), ruleID); key != "" {
		keys = append(keys, key)
	}
	tenantID, accountID := event["tenant_id"].Int64(), event["account_id"].Int64()
	chatID := normalizeTelegramChannelChatID(event["source_chat_id"].String())
	if tenantID <= 0 || accountID <= 0 || chatID == "" {
		return keys
	}
	for _, alias := range tgChannelCacheLookupIds(chatID) {
		prefixes := []string{fmt.Sprintf("collect:v2:%d:%d:%s:", tenantID, accountID, alias)}
		if event["bot_id"].Int64() > 0 && event["source_type"].String() == sysin.CollectSourceTypeBot {
			prefixes = append(prefixes, fmt.Sprintf("collect:v3:%d:%d:bot:%d:%s:", tenantID, accountID, event["bot_id"].Int64(), alias))
		}
		for _, prefix := range prefixes {
			if groupID := strings.TrimSpace(event["source_grouped_id"].String()); groupID != "" {
				keys = append(keys, prefix+"group:"+groupID)
			} else if messageID := event["source_message_id"].Int64(); messageID > 0 {
				keys = append(keys, fmt.Sprintf("%smessage:%d", prefix, messageID))
			}
		}
	}
	return uniqueStrings(keys)
}

// Historical keys were created before v2/v3 and ended in the rule ID.
func legacyCollectMaterialSourceKey(uniqueKey string, ruleID int64) string {
	uniqueKey = strings.TrimSpace(uniqueKey)
	if uniqueKey == "" || ruleID <= 0 {
		return ""
	}
	return fmt.Sprintf("collect:%s:%d", uniqueKey, ruleID)
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
