package sys

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	imageseg "github.com/alibabacloud-go/imageseg-20191230/v4/client"
	"github.com/alibabacloud-go/tea/dara"
	"github.com/alibabacloud-go/tea/tea"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/tencentyun/cos-go-sdk-v5"

	"hotgo/addons/youban_publish/model"
	"hotgo/internal/consts"
	"hotgo/internal/library/storager"
)

const aliyunSegmentBodyTestImageURL = "http://viapi-test.oss-cn-shanghai.aliyuncs.com/viapi-3.0domepic/imageseg/SegmentBody/SegmentBody1.png"

var (
	aliyunSegmentBodyClients sync.Map
	aliyunResultHTTPClient   = &http.Client{Timeout: 8 * time.Second}
)

type aliyunSegmentBodyResult struct {
	ImageBytes       []byte
	RequestID        string
	ApiDuration      time.Duration
	DownloadDuration time.Duration
	TotalDuration    time.Duration
}

func aliyunCOSPortraitMatting(ctx context.Context, imageBytes []byte, imageHash string, conf *model.CloudResourceConfig) (string, error) {
	config := storager.GetConfig()
	if config == nil || config.Drive != consts.UploadDriveCos {
		return "", gerror.New("阿里云人体分割要求上传驱动为COS")
	}
	preparedAt := time.Now()
	normalized, contentType, ext, err := prepareCloudMattingImageBytes(imageBytes, 1600)
	if err != nil {
		return "", err
	}
	g.Log().Infof(ctx, "防扫图阶段完成 stage:aliyun_source_prepare durationMs:%d inputBytes:%d outputBytes:%d imageHash:%s", time.Since(preparedAt).Milliseconds(), len(imageBytes), len(normalized), imageHash)

	basePath := strings.Trim(strings.TrimSpace(config.CosPath), "/")
	sourceKey := basePath + "/anti-scan/source/" + imageHash + "." + ext
	resultKey := basePath + "/anti-scan/segment/" + imageHash + ".png"
	client, err := storager.NewCosClient()
	if err != nil {
		return "", err
	}
	uploadStartedAt := time.Now()
	_, err = client.Object.Put(ctx, sourceKey, bytes.NewReader(normalized), &cos.ObjectPutOptions{ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
		ContentType:  contentType,
		CacheControl: "public, max-age=31536000, immutable",
	}})
	if err != nil {
		return "", gerror.Wrap(err, "保存阿里云人体分割原图失败")
	}
	g.Log().Infof(ctx, "防扫图阶段完成 stage:aliyun_source_upload durationMs:%d sourceBytes:%d imageHash:%s", time.Since(uploadStartedAt).Milliseconds(), len(normalized), imageHash)

	presignedSourceURL, err := client.Object.GetPresignedURL(ctx, http.MethodGet, sourceKey, config.CosSecretId, config.CosSecretKey, 10*time.Minute, nil)
	if err != nil {
		return "", gerror.Wrap(err, "生成阿里云人体分割原图访问地址失败")
	}
	segmentResult, err := aliyunSegmentBodyFromURL(ctx, presignedSourceURL.String(), conf)
	if err != nil {
		return "", err
	}
	g.Log().Infof(ctx, "防扫图阶段完成 stage:aliyun_segment_body apiDurationMs:%d downloadDurationMs:%d outputBytes:%d requestId:%s imageHash:%s", segmentResult.ApiDuration.Milliseconds(), segmentResult.DownloadDuration.Milliseconds(), len(segmentResult.ImageBytes), segmentResult.RequestID, imageHash)

	resultUploadStartedAt := time.Now()
	_, err = client.Object.Put(ctx, resultKey, bytes.NewReader(segmentResult.ImageBytes), &cos.ObjectPutOptions{ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
		ContentType:  "image/png",
		CacheControl: "public, max-age=31536000, immutable",
	}})
	if err != nil {
		return "", gerror.Wrap(err, "保存阿里云人体分割结果失败")
	}
	g.Log().Infof(ctx, "防扫图阶段完成 stage:aliyun_result_upload durationMs:%d outputBytes:%d imageHash:%s", time.Since(resultUploadStartedAt).Milliseconds(), len(segmentResult.ImageBytes), imageHash)
	return storager.LastUrl(ctx, resultKey, consts.UploadDriveCos), nil
}

func aliyunSegmentBodyFromURL(ctx context.Context, imageURL string, conf *model.CloudResourceConfig) (*aliyunSegmentBodyResult, error) {
	startedAt := time.Now()
	client, err := aliyunSegmentBodyClient(conf)
	if err != nil {
		return nil, err
	}
	apiStartedAt := time.Now()
	response, err := client.SegmentBodyWithContext(ctx, &imageseg.SegmentBodyRequest{ImageURL: tea.String(imageURL)}, &dara.RuntimeOptions{
		ConnectTimeout: dara.Int(3000),
		ReadTimeout:    dara.Int(8000),
		Autoretry:      dara.Bool(false),
	})
	apiDuration := time.Since(apiStartedAt)
	if err != nil {
		return nil, gerror.Wrap(err, "调用阿里云 SegmentBody 失败")
	}
	if response == nil || response.Body == nil || response.Body.Data == nil || strings.TrimSpace(tea.StringValue(response.Body.Data.ImageURL)) == "" {
		return nil, gerror.New("阿里云 SegmentBody 未返回结果图片")
	}

	downloadStartedAt := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, tea.StringValue(response.Body.Data.ImageURL), nil)
	if err != nil {
		return nil, gerror.Wrap(err, "创建阿里云分割结果下载请求失败")
	}
	downloadResponse, err := aliyunResultHTTPClient.Do(request)
	if err != nil {
		return nil, gerror.Wrap(err, "下载阿里云分割结果失败")
	}
	defer downloadResponse.Body.Close()
	if downloadResponse.StatusCode != http.StatusOK {
		return nil, gerror.Newf("下载阿里云分割结果失败，HTTP状态码：%d", downloadResponse.StatusCode)
	}
	resultBytes, err := io.ReadAll(io.LimitReader(downloadResponse.Body, 12*1024*1024+1))
	if err != nil {
		return nil, gerror.Wrap(err, "读取阿里云分割结果失败")
	}
	if len(resultBytes) == 0 || len(resultBytes) > 12*1024*1024 {
		return nil, gerror.New("阿里云分割结果大小不合法")
	}
	if _, _, err = image.DecodeConfig(bytes.NewReader(resultBytes)); err != nil {
		return nil, gerror.Wrap(err, "阿里云分割结果不是有效图片")
	}
	return &aliyunSegmentBodyResult{
		ImageBytes:       resultBytes,
		RequestID:        tea.StringValue(response.Body.RequestId),
		ApiDuration:      apiDuration,
		DownloadDuration: time.Since(downloadStartedAt),
		TotalDuration:    time.Since(startedAt),
	}, nil
}

func aliyunSegmentBodyClient(conf *model.CloudResourceConfig) (*imageseg.Client, error) {
	if conf == nil || strings.TrimSpace(conf.AliyunAccessKeyId) == "" || strings.TrimSpace(conf.AliyunAccessKeySecret) == "" {
		return nil, gerror.New("阿里云 SegmentBody 缺少 AccessKey ID 或 AccessKey Secret")
	}
	endpoint := strings.TrimSpace(conf.AliyunEndpoint)
	if endpoint == "" {
		endpoint = "imageseg.cn-shanghai.aliyuncs.com"
	}
	digest := sha256.Sum256([]byte(conf.AliyunAccessKeyId + "\x00" + conf.AliyunAccessKeySecret + "\x00" + endpoint))
	cacheKey := hex.EncodeToString(digest[:])
	if cached, ok := aliyunSegmentBodyClients.Load(cacheKey); ok {
		return cached.(*imageseg.Client), nil
	}
	client, err := imageseg.NewClient(&openapi.Config{
		AccessKeyId:     tea.String(strings.TrimSpace(conf.AliyunAccessKeyId)),
		AccessKeySecret: tea.String(strings.TrimSpace(conf.AliyunAccessKeySecret)),
		Endpoint:        tea.String(endpoint),
		RegionId:        tea.String("cn-shanghai"),
	})
	if err != nil {
		return nil, gerror.Wrap(err, "初始化阿里云 SegmentBody 客户端失败")
	}
	actual, _ := aliyunSegmentBodyClients.LoadOrStore(cacheKey, client)
	return actual.(*imageseg.Client), nil
}
