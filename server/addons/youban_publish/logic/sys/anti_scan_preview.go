package sys

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"
	_ "golang.org/x/image/webp"
	"golang.org/x/sync/singleflight"

	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/addons/youban_publish/global"
	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
	"hotgo/internal/consts"
	"hotgo/internal/library/addons"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/contexts"
	lock "hotgo/internal/library/hgrds/lock"
	"hotgo/internal/library/storager"
	basemodel "hotgo/internal/model"
	baseservice "hotgo/internal/service"
	"hotgo/utility/file"
)

const (
	antiScanCacheTable           = "hg_youban_publish_anti_scan_cache"
	antiScanPreviewRenderVersion = 4
	antiScanMattingErrorMessage  = "云端抠图服务暂时不可用，请稍后重试或联系管理员检查云资源额度"
)

var antiScanMattingGroup singleflight.Group

// AdminAntiScanPreview 生成防扫图实时预览，预览产物按精确图片哈希 + 配置 hash 复用缓存。
func (s *sSysPublish) AdminAntiScanPreview(ctx context.Context, in *sysin.AntiScanPreviewInp, upload *ghttp.UploadFile) (res *sysin.AntiScanPreviewModel, err error) {
	totalStartedAt := time.Now()
	defer func() {
		g.Log().Infof(ctx, "防扫图预览完成 stage:total durationMs:%d success:%t", time.Since(totalStartedAt).Milliseconds(), err == nil)
	}()
	stageStartedAt := time.Now()
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	usageOwner := cloudResourceUsageOwner{}
	if needsAntiScanFaceDetection(in) || needsAntiScanMatting(in) {
		account, accountErr := s.currentAccount(ctx)
		if accountErr != nil {
			return nil, accountErr
		}
		usageOwner = cloudResourceUsageOwner{TenantId: account.TenantId, AccountId: account.Id}
		if in.BackgroundReplaceEnabled == 1 {
			if err = s.ensureTenantVipFeature(ctx, account.TenantId, sysin.TenantVipFeatureBackgroundReplace); err != nil {
				return nil, err
			}
		}
	}
	g.Log().Infof(ctx, "防扫图阶段完成 stage:auth durationMs:%d", time.Since(stageStartedAt).Milliseconds())
	stageStartedAt = time.Now()
	imageBytes, originalUrl, err := readAntiScanPreviewImage(ctx, upload, in.UseDefaultImage)
	if err != nil {
		return nil, err
	}
	imageHash, err := antiScanImageHash(imageBytes)
	if err != nil {
		return nil, err
	}
	g.Log().Infof(ctx, "防扫图阶段完成 stage:read_hash durationMs:%d sourceBytes:%d imageHash:%s", time.Since(stageStartedAt).Milliseconds(), len(imageBytes), imageHash)
	stageStartedAt = time.Now()
	cloudConf, err := service.SysConfig().GetCloudResource(ctx)
	if err != nil {
		return nil, err
	}
	configHash := antiScanConfigHash(in, cloudConf)
	g.Log().Infof(ctx, "防扫图阶段完成 stage:config durationMs:%d imageHash:%s", time.Since(stageStartedAt).Milliseconds(), imageHash)
	noop := isAntiScanNoop(in)
	if in.PreviewOnly != 1 {
		if cached, ok := s.getAntiScanPreviewCache(ctx, imageHash, configHash); ok {
			cached.CacheHit = 1
			return cached, nil
		}
	}
	detectRes := &antiScanDetectResult{Provider: "none"}
	warnings := []string{}
	if !noop {
		stageStartedAt = time.Now()
		detectRes, warnings, err = s.detectAntiScanImage(ctx, imageHash, imageBytes, in, cloudConf, usageOwner)
		if err != nil {
			return nil, err
		}
		g.Log().Infof(ctx, "防扫图阶段完成 stage:detect durationMs:%d provider:%s imageHash:%s", time.Since(stageStartedAt).Milliseconds(), detectRes.Provider, imageHash)
	}
	stageStartedAt = time.Now()
	previewBytes, renderWarnings, err := renderAntiScanPreview(ctx, imageBytes, in, detectRes)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, renderWarnings...)
	g.Log().Infof(ctx, "防扫图阶段完成 stage:render durationMs:%d outputBytes:%d imageHash:%s", time.Since(stageStartedAt).Milliseconds(), len(previewBytes), imageHash)
	previewUrl := ""
	if in.PreviewOnly == 1 {
		previewUrl = antiScanPreviewDataURL(previewBytes)
	} else {
		stageStartedAt = time.Now()
		previewUrl, err = uploadAntiScanPreview(ctx, previewBytes, in)
		if err != nil {
			return nil, err
		}
		g.Log().Infof(ctx, "防扫图阶段完成 stage:preview_upload durationMs:%d imageHash:%s", time.Since(stageStartedAt).Milliseconds(), imageHash)
	}
	res = &sysin.AntiScanPreviewModel{
		CacheHit:      0,
		ConfigHash:    configHash,
		FaceCount:     detectRes.FaceCount,
		ImageHash:     imageHash,
		OriginalUrl:   originalUrl,
		PreviewUrl:    previewUrl,
		Provider:      detectRes.Provider,
		Warnings:      warnings,
		CloudRawSaved: detectRes.CloudRawSaved,
	}
	if in.PreviewOnly != 1 {
		if err = s.saveAntiScanPreviewCache(ctx, res, detectRes); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func (s *sSysPublish) AdminAntiScanSegment(ctx context.Context, in *sysin.AntiScanSegmentInp) (res *sysin.AntiScanSegmentModel, err error) {
	totalStartedAt := time.Now()
	defer func() {
		g.Log().Infof(ctx, "人像分割完成 stage:total durationMs:%d success:%t", time.Since(totalStartedAt).Milliseconds(), err == nil)
	}()
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	stageStartedAt := time.Now()
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.ensureTenantVipFeature(ctx, account.TenantId, sysin.TenantVipFeatureBackgroundReplace); err != nil {
		return nil, err
	}
	media, err := s.antiScanSegmentMedia(ctx, in.MediaId, account)
	if err != nil {
		return nil, err
	}
	g.Log().Infof(ctx, "人像分割阶段完成 stage:auth durationMs:%d", time.Since(stageStartedAt).Milliseconds())
	stageStartedAt = time.Now()
	conf, err := service.SysConfig().GetCloudResource(ctx)
	if err != nil {
		return nil, err
	}
	provider := antiScanMattingProvider(conf)
	if cached, ok := s.getAntiScanMediaSegmentCache(ctx, in.MediaId, provider); ok {
		cached.Status = antiScanMattingStatusCompleted
		g.Log().Infof(ctx, "人像分割阶段完成 stage:media_cache_lookup durationMs:%d cacheHit:1 mediaId:%d", time.Since(stageStartedAt).Milliseconds(), in.MediaId)
		return cached, nil
	}
	path, _, err := cachedTelegramMediaFile(ctx, media)
	if err != nil {
		return nil, gerror.Wrap(err, "读取媒体图片失败")
	}
	if strings.TrimSpace(path) == "" {
		return nil, gerror.New("媒体图片地址不存在")
	}
	imageBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, gerror.Wrap(err, "读取媒体图片失败")
	}
	imageHash, err := antiScanImageHash(imageBytes)
	if err != nil {
		return nil, err
	}
	g.Log().Infof(ctx, "人像分割阶段完成 stage:read_hash durationMs:%d sourceBytes:%d imageHash:%s", time.Since(stageStartedAt).Milliseconds(), len(imageBytes), imageHash)
	if state, ok := loadAntiScanMattingTaskState(ctx, in.MediaId, provider); ok && strings.EqualFold(state.ImageHash, imageHash) {
		return state, nil
	}
	stageStartedAt = time.Now()
	g.Log().Infof(ctx, "人像分割阶段完成 stage:config durationMs:%d provider:%s imageHash:%s", time.Since(stageStartedAt).Milliseconds(), provider, imageHash)
	stageStartedAt = time.Now()
	if cached, ok := s.getAntiScanSegmentCache(ctx, imageHash, provider); ok {
		url := antiScanSegmentURL(cached.SegmentRaw)
		if url == "" {
			if legacyBytes := antiScanSegmentImageBytes(cached.SegmentRaw); len(legacyBytes) > 0 {
				url, err = uploadAntiScanSegment(ctx, legacyBytes, imageHash)
				if err != nil {
					return nil, err
				}
				if err = s.saveAntiScanDetectionPart(ctx, imageHash, &antiScanDetectResult{CloudRawSaved: 1, Provider: "fapihub-matting", SegmentRaw: encodeFapiHubSegmentPortraitURL(url)}); err != nil {
					return nil, err
				}
			}
		}
		if url != "" {
			url = antiScanSegmentPresentationURL(url)
			width, height := antiScanImageDimensions(imageBytes)
			g.Log().Infof(ctx, "人像分割阶段完成 stage:cache_lookup durationMs:%d cacheHit:1 imageHash:%s", time.Since(stageStartedAt).Milliseconds(), imageHash)
			res = &sysin.AntiScanSegmentModel{CacheHit: 1, ImageHash: imageHash, SegmentUrl: url, Width: width, Height: height, Status: antiScanMattingStatusCompleted}
			if err = s.saveAntiScanMediaSegmentCache(ctx, in.MediaId, res, cached.SegmentRaw, provider); err != nil {
				return nil, err
			}
			return res, nil
		}
	}
	g.Log().Infof(ctx, "人像分割阶段完成 stage:cache_lookup durationMs:%d cacheHit:0 imageHash:%s", time.Since(stageStartedAt).Milliseconds(), imageHash)
	if err = s.ensureImageQuotaAvailable(ctx, account.TenantId); err != nil {
		return nil, err
	}
	width, height := antiScanImageDimensions(imageBytes)
	res, err = s.enqueueAntiScanMattingTask(ctx, antiScanMattingQueuePayload{
		TenantId: account.TenantId, AccountId: account.Id, MediaId: in.MediaId,
		ImageHash: imageHash, Provider: provider, Width: width, Height: height,
		SourceURL: media.FileUrl, StoragePath: media.StoragePath,
	})
	if err != nil {
		return nil, err
	}
	g.Log().Info(ctx, "人像分割任务已提交", g.Map{"taskId": res.TaskId, "mediaId": in.MediaId, "imageHash": imageHash, "provider": provider})
	return res, nil
}

func (s *sSysPublish) antiScanSegmentMedia(ctx context.Context, mediaId int64, account *sysin.AccountModel) (*telegramMediaItem, error) {
	capability, err := s.activeAccountCapability(ctx, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	mod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,account_id,attachment_id,original_attachment_id,edited_attachment_id,media_type,file_url,original_file_url,edited_file_url,storage_path,original_storage_path,edited_storage_path,edit_status,md5").
		Where("id", mediaId).Where("tenant_id", account.TenantId).WhereNull("deleted_at")
	if capability.AccountType == sysin.PublishAccountTypeUploader && capability.SharedResourceEnabled != 1 {
		mod = mod.Where("account_id", account.Id)
	}
	row, err := mod.One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取媒体信息失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("媒体不存在或无权访问")
	}
	if mediaType := strings.ToLower(strings.TrimSpace(row["media_type"].String())); mediaType != "image" && mediaType != "photo" {
		return nil, gerror.New("仅图片媒体支持人像分割")
	}
	asset := newProfileMediaFromRecord(row).EffectiveAsset()
	return &telegramMediaItem{Id: mediaId, AttachmentId: asset.AttachmentId, MediaType: "image", FileUrl: asset.FileUrl, StoragePath: asset.StoragePath, AssetHash: asset.Hash}, nil
}

func (s *sSysPublish) getAntiScanMediaSegmentCache(ctx context.Context, mediaId int64, provider string) (*sysin.AntiScanSegmentModel, bool) {
	cacheKey := antiScanMediaSegmentCacheKey(mediaId, provider)
	if value, cacheErr := cache.Instance().Get(ctx, cacheKey); cacheErr == nil && !value.IsNil() {
		var cached sysin.AntiScanSegmentModel
		if scanErr := value.Scan(&cached); scanErr == nil && strings.TrimSpace(cached.SegmentUrl) != "" {
			cached.CacheHit = 1
			return &cached, true
		}
	}
	row, err := g.DB().Model(antiScanCacheTable).Safe().Ctx(ctx).
		Where("media_id", mediaId).Where("segment_json <> ''").Where("cloud_raw_saved", 1).
		Where("provider LIKE ?", "%"+antiScanMattingProviderMarker(provider)+"%").
		WhereNull("deleted_at").OrderDesc("id").One()
	if err != nil || row.IsEmpty() {
		return nil, false
	}
	url := antiScanSegmentURL(row["segment_json"].String())
	if url == "" {
		return nil, false
	}
	res := &sysin.AntiScanSegmentModel{CacheHit: 1, ImageHash: row["image_hash"].String(), SegmentUrl: antiScanSegmentPresentationURL(url), Width: row["image_width"].Int(), Height: row["image_height"].Int()}
	if cacheErr := cache.Instance().Set(ctx, cacheKey, res, 24*time.Hour); cacheErr != nil {
		g.Log().Warningf(ctx, "缓存媒体人像分割结果失败 mediaId:%d err:%+v", mediaId, cacheErr)
	}
	return res, true
}

func (s *sSysPublish) getAntiScanMediaMattingCache(ctx context.Context, mediaId int64, imageHash string, provider string) (*antiScanDetectResult, bool) {
	row, err := g.DB().Model(antiScanCacheTable).Safe().Ctx(ctx).
		Where("media_id", mediaId).
		Where("image_hash", imageHash).
		Where("segment_json <> ''").
		Where("cloud_raw_saved", 1).
		Where("provider LIKE ?", "%"+antiScanMattingProviderMarker(provider)+"%").
		WhereNull("deleted_at").OrderDesc("id").One()
	if err != nil || row.IsEmpty() {
		return nil, false
	}
	return &antiScanDetectResult{
		CloudRawSaved: row["cloud_raw_saved"].Int(),
		Provider:      row["provider"].String(),
		SegmentRaw:    row["segment_json"].String(),
	}, true
}

func (s *sSysPublish) saveAntiScanMediaSegmentCache(ctx context.Context, mediaId int64, res *sysin.AntiScanSegmentModel, segmentRaw string, provider string) error {
	_, err := g.DB().Model(antiScanCacheTable).Safe().Ctx(ctx).Data(g.Map{
		"media_id": mediaId, "image_hash": res.ImageHash, "config_hash": "", "provider": antiScanMattingProviderMarker(provider),
		"face_count": 0, "face_json": "", "segment_json": segmentRaw, "original_url": "", "preview_url": "",
		"warnings_json": "[]", "cloud_raw_saved": 1, "image_width": res.Width, "image_height": res.Height,
		"created_at": gtime.Now(), "updated_at": gtime.Now(),
	}).Insert()
	if err != nil {
		return gerror.Wrap(err, "保存媒体人像分割缓存失败")
	}
	if cacheErr := cache.Instance().Set(ctx, antiScanMediaSegmentCacheKey(mediaId, provider), res, 24*time.Hour); cacheErr != nil {
		g.Log().Warningf(ctx, "缓存媒体人像分割结果失败 mediaId:%d err:%+v", mediaId, cacheErr)
	}
	return nil
}

func antiScanMediaSegmentCacheKey(mediaId int64, provider string) string {
	return fmt.Sprintf("%s%d:%s", publishconsts.AntiScanMediaSegmentKeyPrefix, mediaId, provider)
}

func invalidateAntiScanMediaSegmentCache(ctx context.Context, mediaId int64) error {
	for _, provider := range []string{"facepp", "aliyun", "tencent", "fapihub"} {
		if _, err := cache.Instance().Remove(ctx, antiScanMediaSegmentCacheKey(mediaId, provider)); err != nil {
			g.Log().Warningf(ctx, "清除媒体人像分割快速缓存失败 mediaId:%d provider:%s err:%+v", mediaId, provider, err)
		}
		if _, err := cache.Instance().Remove(ctx, antiScanMattingTaskCacheKey(mediaId, provider)); err != nil {
			g.Log().Warningf(ctx, "清除媒体人像分割任务缓存失败 mediaId:%d provider:%s err:%+v", mediaId, provider, err)
		}
	}
	if _, err := g.DB().Model(antiScanCacheTable).Safe().Ctx(ctx).Where("media_id", mediaId).Delete(); err != nil {
		return gerror.Wrap(err, "清除媒体人像分割缓存失败")
	}
	return nil
}

func antiScanPreviewDataURL(imageBytes []byte) string {
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(imageBytes)
}

type antiScanDetectResult struct {
	CloudRawSaved int
	FaceCount     int
	FaceRaw       string
	Provider      string
	SegmentRaw    string
}

// detectAntiScanImage 按精确图片哈希缓存云识别结果，避免相似图误复用抠图结果。
func (s *sSysPublish) detectAntiScanImage(ctx context.Context, imageHash string, imageBytes []byte, in *sysin.AntiScanPreviewInp, conf *model.CloudResourceConfig, usageOwner cloudResourceUsageOwner) (*antiScanDetectResult, []string, error) {
	if conf == nil {
		return nil, nil, gerror.New("云资源配置不合法")
	}
	res := &antiScanDetectResult{Provider: "none"}
	warnings := []string{}
	if needsAntiScanFaceDetection(in) {
		faceRaw, faceCount, err := s.getOrCreateAntiScanFaceDetection(ctx, imageHash, imageBytes, conf, usageOwner)
		if err != nil {
			return nil, nil, err
		}
		res.FaceRaw = faceRaw
		res.FaceCount = faceCount
		if faceRaw != "" {
			res.CloudRawSaved = 1
			res.Provider = appendAntiScanProvider(res.Provider, "tencent-face")
		} else {
			warnings = append(warnings, "腾讯云人脸检测未启用，二维码/贴图只能按默认位置避让")
		}
	}
	if needsAntiScanMatting(in) {
		mattingCtx := ctx
		cancel := func() {}
		if in.PreviewOnly == 1 {
			mattingCtx, cancel = context.WithTimeout(ctx, 8*time.Second)
		}
		segmentRaw, _, err := s.getOrCreateAntiScanMatting(mattingCtx, imageHash, imageBytes, conf, usageOwner)
		cancel()
		if err != nil {
			if in.PreviewOnly == 1 {
				warnings = append(warnings, "云端抠图响应较慢，预览已先使用背景纹理；正式应用时会继续尝试抠图")
				return res, warnings, nil
			}
			return nil, nil, err
		}
		res.SegmentRaw = segmentRaw
		if segmentRaw != "" {
			res.CloudRawSaved = 1
			res.Provider = appendAntiScanProvider(res.Provider, antiScanMattingProvider(conf)+"-matting")
		} else {
			warnings = append(warnings, "云端抠图能力未启用，背景替换将降级为本地纹理处理")
		}
	}
	return res, warnings, nil
}

func needsAntiScanFaceDetection(in *sysin.AntiScanPreviewInp) bool {
	return false
}

func needsAntiScanMatting(in *sysin.AntiScanPreviewInp) bool {
	return in.BackgroundReplaceEnabled == 1
}

func appendAntiScanProvider(current string, next string) string {
	if current == "" || current == "none" {
		return next
	}
	if strings.Contains(current, next) {
		return current
	}
	return current + "+" + next
}

func (s *sSysPublish) getOrCreateAntiScanFaceDetection(ctx context.Context, imageHash string, imageBytes []byte, conf *model.CloudResourceConfig, usageOwner cloudResourceUsageOwner) (string, int, error) {
	if cached, ok := s.getAntiScanFaceCache(ctx, imageHash); ok {
		return cached.FaceRaw, cached.FaceCount, nil
	}
	if conf.TencentVisionEnabled != 1 {
		return "", 0, nil
	}
	normalized, err := normalizeTencentVisionImageBytes(imageBytes)
	if err != nil {
		return "", 0, err
	}
	client := newTencentVisionClient(conf.TencentSecretId, conf.TencentSecretKey, conf.TencentCloudSite, conf.TencentRegion, conf.TencentBdaEndpoint, conf.TencentIaiEndpoint)
	startedAt := time.Now()
	faceRaw, faceCount, err := client.detectFace(ctx, base64.StdEncoding.EncodeToString(normalized))
	recordCloudResourceUsage(ctx, cloudResourceUsageEvent{
		cloudResourceUsageOwner: usageOwner,
		ResourceType:            sysin.CloudResourceTypeFaceDetection,
		Provider:                sysin.CloudResourceProviderTencent,
		Scene:                   cloudResourceUsageScenePreview,
		Success:                 err == nil,
		Duration:                time.Since(startedAt),
	})
	if err != nil {
		return "", 0, err
	}
	if err = s.saveAntiScanDetectionPart(ctx, imageHash, &antiScanDetectResult{
		CloudRawSaved: 1,
		FaceCount:     faceCount,
		FaceRaw:       faceRaw,
		Provider:      "tencent-face",
	}); err != nil {
		return "", 0, err
	}
	return faceRaw, faceCount, nil
}

func (s *sSysPublish) getOrCreateAntiScanMatting(ctx context.Context, imageHash string, imageBytes []byte, conf *model.CloudResourceConfig, usageOwner cloudResourceUsageOwner) (string, bool, error) {
	provider := antiScanMattingProvider(conf)
	if cached, ok := s.getAntiScanSegmentCache(ctx, imageHash, provider); ok {
		return cached.SegmentRaw, false, nil
	}
	key := provider + ":" + imageHash
	requestToken := guid.S()
	value, err, _ := antiScanMattingGroup.Do(key, func() (interface{}, error) {
		if cached, ok := s.getAntiScanSegmentCache(ctx, imageHash, provider); ok {
			return antiScanMattingResult{SegmentRaw: cached.SegmentRaw}, nil
		}
		distributedLock := lock.NewConfig(2*time.Minute, 100*time.Millisecond).Mutex("youban_publish:anti_scan:matting:" + key)
		if lockErr := distributedLock.Lock(ctx); lockErr != nil {
			return nil, gerror.Wrap(lockErr, "等待云端抠图任务失败")
		}
		defer func() { _ = distributedLock.Unlock(context.Background()) }()
		if cached, ok := s.getAntiScanSegmentCache(ctx, imageHash, provider); ok {
			return antiScanMattingResult{SegmentRaw: cached.SegmentRaw}, nil
		}
		segmentRaw, createErr := s.createAntiScanMatting(ctx, imageHash, imageBytes, conf, usageOwner, provider)
		return antiScanMattingResult{SegmentRaw: segmentRaw, CreatorToken: requestToken}, createErr
	})
	if err != nil {
		return "", false, err
	}
	result := value.(antiScanMattingResult)
	return result.SegmentRaw, result.CreatorToken == requestToken, nil
}

type antiScanMattingResult struct {
	SegmentRaw   string
	CreatorToken string
}

func (s *sSysPublish) createAntiScanMatting(ctx context.Context, imageHash string, imageBytes []byte, conf *model.CloudResourceConfig, usageOwner cloudResourceUsageOwner, provider string) (string, error) {
	startedAt := time.Now()
	var segmentURL string
	var err error
	providerName := "aliyun-matting"
	if provider == "facepp" {
		segmentURL, err = facePPPortraitMatting(ctx, imageBytes, imageHash, conf)
		providerName = "facepp-matting"
		if err != nil && antiScanTencentMattingFallbackReady(conf) {
			primaryErr := err
			fallbackStartedAt := time.Now()
			result, fallbackErr := tencentCOSPortraitMatting(ctx, imageBytes, imageHash, conf, false)
			if fallbackErr == nil && result != nil && strings.TrimSpace(result.URL) != "" {
				segmentURL = result.URL
				err = nil
				// provider 列仅允许 32 字符；持久化为所选能力标记，实际降级信息由调用日志与指标记录。
				providerName = "facepp-matting"
				g.Log().Warningf(ctx, "Face++抠图失败，已降级腾讯云 imageHash:%s fallbackDurationMs:%d primaryErr:%+v", imageHash, time.Since(fallbackStartedAt).Milliseconds(), primaryErr)
			} else {
				g.Log().Warningf(ctx, "Face++抠图及腾讯云降级均失败 imageHash:%s primaryErr:%+v fallbackErr:%+v", imageHash, primaryErr, fallbackErr)
			}
		}
	} else if provider == "aliyun" {
		segmentURL, err = aliyunCOSPortraitMatting(ctx, imageBytes, imageHash, conf)
	} else if provider == "tencent" {
		providerName = "tencent-ci-matting"
		var result *tencentPortraitMattingResult
		result, err = tencentCOSPortraitMatting(ctx, imageBytes, imageHash, conf, false)
		if result != nil {
			segmentURL = result.URL
		}
	} else {
		providerName = "fapihub-matting"
		client := newFapiHubClient(conf.FapiHubApiKey, conf.FapiHubEndpoint, conf.FapiHubModel)
		var pngBytes []byte
		pngBytes, err = client.removeBackground(ctx, imageBytes)
		if err == nil {
			uploadStartedAt := time.Now()
			segmentURL, err = uploadAntiScanSegment(ctx, pngBytes, imageHash)
			g.Log().Infof(ctx, "防扫图阶段完成 stage:segment_upload durationMs:%d outputBytes:%d imageHash:%s", time.Since(uploadStartedAt).Milliseconds(), len(pngBytes), imageHash)
		}
	}
	recordCloudResourceUsage(ctx, cloudResourceUsageEvent{
		cloudResourceUsageOwner: usageOwner,
		ResourceType:            sysin.CloudResourceTypeBackgroundMatting,
		Provider:                provider,
		Scene:                   cloudResourceUsageScenePreview,
		Success:                 err == nil,
		Duration:                time.Since(startedAt),
	})
	if err != nil {
		g.Log().Warningf(ctx, "云端抠图调用失败 imageHash:%s err:%+v", imageHash, err)
		return "", antiScanMattingPublicError()
	}
	segmentRaw := encodeAntiScanSegmentPortraitURL(providerName, segmentURL)
	saveStartedAt := time.Now()
	if err = s.saveAntiScanDetectionPart(ctx, imageHash, &antiScanDetectResult{
		CloudRawSaved: 1,
		Provider:      providerName,
		SegmentRaw:    segmentRaw,
	}); err != nil {
		return "", err
	}
	g.Log().Infof(ctx, "防扫图阶段完成 stage:segment_cache_save durationMs:%d imageHash:%s", time.Since(saveStartedAt).Milliseconds(), imageHash)
	return segmentRaw, nil
}

func antiScanTencentMattingFallbackReady(conf *model.CloudResourceConfig) bool {
	return conf != nil &&
		strings.TrimSpace(conf.TencentSecretId) != "" &&
		strings.TrimSpace(conf.TencentSecretKey) != "" &&
		strings.TrimSpace(conf.TencentMattingBucket) != ""
}

func antiScanMattingProvider(conf *model.CloudResourceConfig) string {
	if conf != nil && strings.EqualFold(strings.TrimSpace(conf.MattingProvider), "facepp") {
		return "facepp"
	}
	if conf != nil && strings.EqualFold(strings.TrimSpace(conf.MattingProvider), "aliyun") {
		return "aliyun"
	}
	if conf != nil && strings.EqualFold(strings.TrimSpace(conf.MattingProvider), "tencent") {
		return "tencent"
	}
	return "fapihub"
}

func antiScanMattingProviderMarker(provider string) string {
	switch provider {
	case "facepp":
		return "facepp-matting"
	case "aliyun":
		return "aliyun-matting"
	case "tencent":
		return "tencent-ci-matting"
	default:
		return "fapihub-matting"
	}
}

func antiScanMattingQuotaReference(tenantId int64, provider string, imageHash string, now time.Time) string {
	return fmt.Sprintf("matting:%d:%s:%s:%s", tenantId, provider, imageHash, imageQuotaPeriod(now))
}

func antiScanMattingCacheMatches(cached *antiScanDetectResult, provider string) bool {
	if cached == nil {
		return false
	}
	return strings.Contains(cached.Provider, antiScanMattingProviderMarker(provider))
}

func antiScanMattingPublicError() error {
	return gerror.New(antiScanMattingErrorMessage)
}

func encodeFapiHubSegmentPortrait(imageBytes []byte) string {
	data, _ := json.Marshal(g.Map{
		"Provider": "fapihub",
		"Response": g.Map{
			"ResultImage": base64.StdEncoding.EncodeToString(imageBytes),
		},
	})
	return string(data)
}

func encodeFapiHubSegmentPortraitURL(url string) string {
	return encodeAntiScanSegmentPortraitURL("fapihub-matting", url)
}

func encodeAntiScanSegmentPortraitURL(provider string, url string) string {
	data, _ := json.Marshal(g.Map{"Provider": provider, "Response": g.Map{"ResultImageUrl": url}})
	return string(data)
}

func antiScanSegmentURL(raw string) string {
	var parsed struct {
		Response struct {
			ResultImageURL string `json:"ResultImageUrl"`
		} `json:"Response"`
	}
	if json.Unmarshal([]byte(raw), &parsed) != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Response.ResultImageURL)
}

// antiScanSegmentPresentationURL keeps the cached provider response stable while
// serving our generated segment through the frontend CDN.
func antiScanSegmentPresentationURL(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return raw
	}
	storagePath := strings.TrimLeft(parsed.Path, "/")
	if !strings.Contains(storagePath, "/anti-scan/segment/") {
		return raw
	}
	return normalizeMediaPresentationURL(raw, storagePath)
}

func antiScanSegmentImageBytes(raw string) []byte {
	var parsed struct {
		Response struct {
			ResultImage string `json:"ResultImage"`
			ResultMask  string `json:"ResultMask"`
		} `json:"Response"`
	}
	if json.Unmarshal([]byte(raw), &parsed) != nil {
		return nil
	}
	value := parsed.Response.ResultImage
	if value == "" {
		value = parsed.Response.ResultMask
	}
	data, _ := base64.StdEncoding.DecodeString(value)
	return data
}

func antiScanImageDimensions(imageBytes []byte) (int, int) {
	config, _, err := image.DecodeConfig(bytes.NewReader(imageBytes))
	if err != nil {
		return 0, 0
	}
	return config.Width, config.Height
}

func uploadAntiScanSegment(ctx context.Context, imageBytes []byte, imageHash string) (string, error) {
	fileHeader, err := file.NewMultipartFileHeader("anti-scan-segment-"+imageHash[:12]+".png", imageBytes)
	if err != nil {
		return "", gerror.Wrap(err, "创建人像分割文件失败")
	}
	attachment, err := baseservice.CommonUpload().UploadFile(antiScanSegmentUploadContext(ctx), storager.KindImg, &ghttp.UploadFile{FileHeader: fileHeader})
	if err != nil {
		return "", gerror.Wrap(err, "保存人像分割文件失败")
	}
	return attachment.FileUrl, nil
}

func antiScanSegmentUploadContext(ctx context.Context) context.Context {
	if contexts.GetModule(ctx) != "" {
		return ctx
	}
	return context.WithValue(ctx, consts.ContextHTTPKey, &basemodel.Context{
		Module:    consts.AppAdmin,
		AddonName: "youban_publish",
		User:      contexts.GetUser(ctx),
		Data:      g.Map{},
	})
}

func readAntiScanPreviewImage(ctx context.Context, upload *ghttp.UploadFile, useDefault int) ([]byte, string, error) {
	if upload != nil {
		reader, err := upload.Open()
		if err != nil {
			return nil, "", gerror.Wrap(err, "读取预览图片失败")
		}
		defer reader.Close()
		content := bytes.NewBuffer(nil)
		if _, err = content.ReadFrom(reader); err != nil {
			return nil, "", gerror.Wrap(err, "读取预览图片内容失败")
		}
		return content.Bytes(), "", nil
	}
	if useDefault != 1 {
		return nil, "", gerror.New("请上传预览图片或使用默认图片")
	}
	path := gfile.Join(addons.GetResourcePath(ctx), "addons", global.GetSkeleton().Name, "public", "antiscan", "default-preview.webp")
	if !gfile.Exists(path) {
		return nil, "", gerror.New("默认防扫图预览图片不存在")
	}
	return gfile.GetBytes(path), "/addons/" + global.GetSkeleton().Name + "/antiscan/default-preview.webp", nil
}

func antiScanImageHash(imageBytes []byte) (string, error) {
	if _, _, err := image.Decode(bytes.NewReader(imageBytes)); err != nil {
		return "", gerror.New("图片格式不支持，请上传 JPG、PNG、GIF 或 WEBP")
	}
	sum := sha256.Sum256(imageBytes)
	return hex.EncodeToString(sum[:]), nil
}

func antiScanConfigHash(in *sysin.AntiScanPreviewInp, cloudConf *model.CloudResourceConfig) string {
	cloudData := g.Map{}
	if cloudConf != nil {
		cloudData = g.Map{
			"fapiHubEnabled": cloudConf.FapiHubEnabled,
			"fapiHubModel":   cloudConf.FapiHubModel,
		}
	}
	data, _ := json.Marshal(g.Map{
		"antiScan":      in.AntiScanConfig,
		"cloud":         cloudData,
		"renderVersion": antiScanPreviewRenderVersion,
	})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func uploadAntiScanPreview(ctx context.Context, imageBytes []byte, in *sysin.AntiScanPreviewInp) (string, error) {
	_, format, err := image.DecodeConfig(bytes.NewReader(imageBytes))
	if err != nil {
		return "", gerror.New("图片格式不支持，请上传 JPG、PNG、GIF 或 WEBP")
	}
	imageBytes, format, err = normalizeAntiScanPreviewUploadBytes(imageBytes, format)
	if err != nil {
		return "", err
	}
	fileHeader, err := file.NewMultipartFileHeader("anti-scan-preview."+antiScanImageExt(format), imageBytes)
	if err != nil {
		return "", gerror.Wrap(err, "创建预览图片失败")
	}
	attachment, err := baseservice.CommonUpload().UploadFile(ctx, storager.KindImg, &ghttp.UploadFile{FileHeader: fileHeader})
	if err != nil {
		return "", err
	}
	return attachment.FileUrl, nil
}

func normalizeAntiScanPreviewUploadBytes(imageBytes []byte, format string) ([]byte, string, error) {
	if strings.ToLower(format) != "webp" {
		return imageBytes, format, nil
	}
	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, "", gerror.New("图片格式不支持，请上传 JPG、PNG、GIF 或 WEBP")
	}
	buf := bytes.NewBuffer(nil)
	if err = jpeg.Encode(buf, img, &jpeg.Options{Quality: 92}); err != nil {
		return nil, "", gerror.Wrap(err, "转换预览图片失败")
	}
	return buf.Bytes(), "jpeg", nil
}

func antiScanImageExt(format string) string {
	switch strings.ToLower(format) {
	case "jpeg":
		return "jpg"
	case "png", "gif", "webp":
		return strings.ToLower(format)
	default:
		return "jpg"
	}
}

func (s *sSysPublish) getAntiScanPreviewCache(ctx context.Context, imageHash string, configHash string) (*sysin.AntiScanPreviewModel, bool) {
	row, err := g.DB().Model(antiScanCacheTable).Safe().Ctx(ctx).
		Where("image_hash", imageHash).
		Where("config_hash", configHash).
		Where("preview_url <> ''").
		WhereNull("deleted_at").
		One()
	if err != nil || row.IsEmpty() {
		return nil, false
	}
	warnings := []string{}
	_ = json.Unmarshal([]byte(row["warnings_json"].String()), &warnings)
	return &sysin.AntiScanPreviewModel{
		ConfigHash:  row["config_hash"].String(),
		FaceCount:   row["face_count"].Int(),
		ImageHash:   row["image_hash"].String(),
		OriginalUrl: row["original_url"].String(),
		PreviewUrl:  row["preview_url"].String(),
		Provider:    row["provider"].String(),
		Warnings:    warnings,
	}, true
}

func (s *sSysPublish) getAntiScanFaceCache(ctx context.Context, imageHash string) (*antiScanDetectResult, bool) {
	row, err := g.DB().Model(antiScanCacheTable).Safe().Ctx(ctx).
		Where("image_hash", imageHash).
		Where("face_json <> ''").
		Where("cloud_raw_saved", 1).
		WhereNull("deleted_at").
		OrderDesc("id").
		One()
	if err != nil || row.IsEmpty() {
		return nil, false
	}
	return &antiScanDetectResult{
		CloudRawSaved: row["cloud_raw_saved"].Int(),
		FaceCount:     row["face_count"].Int(),
		FaceRaw:       row["face_json"].String(),
		Provider:      row["provider"].String(),
	}, true
}

func (s *sSysPublish) getAntiScanSegmentCache(ctx context.Context, imageHash string, provider string) (*antiScanDetectResult, bool) {
	rows, err := g.DB().Model(antiScanCacheTable).Safe().Ctx(ctx).
		Where("image_hash", imageHash).
		Where("segment_json <> ''").
		Where("cloud_raw_saved", 1).
		WhereNull("deleted_at").
		OrderDesc("id").
		All()
	if err != nil || rows.IsEmpty() {
		return nil, false
	}
	for _, row := range rows {
		cached := &antiScanDetectResult{
			CloudRawSaved: row["cloud_raw_saved"].Int(),
			Provider:      row["provider"].String(),
			SegmentRaw:    row["segment_json"].String(),
		}
		if antiScanMattingCacheMatches(cached, provider) {
			return cached, true
		}
	}
	return nil, false
}

func (s *sSysPublish) saveAntiScanDetectionPart(ctx context.Context, imageHash string, detect *antiScanDetectResult) error {
	_, err := g.DB().Model(antiScanCacheTable).Safe().Ctx(ctx).Data(g.Map{
		"image_hash":      imageHash,
		"config_hash":     "",
		"provider":        strings.TrimSpace(detect.Provider),
		"face_count":      detect.FaceCount,
		"face_json":       detect.FaceRaw,
		"segment_json":    detect.SegmentRaw,
		"original_url":    "",
		"preview_url":     "",
		"warnings_json":   "[]",
		"cloud_raw_saved": detect.CloudRawSaved,
		"created_at":      gtime.Now(),
		"updated_at":      gtime.Now(),
	}).Insert()
	return err
}

func (s *sSysPublish) saveAntiScanPreviewCache(ctx context.Context, res *sysin.AntiScanPreviewModel, detect *antiScanDetectResult) error {
	warningsJSON, _ := json.Marshal(res.Warnings)
	_, err := g.DB().Model(antiScanCacheTable).Safe().Ctx(ctx).Data(g.Map{
		"image_hash":      res.ImageHash,
		"config_hash":     res.ConfigHash,
		"provider":        strings.TrimSpace(res.Provider),
		"face_count":      res.FaceCount,
		"face_json":       detect.FaceRaw,
		"segment_json":    detect.SegmentRaw,
		"original_url":    res.OriginalUrl,
		"preview_url":     res.PreviewUrl,
		"warnings_json":   string(warningsJSON),
		"cloud_raw_saved": res.CloudRawSaved,
		"created_at":      gtime.Now(),
		"updated_at":      gtime.Now(),
	}).Insert()
	return err
}
