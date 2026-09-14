package sys

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/tencentyun/cos-go-sdk-v5"

	"hotgo/addons/youban_publish/model"
	"hotgo/internal/consts"
	"hotgo/internal/library/storager"
)

type tencentPortraitMattingResult struct {
	URL              string
	ProcessDuration  time.Duration
	DownloadDuration time.Duration
	UploadDuration   time.Duration
	TotalDuration    time.Duration
	OutputBytes      int
	RequestID        string
}

func tencentCOSPortraitMatting(ctx context.Context, imageBytes []byte, imageHash string, conf *model.CloudResourceConfig, requireCleanupPermission bool) (*tencentPortraitMattingResult, error) {
	startedAt := time.Now()
	config := storager.GetConfig()
	if config == nil || config.Drive != consts.UploadDriveCos {
		return nil, gerror.New("腾讯云数据万象人像抠图要求最终存储驱动为COS")
	}
	if strings.TrimSpace(config.CosBucketURL) == "" || strings.TrimSpace(config.CosPath) == "" {
		return nil, gerror.New("腾讯云数据万象人像抠图缺少最终COS存储配置")
	}
	if conf == nil || strings.TrimSpace(conf.TencentSecretId) == "" || strings.TrimSpace(conf.TencentSecretKey) == "" {
		return nil, gerror.New("腾讯云数据万象人像抠图缺少 A 账号 SecretId 或 SecretKey")
	}
	if strings.TrimSpace(conf.TencentMattingBucket) == "" || strings.TrimSpace(conf.TencentRegion) == "" {
		return nil, gerror.New("腾讯云数据万象人像抠图缺少 A 账号处理桶或地域")
	}

	normalized, contentType, ext, err := prepareCloudMattingImageBytes(imageBytes, 1600)
	if err != nil {
		return nil, err
	}
	processingPath := strings.Trim(strings.TrimSpace(conf.TencentMattingPath), "/")
	if processingPath == "" {
		processingPath = "youban-matting"
	}
	sourceKey := processingPath + "/source/" + imageHash + "." + ext
	processingResultKey := processingPath + "/result/" + imageHash + ".png"
	finalPath := strings.Trim(strings.TrimSpace(config.CosPath), "/")
	finalResultKey := finalPath + "/anti-scan/segment/" + imageHash + ".png"
	processingBucketURL, err := url.Parse("https://" + strings.TrimSpace(conf.TencentMattingBucket) + ".cos." + strings.TrimSpace(conf.TencentRegion) + ".myqcloud.com")
	if err != nil {
		return nil, gerror.Wrap(err, "腾讯云 A 账号处理桶配置不正确")
	}
	processingClient := cos.NewClient(&cos.BaseURL{BucketURL: processingBucketURL}, &http.Client{
		Timeout: 15 * time.Second,
		Transport: &cos.AuthorizationTransport{
			SecretID:  strings.TrimSpace(conf.TencentSecretId),
			SecretKey: strings.TrimSpace(conf.TencentSecretKey),
		},
	})
	cleanupPending := true
	defer func() {
		if cleanupPending {
			if cleanupErr := cleanupTencentMattingObjects(ctx, processingClient, sourceKey, processingResultKey); cleanupErr != nil {
				g.Log().Warningf(ctx, "腾讯云 A 账号处理桶临时文件清理失败 err:%+v", cleanupErr)
			}
		}
	}()
	headers := http.Header{}
	headers.Set("Pic-Operations", cos.EncodePicOperations(&cos.PicOperations{
		Rules: []cos.PicOperationsRules{{
			FileId: processingResultKey,
			Rule:   "ci-process=AIPortraitMatting",
		}},
	}))
	processStartedAt := time.Now()
	result, response, err := processingClient.CI.Put(ctx, sourceKey, bytes.NewReader(normalized), &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType:   contentType,
			XOptionHeader: &headers,
		},
	})
	processDuration := time.Since(processStartedAt)
	if err != nil {
		return nil, gerror.Wrap(err, "腾讯云 A 账号处理桶上传或 AIPortraitMatting 权限检测失败")
	}
	if result == nil || len(result.ProcessResults) == 0 || strings.TrimSpace(result.ProcessResults[0].Key) == "" {
		return nil, gerror.New("腾讯云 AIPortraitMatting 未返回结果文件，请检查处理桶是否已绑定数据万象")
	}
	processingResultKey = strings.TrimLeft(strings.TrimSpace(result.ProcessResults[0].Key), "/")
	requestID := ""
	if response != nil && response.Response != nil {
		requestID = response.Header.Get("x-cos-request-id")
	}
	g.Log().Infof(ctx, "防扫图阶段完成 stage:tencent_ci_matting durationMs:%d inputBytes:%d outputBytes:%d requestId:%s imageHash:%s", processDuration.Milliseconds(), len(normalized), result.ProcessResults[0].Size, requestID, imageHash)

	downloadStartedAt := time.Now()
	downloadResponse, err := processingClient.Object.Get(ctx, processingResultKey, nil)
	if err != nil {
		return nil, gerror.Wrap(err, "腾讯云 A 账号处理结果下载权限检测失败")
	}
	resultBytes, readErr := io.ReadAll(io.LimitReader(downloadResponse.Body, 20<<20))
	closeErr := downloadResponse.Body.Close()
	if readErr != nil {
		return nil, gerror.Wrap(readErr, "读取腾讯云 AIPortraitMatting 结果失败")
	}
	if closeErr != nil {
		return nil, gerror.Wrap(closeErr, "关闭腾讯云 AIPortraitMatting 结果响应失败")
	}
	if len(resultBytes) == 0 {
		return nil, gerror.New("腾讯云 AIPortraitMatting 返回了空文件")
	}
	downloadDuration := time.Since(downloadStartedAt)
	g.Log().Infof(ctx, "防扫图阶段完成 stage:tencent_matting_download durationMs:%d outputBytes:%d requestId:%s imageHash:%s", downloadDuration.Milliseconds(), len(resultBytes), requestID, imageHash)

	storageClient, err := storager.NewCosClient()
	if err != nil {
		return nil, gerror.Wrap(err, "初始化 B 账号最终存储客户端失败")
	}
	uploadStartedAt := time.Now()
	_, err = storageClient.Object.Put(ctx, finalResultKey, bytes.NewReader(resultBytes), &cos.ObjectPutOptions{ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
		ContentType:  "image/png",
		CacheControl: "public, max-age=31536000, immutable",
	}})
	if err != nil {
		return nil, gerror.Wrap(err, "B 账号最终 COS 写入权限检测失败")
	}
	uploadDuration := time.Since(uploadStartedAt)
	g.Log().Infof(ctx, "防扫图阶段完成 stage:tencent_result_storage durationMs:%d outputBytes:%d imageHash:%s", uploadDuration.Milliseconds(), len(resultBytes), imageHash)
	if err = cleanupTencentMattingObjects(ctx, processingClient, sourceKey, processingResultKey); err != nil {
		if requireCleanupPermission {
			return nil, gerror.Wrap(err, "腾讯云 A 账号处理桶 DeleteObject 权限检测失败")
		}
		g.Log().Warningf(ctx, "腾讯云 A 账号处理桶临时文件清理失败 err:%+v", err)
	}
	cleanupPending = false

	return &tencentPortraitMattingResult{
		URL:              storager.LastUrl(ctx, finalResultKey, consts.UploadDriveCos),
		ProcessDuration:  processDuration,
		DownloadDuration: downloadDuration,
		UploadDuration:   uploadDuration,
		TotalDuration:    time.Since(startedAt),
		OutputBytes:      len(resultBytes),
		RequestID:        requestID,
	}, nil
}

func cleanupTencentMattingObjects(ctx context.Context, client *cos.Client, keys ...string) error {
	for _, key := range keys {
		if _, err := client.Object.Delete(ctx, key); err != nil {
			return gerror.Wrapf(err, "删除临时文件失败 key:%s", key)
		}
	}
	return nil
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
