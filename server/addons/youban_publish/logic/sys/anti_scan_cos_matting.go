package sys

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/tencentyun/cos-go-sdk-v5"

	"hotgo/internal/consts"
	"hotgo/internal/library/storager"
)

func tencentCOSPortraitMatting(ctx context.Context, imageBytes []byte, imageHash string, secretID string, secretKey string) (string, error) {
	config := storager.GetConfig()
	if config == nil || config.Drive != consts.UploadDriveCos {
		return "", gerror.New("腾讯云数据万象人像抠图要求上传驱动为COS")
	}
	if strings.TrimSpace(config.CosBucketURL) == "" || strings.TrimSpace(config.CosPath) == "" {
		return "", gerror.New("腾讯云数据万象人像抠图缺少COS存储配置")
	}
	if strings.TrimSpace(secretID) == "" || strings.TrimSpace(secretKey) == "" {
		return "", gerror.New("腾讯云数据万象人像抠图缺少 SecretId 或 SecretKey")
	}

	normalized, contentType, ext, err := prepareCloudMattingImageBytes(imageBytes, 1600)
	if err != nil {
		return "", err
	}
	basePath := strings.Trim(strings.TrimSpace(config.CosPath), "/")
	sourceKey := basePath + "/anti-scan/source/" + imageHash + "." + ext
	resultKey := basePath + "/anti-scan/segment/" + imageHash + ".png"
	bucketURL, err := url.Parse(strings.TrimSpace(config.CosBucketURL))
	if err != nil {
		return "", gerror.Wrap(err, "COS Bucket访问域名配置不正确")
	}
	client := cos.NewClient(&cos.BaseURL{BucketURL: bucketURL}, &http.Client{
		Timeout: 8 * time.Second,
		Transport: &cos.AuthorizationTransport{
			SecretID:  strings.TrimSpace(secretID),
			SecretKey: strings.TrimSpace(secretKey),
		},
	})
	headers := http.Header{}
	headers.Set("Pic-Operations", cos.EncodePicOperations(&cos.PicOperations{
		Rules: []cos.PicOperationsRules{{
			FileId: resultKey,
			Rule:   "ci-process=AIPortraitMatting",
		}},
	}))
	startedAt := time.Now()
	result, _, err := client.CI.Put(ctx, sourceKey, bytes.NewReader(normalized), &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType:   contentType,
			CacheControl:  "public, max-age=31536000, immutable",
			XOptionHeader: &headers,
		},
	})
	if err != nil {
		return "", gerror.Wrap(err, "腾讯云数据万象人像抠图失败")
	}
	if result == nil || len(result.ProcessResults) == 0 || strings.TrimSpace(result.ProcessResults[0].Key) == "" {
		return "", gerror.New("腾讯云数据万象人像抠图未返回结果文件")
	}
	resultKey = strings.TrimLeft(strings.TrimSpace(result.ProcessResults[0].Key), "/")
	g.Log().Infof(ctx, "防扫图阶段完成 stage:tencent_ci_matting durationMs:%d inputBytes:%d outputBytes:%d imageHash:%s", time.Since(startedAt).Milliseconds(), len(normalized), result.ProcessResults[0].Size, imageHash)
	return storager.LastUrl(ctx, resultKey, consts.UploadDriveCos), nil
}

func prepareCloudMattingImageBytes(imageBytes []byte, maximumDimension int) ([]byte, string, string, error) {
	img, format, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return nil, "", "", gerror.New("腾讯云人像抠图仅支持 JPG、PNG 或可转换的图片")
	}
	bounds := img.Bounds()
	format = strings.ToLower(format)
	if (format == "jpeg" || format == "png") && bounds.Dx() <= maximumDimension && bounds.Dy() <= maximumDimension {
		if format == "jpeg" {
			return imageBytes, "image/jpeg", "jpg", nil
		}
		return imageBytes, "image/png", "png", nil
	}
	if maximumDimension > 0 && (bounds.Dx() > maximumDimension || bounds.Dy() > maximumDimension) {
		img = resizeAntiScanPreviewImage(img, maximumDimension)
	}
	buffer := bytes.NewBuffer(nil)
	if err = jpeg.Encode(buffer, img, &jpeg.Options{Quality: 88}); err != nil {
		return nil, "", "", gerror.Wrap(err, "转换腾讯云人像抠图图片失败")
	}
	return buffer.Bytes(), "image/jpeg", "jpg", nil
}
