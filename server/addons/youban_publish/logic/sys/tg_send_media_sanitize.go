package sys

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const telegramPhotoMaxUploadBytes int64 = 10 * 1024 * 1024
const telegramPhotoMaxDimensionsSum = 10000
const telegramPhotoMaxDimensionRatio = 20.0

func prepareTelegramMediaUploadFile(ctx context.Context, media *telegramMediaItem, path string, cleanup func()) (string, func(), error) {
	path = strings.TrimSpace(path)
	if path == "" || media == nil {
		return path, cleanup, nil
	}
	if media.AntiScanEnabled && isTelegramImageMedia(media.MediaType) {
		return prepareTelegramAntiScanUploadFile(ctx, media, path, cleanup, "image")
	}
	switch strings.ToLower(strings.TrimSpace(media.MediaType)) {
	case "video":
		return prepareTelegramVideoUploadFile(ctx, path, cleanup)
	default:
		return prepareTelegramPhotoUploadFile(ctx, path, cleanup)
	}
}

func prepareTelegramAntiScanUploadFile(ctx context.Context, media *telegramMediaItem, sourcePath string, cleanup func(), kind string) (string, func(), error) {
	historyKey := telegramAntiScanHistoryKey(media, kind)
	history := loadTelegramAntiScanHashHistory(ctx, historyKey)
	sourceHash, sourceHashErr := telegramAntiScanFileHash(sourcePath)
	bestScore := -1
	bestPath := ""
	var bestCleanup func()
	var bestHash telegramAntiScanHash
	for attempt := 0; attempt < telegramAntiScanCandidateAttempts; attempt++ {
		var protectedPath string
		var err error
		if kind == "thumbnail" {
			protectedPath, err = prepareTelegramAntiScanThumbnailFile(ctx, media, sourcePath, attempt)
		} else {
			protectedPath, err = prepareTelegramAntiScanFile(ctx, media, sourcePath, attempt)
		}
		if err != nil {
			continue
		}
		candidatePath, candidateCleanup, err := prepareTelegramPhotoUploadFile(ctx, protectedPath, nil)
		if err != nil {
			continue
		}
		candidateHash, err := telegramAntiScanFileHash(candidatePath)
		if err != nil {
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
		if cleanup != nil {
			cleanup()
		}
		return "", nil, gerror.New("生成防扫图候选文件失败")
	}
	media.ProtectedHashKey = historyKey
	media.ProtectedPHash = bestHash.PHash
	media.ProtectedDHash = bestHash.DHash
	g.Log().Warningf(ctx, "防扫图候选未完全达到Hash距离要求，使用最优候选 mediaId:%d kind:%s score:%d", media.Id, kind, bestScore)
	return bestPath, chainCleanup(cleanup, bestCleanup), nil
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
	cleanPath, err := stripTelegramVideoMetadata(ctx, path)
	if err != nil {
		g.Log().Warningf(ctx, "清理视频元数据失败，使用原视频发送 path:%s err:%+v", path, err)
		return path, cleanup, nil
	}
	return cleanPath, chainCleanup(cleanup, func() { _ = os.Remove(cleanPath) }), nil
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
