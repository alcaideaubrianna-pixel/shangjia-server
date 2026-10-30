package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
)

const telegramPhotoMaxUploadBytes int64 = 10 * 1024 * 1024
const telegramPhotoMaxDimensionsSum = 10000
const telegramPhotoMaxDimensionRatio = 20.0

var errTelegramOCRDocument = errors.New("OCR 判定为文档图片")

func (s *sSysPublish) prepareTelegramMediaUploadFile(ctx context.Context, media *telegramMediaItem, path string, cleanup func()) (resultPath string, resultCleanup func(), resultErr error) {
	ctx = telegramPublishMediaTraceContext(ctx, media)
	stage := beginTelegramPublishStage(ctx, "media.upload.prepare", telegramPublishMediaLogFields(media))
	defer stage.Finish(&resultErr)
	path = strings.TrimSpace(path)
	if path == "" || media == nil {
		return path, cleanup, nil
	}
	if media.AntiScanEnabled && isTelegramImageMedia(media.MediaType) {
		return s.prepareTelegramAntiScanUploadFile(ctx, media, path, cleanup, "image")
	}
	switch strings.ToLower(strings.TrimSpace(media.MediaType)) {
	case "video":
		return prepareTelegramVideoUploadFile(ctx, path, cleanup)
	default:
		return prepareTelegramPhotoUploadFile(ctx, path, cleanup)
	}
}

func (s *sSysPublish) prepareTelegramAntiScanUploadFile(ctx context.Context, media *telegramMediaItem, sourcePath string, cleanup func(), kind string) (resultPath string, resultCleanup func(), resultErr error) {
	// Every successful branch (including background replacement and cache reuse)
	// must index the final upload bytes before they can be sent. A failed index
	// write is a preparation failure and uses the existing job retry mechanism.
	defer func() {
		if resultErr != nil || resultPath == "" {
			return
		}
		if err := s.indexTelegramGeneratedMedia(ctx, media, resultPath, kind); err != nil {
			if resultCleanup != nil {
				resultCleanup()
			}
			resultPath, resultCleanup, resultErr = "", nil, err
		}
	}()

	if kind == "image" && media.AntiScanMode == "background_replace" && media.AntiScanBackgroundURL != "" {
		startedAt := time.Now()
		path, generatedCleanup, err := s.prepareTelegramBackgroundReplacement(ctx, media, sourcePath)
		if err == nil && path != "" {
			g.Log().Infof(ctx, "频道防扫图阶段完成 stage:background_replace durationMs:%d jobId:%d mediaId:%d", time.Since(startedAt).Milliseconds(), media.JobId, media.Id)
			return prepareTelegramPhotoUploadFile(ctx, path, chainCleanup(cleanup, generatedCleanup))
		}
		if errors.Is(err, errTelegramOCRDocument) {
			g.Log().Infof(ctx, "频道防扫图跳过 OCR 文档图片 jobId:%d mediaId:%d", media.JobId, media.Id)
			// The original document is intentionally not a processed asset;
			// prevent Telegram file-id writeback from caching it as one.
			media.ProcessedAssetHash = ""
			return prepareTelegramPhotoUploadFile(ctx, sourcePath, cleanup)
		}
		g.Log().Warningf(ctx, "频道背景替换已降级为轻量扰动 jobId:%d mediaId:%d durationMs:%d err:%+v", media.JobId, media.Id, time.Since(startedAt).Milliseconds(), err)
		// The variant hash is reserved for a successfully generated processed
		// asset. Do not persist a background-replacement hash when we fell back
		// to lightweight protection, otherwise later pushes will reuse the
		// fallback file and never retry the configured replacement.
		media.ProcessedAssetHash = ""
	}
	historyKey := telegramAntiScanHistoryKey(media, kind)
	history := loadTelegramAntiScanHashHistory(ctx, historyKey)
	sourceHash, sourceHashErr := telegramAntiScanFileHash(sourcePath)
	bestScore := -1
	bestPath := ""
	var bestCleanup func()
	var bestHash telegramAntiScanHash
	var lastCandidateErr error
	for attempt := 0; attempt < telegramAntiScanCandidateAttempts; attempt++ {
		var protectedPath string
		var err error
		if kind == "thumbnail" {
			protectedPath, err = prepareTelegramAntiScanThumbnailFile(ctx, media, sourcePath, attempt)
		} else {
			protectedPath, err = prepareTelegramAntiScanFile(ctx, media, sourcePath, attempt)
		}
		if err != nil {
			lastCandidateErr = err
			g.Log().Warningf(ctx, "防扫图候选生成失败 jobId:%d mediaId:%d kind:%s attempt:%d/%d err:%+v", media.JobId, media.Id, kind, attempt+1, telegramAntiScanCandidateAttempts, err)
			continue
		}
		candidatePath, candidateCleanup, err := prepareTelegramPhotoUploadFile(ctx, protectedPath, nil)
		if err != nil {
			lastCandidateErr = err
			g.Log().Warningf(ctx, "防扫图候选压缩失败 jobId:%d mediaId:%d kind:%s attempt:%d/%d err:%+v", media.JobId, media.Id, kind, attempt+1, telegramAntiScanCandidateAttempts, err)
			continue
		}
		candidateHash, err := telegramAntiScanFileHash(candidatePath)
		if err != nil {
			lastCandidateErr = err
			g.Log().Warningf(ctx, "防扫图候选计算指纹失败 jobId:%d mediaId:%d kind:%s attempt:%d/%d err:%+v", media.JobId, media.Id, kind, attempt+1, telegramAntiScanCandidateAttempts, err)
			if candidateCleanup != nil {
				candidateCleanup()
			}
			continue
		}
		score, passed := 0, true
		if sourceHashErr == nil {
			score, passed = telegramAntiScanCandidateScore(sourceHash, candidateHash, history)
		}
		if passed {
			if bestCleanup != nil {
				bestCleanup()
			}
			media.ProtectedHashKey = historyKey
			media.ProtectedPHash = candidateHash.PHash
			media.ProtectedDHash = candidateHash.DHash
			return candidatePath, chainCleanup(cleanup, candidateCleanup), nil
		}
		if score > bestScore {
			if bestCleanup != nil {
				bestCleanup()
			}
			bestScore, bestPath, bestCleanup, bestHash = score, candidatePath, candidateCleanup, candidateHash
		} else if candidateCleanup != nil {
			candidateCleanup()
		}
	}
	if bestPath == "" {
		// Anti-scan is an optional transformation. A transient provider,
		// image decoder, or candidate scoring failure must not turn an
		// otherwise usable media item into an Account retry loop. Reuse the
		// canonical source through the normal photo preparation path; this
		// still validates/rewrites oversized or unsupported images before the
		// Telegram upload. If the source itself is unavailable, return that
		// concrete preparation error instead of hiding it behind the
		// anti-scan error.
		g.Log().Warningf(ctx, "防扫图候选生成失败，降级使用原始图片 jobId:%d mediaId:%d kind:%s attempts:%d lastErr:%+v", media.JobId, media.Id, kind, telegramAntiScanCandidateAttempts, lastCandidateErr)
		return prepareTelegramPhotoUploadFile(ctx, sourcePath, cleanup)
	}
	media.ProtectedHashKey = historyKey
	media.ProtectedPHash = bestHash.PHash
	media.ProtectedDHash = bestHash.DHash
	g.Log().Warningf(ctx, "防扫图候选未完全达到Hash距离要求，使用最优候选 mediaId:%d kind:%s score:%d", media.Id, kind, bestScore)
	return bestPath, chainCleanup(cleanup, bestCleanup), nil
}

func (s *sSysPublish) prepareTelegramBackgroundReplacement(ctx context.Context, media *telegramMediaItem, sourcePath string) (string, func(), error) {
	imageBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", nil, gerror.Wrap(err, "读取背景替换源图失败")
	}
	sum := sha256.Sum256(imageBytes)
	imageHash := hex.EncodeToString(sum[:])
	conf, err := service.SysConfig().GetCloudResource(ctx)
	if err != nil {
		return "", nil, err
	}
	// OCR is an optional document guard. When it is disabled, background
	// replacement must continue normally; an unavailable OCR service must not
	// silently downgrade the configured replacement to lightweight protection.
	if conf != nil && conf.OcrEnabled == 1 && strings.TrimSpace(conf.OcrEndpoint) != "" {
		ocrResult, ocrErr := classifyTelegramDocumentByOCR(ctx, imageBytes, conf)
		if ocrErr != nil {
			return "", nil, ocrErr
		}
		if ocrResult.HasText || ocrResult.KeywordHit {
			return "", nil, errTelegramOCRDocument
		}
	}
	provider := antiScanMattingProvider(conf)
	segmentRaw := ""
	created := false
	cacheHit := false
	mediaCacheHit := false
	if cached, ok := s.getAntiScanMediaMattingCache(ctx, media.Id, imageHash, provider); ok {
		segmentRaw = cached.SegmentRaw
		cacheHit = true
		mediaCacheHit = true
	} else {
		_, cacheHit = s.getAntiScanSegmentCache(ctx, imageHash, provider)
		if !cacheHit {
			if err = s.ensureImageQuotaAvailable(ctx, media.TenantId); err != nil {
				return "", nil, err
			}
		}
		segmentRaw, created, err = s.getOrCreateAntiScanMatting(ctx, imageHash, imageBytes, conf, cloudResourceUsageOwner{TenantId: media.TenantId, AccountId: media.AccountId})
		if err != nil {
			return "", nil, err
		}
	}
	if created {
		reference := antiScanMattingQuotaReference(media.TenantId, provider, imageHash, time.Now())
		if err = s.consumeImageQuota(ctx, media.TenantId, media.AccountId, reference, "频道批量替换背景"); err != nil {
			return "", nil, err
		}
	}
	width, height := antiScanImageDimensions(imageBytes)
	segmentURL := antiScanSegmentPresentationURL(antiScanSegmentURL(segmentRaw))
	if segmentURL != "" && !mediaCacheHit {
		res := &sysin.AntiScanSegmentModel{CacheHit: boolToInt(cacheHit || !created), ImageHash: imageHash, SegmentUrl: segmentURL, Width: width, Height: height}
		if err = s.saveAntiScanMediaSegmentCache(ctx, media.Id, res, segmentRaw, provider); err != nil {
			return "", nil, err
		}
	}
	in := &sysin.AntiScanPreviewInp{}
	in.BackgroundReplaceEnabled = 1
	in.BackgroundTextureImage = media.AntiScanBackgroundURL
	in.StickerOpacity = 30
	output, _, err := renderAntiScanPreviewLossless(ctx, imageBytes, in, &antiScanDetectResult{Provider: provider, SegmentRaw: segmentRaw})
	if err != nil {
		return "", nil, err
	}
	out, err := os.CreateTemp("", "ybp-background-replace-*.png")
	if err != nil {
		return "", nil, err
	}
	name := out.Name()
	if _, err = out.Write(output); err != nil {
		_ = out.Close()
		_ = os.Remove(name)
		return "", nil, err
	}
	if err = out.Close(); err != nil {
		_ = os.Remove(name)
		return "", nil, err
	}
	return name, func() { _ = os.Remove(name) }, nil
}

func prepareTelegramPhotoUploadFile(ctx context.Context, path string, cleanup func()) (string, func(), error) {
	info, err := os.Stat(path)
	if err != nil {
		return path, cleanup, err
	}
	if info.Size() <= telegramPhotoMaxUploadBytes {
		file, openErr := os.Open(path)
		if openErr == nil {
			config, _, decodeErr := image.DecodeConfig(file)
			_ = file.Close()
			if decodeErr == nil && telegramPhotoDimensionsValid(config.Width, config.Height) {
				return path, cleanup, nil
			}
		}
	}
	cleanPath, err := compressTelegramPhotoForUpload(ctx, path)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return "", nil, err
	}
	return cleanPath, chainCleanup(cleanup, func() { _ = os.Remove(cleanPath) }), nil
}

func stripTelegramPhotoMetadata(ctx context.Context, path string, ext string, quality int) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", gerror.Wrap(err, "打开图片失败")
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return "", gerror.Wrap(err, "解析图片失败")
	}
	img = normalizeTelegramPhotoDimensions(img)
	out, err := os.CreateTemp("", "ybp-tg-photo-*"+normalizedTelegramPhotoExt(ext))
	if err != nil {
		return "", gerror.Wrap(err, "创建图片临时文件失败")
	}
	defer out.Close()
	if ext == ".png" && quality <= 0 {
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		if err = encoder.Encode(out, img); err != nil {
			_ = os.Remove(out.Name())
			return "", gerror.Wrap(err, "清理图片元数据失败")
		}
		return out.Name(), nil
	}
	if quality <= 0 {
		quality = 95
	}
	if err = jpeg.Encode(out, img, &jpeg.Options{Quality: quality}); err != nil {
		_ = os.Remove(out.Name())
		return "", gerror.Wrap(err, "压缩图片失败")
	}
	return out.Name(), nil
}

func compressTelegramPhotoForUpload(ctx context.Context, path string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".png" {
		if cleanPath, err := stripTelegramPhotoMetadata(ctx, path, ext, 0); err == nil {
			if size, sizeErr := fileSize(cleanPath); sizeErr == nil && size <= telegramPhotoMaxUploadBytes {
				return cleanPath, nil
			}
			_ = os.Remove(cleanPath)
		}
	}
	file, err := os.Open(path)
	if err != nil {
		return "", gerror.Wrap(err, "打开图片失败")
	}
	img, _, err := image.Decode(file)
	_ = file.Close()
	if err != nil {
		return "", gerror.Wrap(err, "解析图片失败")
	}
	img = normalizeTelegramPhotoDimensions(img)
	for _, quality := range []int{95, 92, 90, 88, 85, 82, 80, 76, 72, 68, 64, 60} {
		outPath, err := encodeTelegramJPEG(img, quality)
		if err != nil {
			return "", err
		}
		size, err := fileSize(outPath)
		if err == nil && size <= telegramPhotoMaxUploadBytes {
			return outPath, nil
		}
		_ = os.Remove(outPath)
	}
	current := img
	for scale := 0.92; scale >= 0.50; scale -= 0.08 {
		resized := resizeImage(current, scale)
		if resized == nil {
			break
		}
		for _, quality := range []int{88, 84, 80, 76, 72, 68, 64, 60} {
			outPath, err := encodeTelegramJPEG(resized, quality)
			if err != nil {
				return "", err
			}
			size, err := fileSize(outPath)
			if err == nil && size <= telegramPhotoMaxUploadBytes {
				return outPath, nil
			}
			_ = os.Remove(outPath)
		}
		current = resized
	}
	return "", gerror.New("图片超过 Telegram photo 10MB 限制，压缩后仍无法满足发送要求")
}

func prepareTelegramVideoUploadFile(ctx context.Context, path string, cleanup func()) (string, func(), error) {
	preparedPath := path
	preparedCleanup := cleanup
	if telegramVideoPathNeedsNormalization(path) {
		step := beginTelegramPublishStage(ctx, "media.video.normalize", g.Map{"inputExtension": strings.ToLower(filepath.Ext(path))})
		normalizedPath, err := normalizeTelegramVideo(ctx, path)
		step.End(err)
		if err != nil {
			if cleanup != nil {
				cleanup()
			}
			return "", nil, err
		}
		preparedPath = normalizedPath
		preparedCleanup = chainCleanup(cleanup, func() { _ = os.Remove(normalizedPath) })
	} else {
		step := beginTelegramPublishStage(ctx, "media.video.strip_metadata")
		cleanPath, err := stripTelegramVideoMetadata(ctx, path)
		step.End(err, g.Map{"fallbackToOriginal": err != nil})
		if err != nil {
			g.Log().Warningf(ctx, "清理视频元数据失败，使用原视频发送 path:%s err:%+v", path, err)
		} else {
			preparedPath = cleanPath
			preparedCleanup = chainCleanup(cleanup, func() { _ = os.Remove(cleanPath) })
		}
	}
	info, err := os.Stat(preparedPath)
	if err != nil {
		if preparedCleanup != nil {
			preparedCleanup()
		}
		return "", nil, gerror.Wrap(err, "读取验证视频大小失败")
	}
	if info.Size() <= telegramVideoMaxUploadBytes {
		return preparedPath, preparedCleanup, nil
	}
	step := beginTelegramPublishStage(ctx, "media.video.compress", g.Map{"inputBytes": info.Size(), "limitBytes": telegramVideoMaxUploadBytes})
	compressedPath, err := compressTelegramVideoForUpload(ctx, preparedPath)
	step.End(err)
	if err != nil {
		if preparedCleanup != nil {
			preparedCleanup()
		}
		return "", nil, err
	}
	return compressedPath, chainCleanup(preparedCleanup, func() { _ = os.Remove(compressedPath) }), nil
}

func telegramVideoPathNeedsNormalization(path string) bool {
	return !strings.EqualFold(filepath.Ext(strings.TrimSpace(path)), ".mp4")
}

func normalizeTelegramVideo(ctx context.Context, path string) (string, error) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", gerror.New("ffmpeg 未安装，无法转换Telegram兼容视频")
	}
	return cachedTelegramVideoNormalize(ctx, ffmpegPath, path)
}

func stripTelegramVideoMetadata(ctx context.Context, path string) (string, error) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return "", gerror.New("ffmpeg 未安装，无法清理视频元数据")
	}
	return cachedTelegramVideoSanitize(ctx, ffmpegPath, path)
}

func encodeTelegramJPEG(img image.Image, quality int) (string, error) {
	out, err := os.CreateTemp("", "ybp-tg-photo-*.jpg")
	if err != nil {
		return "", gerror.Wrap(err, "创建图片临时文件失败")
	}
	defer out.Close()
	if err = jpeg.Encode(out, img, &jpeg.Options{Quality: quality}); err != nil {
		_ = os.Remove(out.Name())
		return "", gerror.Wrap(err, "压缩图片失败")
	}
	return out.Name(), nil
}

func resizeImage(img image.Image, scale float64) image.Image {
	bounds := img.Bounds()
	width := int(float64(bounds.Dx()) * scale)
	height := int(float64(bounds.Dy()) * scale)
	if width <= 0 || height <= 0 {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, xdraw.Over, nil)
	return dst
}

func normalizeTelegramPhotoDimensions(img image.Image) image.Image {
	if img == nil {
		return img
	}
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if telegramPhotoDimensionsValid(width, height) {
		return img
	}
	contentWidth, contentHeight, canvasWidth, canvasHeight := telegramPhotoTargetDimensions(width, height)
	if contentWidth <= 0 || contentHeight <= 0 || canvasWidth <= 0 || canvasHeight <= 0 {
		return img
	}
	dst := image.NewRGBA(image.Rect(0, 0, canvasWidth, canvasHeight))
	fillImage(dst, color.White)
	offsetX := (canvasWidth - contentWidth) / 2
	offsetY := (canvasHeight - contentHeight) / 2
	target := image.Rect(offsetX, offsetY, offsetX+contentWidth, offsetY+contentHeight)
	xdraw.CatmullRom.Scale(dst, target, img, bounds, xdraw.Over, nil)
	return dst
}

func telegramPhotoTargetDimensions(width int, height int) (int, int, int, int) {
	contentWidth := width
	contentHeight := height
	canvasWidth := width
	canvasHeight := height
	for i := 0; i < 6; i++ {
		canvasWidth, canvasHeight = telegramPhotoCanvasDimensions(contentWidth, contentHeight)
		if canvasWidth+canvasHeight <= telegramPhotoMaxDimensionsSum {
			return contentWidth, contentHeight, canvasWidth, canvasHeight
		}
		scale := float64(telegramPhotoMaxDimensionsSum) / float64(canvasWidth+canvasHeight)
		contentWidth = maxTelegramPhotoInt(1, int(math.Floor(float64(contentWidth)*scale)))
		contentHeight = maxTelegramPhotoInt(1, int(math.Floor(float64(contentHeight)*scale)))
	}
	canvasWidth, canvasHeight = telegramPhotoCanvasDimensions(contentWidth, contentHeight)
	for canvasWidth+canvasHeight > telegramPhotoMaxDimensionsSum && contentWidth+contentHeight > 2 {
		if contentWidth >= contentHeight && contentWidth > 1 {
			contentWidth--
		} else if contentHeight > 1 {
			contentHeight--
		} else {
			break
		}
		canvasWidth, canvasHeight = telegramPhotoCanvasDimensions(contentWidth, contentHeight)
	}
	return contentWidth, contentHeight, canvasWidth, canvasHeight
}

func telegramPhotoCanvasDimensions(width int, height int) (int, int) {
	canvasWidth := maxTelegramPhotoInt(1, width)
	canvasHeight := maxTelegramPhotoInt(1, height)
	if float64(canvasWidth)/float64(canvasHeight) > telegramPhotoMaxDimensionRatio {
		canvasHeight = int(math.Ceil(float64(canvasWidth) / telegramPhotoMaxDimensionRatio))
	}
	if float64(canvasHeight)/float64(canvasWidth) > telegramPhotoMaxDimensionRatio {
		canvasWidth = int(math.Ceil(float64(canvasHeight) / telegramPhotoMaxDimensionRatio))
	}
	return canvasWidth, canvasHeight
}

func telegramPhotoDimensionsValid(width int, height int) bool {
	if width <= 0 || height <= 0 {
		return false
	}
	if width+height > telegramPhotoMaxDimensionsSum {
		return false
	}
	ratio := float64(width) / float64(height)
	if ratio < 1 {
		ratio = 1 / ratio
	}
	return ratio <= telegramPhotoMaxDimensionRatio
}

func fillImage(img *image.RGBA, c color.Color) {
	if img == nil {
		return
	}
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			img.Set(x, y, c)
		}
	}
}

func maxTelegramPhotoInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func normalizedTelegramPhotoExt(ext string) string {
	if strings.EqualFold(ext, ".png") {
		return ".png"
	}
	return ".jpg"
}

func chainCleanup(first func(), second func()) func() {
	if first == nil && second == nil {
		return nil
	}
	return func() {
		if first != nil {
			first()
		}
		if second != nil {
			second()
		}
	}
}

func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// indexTelegramGeneratedMedia records the final file, after upload resizing or
// JPEG conversion. The stored media owns the search scope, not the publishing
// account: shared-source media can be published by another authorized account.
func (s *sSysPublish) indexTelegramGeneratedMedia(ctx context.Context, media *telegramMediaItem, path, kind string) error {
	if media == nil {
		return gerror.New("生成媒体信息为空")
	}
	hash, err := telegramAntiScanFileHash(path)
	if err != nil {
		return gerror.Wrap(err, "计算生成媒体搜索指纹失败")
	}
	media.ProtectedHashKey = telegramAntiScanHistoryKey(media, kind)
	media.ProtectedPHash, media.ProtectedDHash = hash.PHash, hash.DHash
	// Preview-only inputs have no persisted source media.
	if media.Id <= 0 {
		return nil
	}
	owner, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Fields("tenant_id,account_id,profile_id").Where("id", media.Id).WhereNull("deleted_at").One()
	if err != nil {
		return gerror.Wrap(err, "读取生成媒体归属失败")
	}
	if owner.IsEmpty() || owner["tenant_id"].Int64() <= 0 || owner["account_id"].Int64() <= 0 || owner["profile_id"].Int64() <= 0 {
		return gerror.New("生成媒体原始资料已失效，无法建立搜索索引")
	}
	return s.saveAntiScanSearchFingerprint(ctx, telegramJobRecord{
		TenantId: owner["tenant_id"].Int64(), AccountId: owner["account_id"].Int64(), ProfileId: owner["profile_id"].Int64(),
	}, &telegramSentMessage{MediaId: media.Id, ProtectedHashKey: media.ProtectedHashKey, ProtectedPHash: hash.PHash})
}
