package sys

import (
	"bytes"
	"context"
	"fmt"
	"image"
	stddraw "image/draw"
	"image/jpeg"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/internal/library/storager"
	baseservice "hotgo/internal/service"
	"hotgo/utility/file"
)

const (
	inlinePromotionImageMaxDownloadBytes = 20 * 1024 * 1024
	inlinePromotionImageMaxTelegramBytes = 5 * 1024 * 1024
	inlinePromotionImageMinShortSide     = 360
	inlinePromotionImageMinLongSide      = 640
	inlinePromotionImageMaxLongSide      = 2560
	inlinePromotionThumbnailSize         = 640
)

type inlinePromotionImageAssets struct {
	MainImage      []byte
	ThumbnailImage []byte
	Width          int
	Height         int
}

func (s *sSysBot) normalizeInlinePromotionImageConfig(ctx context.Context, config map[string]interface{}) error {
	imageURL := inlinePromotionConfigString(config, "imageUrl")
	if imageURL == "" {
		clearInlinePromotionImageMetadata(config)
		return nil
	}
	if imageURL == inlinePromotionConfigString(config, "imagePreparedSource") &&
		inlinePromotionConfigString(config, "imageThumbnailUrl") != "" &&
		inlinePromotionConfigInt(config, "imageWidth") > 0 &&
		inlinePromotionConfigInt(config, "imageHeight") > 0 {
		return nil
	}
	absoluteURL := normalizePreviewMediaURL(s.absoluteMediaURL(ctx, imageURL))
	if !strings.HasPrefix(absoluteURL, "http://") && !strings.HasPrefix(absoluteURL, "https://") {
		return gerror.New("宣传图片必须是系统上传地址或有效的 HTTP/HTTPS 外链")
	}
	data, err := downloadInlinePromotionImage(ctx, absoluteURL)
	if err != nil {
		return err
	}
	assets, err := buildInlinePromotionImageAssets(data)
	if err != nil {
		return err
	}
	finalImageURL := imageURL
	if len(assets.MainImage) > 0 {
		finalImageURL, err = uploadInlinePromotionImage(ctx, "inline-promotion.jpg", assets.MainImage)
		if err != nil {
			return gerror.Wrap(err, "上传优化后的宣传图片失败")
		}
	}
	thumbnailURL, err := uploadInlinePromotionImage(ctx, "inline-promotion-thumbnail.jpg", assets.ThumbnailImage)
	if err != nil {
		return gerror.Wrap(err, "上传宣传图片缩略图失败")
	}
	config["imageUrl"] = finalImageURL
	config["imageThumbnailUrl"] = thumbnailURL
	config["imageWidth"] = assets.Width
	config["imageHeight"] = assets.Height
	config["imagePreparedSource"] = finalImageURL
	return nil
}

func downloadInlinePromotionImage(ctx context.Context, imageURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, gerror.Wrap(err, "创建宣传图片下载请求失败")
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, gerror.Wrap(err, "下载宣传图片失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, gerror.Newf("下载宣传图片失败：HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, inlinePromotionImageMaxDownloadBytes+1))
	if err != nil {
		return nil, gerror.Wrap(err, "读取宣传图片失败")
	}
	if len(data) == 0 {
		return nil, gerror.New("宣传图片内容为空")
	}
	if len(data) > inlinePromotionImageMaxDownloadBytes {
		return nil, gerror.New("宣传图片原文件不能超过 20 MB")
	}
	return data, nil
}

func buildInlinePromotionImageAssets(data []byte) (*inlinePromotionImageAssets, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, gerror.New("无法读取宣传图片，请上传有效的 JPEG 图片")
	}
	if strings.ToLower(format) != "jpeg" {
		return nil, gerror.New("Telegram Inline 宣传图片仅支持 JPEG 格式")
	}
	shortSide := min(config.Width, config.Height)
	longSide := max(config.Width, config.Height)
	if shortSide < inlinePromotionImageMinShortSide || longSide < inlinePromotionImageMinLongSide {
		return nil, gerror.Newf("宣传图片尺寸不能小于 %d×%d 像素", inlinePromotionImageMinLongSide, inlinePromotionImageMinShortSide)
	}
	if shortSide == 0 || float64(longSide)/float64(shortSide) > 20 {
		return nil, gerror.New("宣传图片长宽比不能超过 20:1")
	}
	source, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, gerror.New("宣传图片 JPEG 数据无效")
	}
	mainImage := source
	mainChanged := longSide > inlinePromotionImageMaxLongSide || len(data) > inlinePromotionImageMaxTelegramBytes
	if longSide > inlinePromotionImageMaxLongSide {
		mainImage = resizeInlinePromotionImage(source, inlinePromotionImageMaxLongSide)
	}
	mainBounds := mainImage.Bounds()
	assets := &inlinePromotionImageAssets{Width: mainBounds.Dx(), Height: mainBounds.Dy()}
	if mainChanged {
		assets.MainImage, err = encodeInlinePromotionJPEG(mainImage, inlinePromotionImageMaxTelegramBytes)
		if err != nil {
			return nil, err
		}
	}
	thumbnail := squareInlinePromotionThumbnail(mainImage, inlinePromotionThumbnailSize)
	assets.ThumbnailImage, err = encodeInlinePromotionJPEG(thumbnail, inlinePromotionImageMaxTelegramBytes)
	if err != nil {
		return nil, err
	}
	return assets, nil
}

func resizeInlinePromotionImage(source image.Image, maximumLongSide int) image.Image {
	bounds := source.Bounds()
	longSide := max(bounds.Dx(), bounds.Dy())
	if longSide <= maximumLongSide {
		return source
	}
	scale := float64(maximumLongSide) / float64(longSide)
	width := max(1, int(math.Round(float64(bounds.Dx())*scale)))
	height := max(1, int(math.Round(float64(bounds.Dy())*scale)))
	destination := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(destination, destination.Bounds(), source, bounds, stddraw.Src, nil)
	return destination
}

func squareInlinePromotionThumbnail(source image.Image, size int) image.Image {
	bounds := source.Bounds()
	side := min(bounds.Dx(), bounds.Dy())
	left := bounds.Min.X + (bounds.Dx()-side)/2
	top := bounds.Min.Y + (bounds.Dy()-side)/2
	crop := image.Rect(left, top, left+side, top+side)
	destination := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(destination, destination.Bounds(), source, crop, stddraw.Src, nil)
	return destination
}

func encodeInlinePromotionJPEG(source image.Image, maximumBytes int) ([]byte, error) {
	for _, quality := range []int{88, 84, 80, 76, 72, 68} {
		buffer := bytes.NewBuffer(nil)
		if err := jpeg.Encode(buffer, source, &jpeg.Options{Quality: quality}); err != nil {
			return nil, gerror.Wrap(err, "编码宣传图片失败")
		}
		if buffer.Len() <= maximumBytes {
			return buffer.Bytes(), nil
		}
	}
	return nil, gerror.New("宣传图片压缩后仍超过 5 MB，请降低图片复杂度后重试")
}

func uploadInlinePromotionImage(ctx context.Context, filename string, data []byte) (string, error) {
	fileHeader, err := file.NewMultipartFileHeader(filename, data)
	if err != nil {
		return "", gerror.Wrap(err, "创建宣传图片上传文件失败")
	}
	attachment, err := baseservice.CommonUpload().UploadFile(ctx, storager.KindImg, &ghttp.UploadFile{FileHeader: fileHeader})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(attachment.FileUrl), nil
}

func inlinePromotionConfigString(config map[string]interface{}, field string) string {
	value, ok := config[field]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func inlinePromotionConfigInt(config map[string]interface{}, field string) int {
	var value int
	_, _ = fmt.Sscan(inlinePromotionConfigString(config, field), &value)
	return value
}

func clearInlinePromotionImageMetadata(config map[string]interface{}) {
	delete(config, "imageThumbnailUrl")
	delete(config, "imageWidth")
	delete(config, "imageHeight")
	delete(config, "imagePreparedSource")
}
