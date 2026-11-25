package sys

import (
	"context"

	aidge "github.com/alibabacloud-go/aidge-20260428/v6/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
	credential "github.com/aliyun/credentials-go/credentials"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model"
)

const cloudResourceConfigCacheKey = "youban_publish:config:cloud_resource"
const cloudResourceConfigCacheTTL = 10 * time.Minute

func defaultCloudResourceConfig() *model.CloudResourceConfig {
	return &model.CloudResourceConfig{
		OcrEnabled:              0,
		OcrTimeoutMs:            800,
		MattingProvider:         "facepp",
		AliyunEndpoint:          "imageseg.cn-shanghai.aliyuncs.com",
		AliyunEcommerceEndpoint: "aidge.ap-southeast-1.aliyuncs.com",
		TencentVisionEnabled:    0,
		TencentCloudSite:        "intl",
		TencentRegion:           "ap-singapore",
		TencentMattingPath:      "youban-matting",
		TencentBdaEndpoint:      "bda.intl.tencentcloudapi.com",
		TencentIaiEndpoint:      "iai.intl.tencentcloudapi.com",
		FapiHubEnabled:          0,
		FapiHubEndpoint:         "https://fapihub.com/v2/rembg/",
		FapiHubModel:            "falcon",
		FacePlusEndpoint:        "https://api-cn.faceplusplus.com/humanbodypp/v2/segment",
		FacePlusConcurrency:     2,
	}
}

func cloudResourceConfigMap(conf *model.CloudResourceConfig) g.Map {
	return g.Map{
		"ocrEnabled":              conf.OcrEnabled,
		"ocrEndpoint":             conf.OcrEndpoint,
		"ocrTimeoutMs":            conf.OcrTimeoutMs,
		"ocrBlockedKeywords":      conf.OcrBlockedKeywords,
		"mattingProvider":         conf.MattingProvider,
		"aliyunAccessKeyId":       conf.AliyunAccessKeyId,
		"aliyunAccessKeySecret":   conf.AliyunAccessKeySecret,
		"aliyunEndpoint":          conf.AliyunEndpoint,
		"aliyunEcommerceEndpoint": conf.AliyunEcommerceEndpoint,
		"aliyunEcommerceDomain":   conf.AliyunEcommerceDomain,
		"tencentVisionEnabled":    conf.TencentVisionEnabled,
		"tencentCloudSite":        conf.TencentCloudSite,
		"tencentSecretId":         conf.TencentSecretId,
		"tencentSecretKey":        conf.TencentSecretKey,
		"tencentRegion":           conf.TencentRegion,
		"tencentMattingBucket":    conf.TencentMattingBucket,
		"tencentMattingPath":      conf.TencentMattingPath,
		"tencentBdaEndpoint":      conf.TencentBdaEndpoint,
		"tencentIaiEndpoint":      conf.TencentIaiEndpoint,
		"fapiHubEnabled":          conf.FapiHubEnabled,
		"fapiHubApiKey":           conf.FapiHubApiKey,
		"fapiHubEndpoint":         conf.FapiHubEndpoint,
		"fapiHubModel":            conf.FapiHubModel,
		"facePlusApiKey":          conf.FacePlusApiKey,
		"facePlusApiSecret":       conf.FacePlusApiSecret,
		"facePlusEndpoint":        conf.FacePlusEndpoint,
		"facePlusConcurrency":     conf.FacePlusConcurrency,
	}
}

func validateCloudResourceCredential(ctx context.Context, conf *model.CloudResourceConfig) error {
	if conf.MattingProvider == "aliyun" {
		_, err := aliyunSegmentBodyFromURL(ctx, aliyunSegmentBodyTestImageURL, conf)
		if err != nil {
			return gerror.Wrap(err, "阿里云 SegmentBody 配置校验失败")
		}
		return nil
	}
	if conf.MattingProvider == "aliyun-aidge" {
		cred, err := credential.NewCredential(&credential.Config{AccessKeyId: tea.String(conf.AliyunAccessKeyId), AccessKeySecret: tea.String(conf.AliyunAccessKeySecret)})
		if err != nil {
			return gerror.Wrap(err, "初始化阿里云电商抠图凭证失败")
		}
		endpoint := conf.AliyunEcommerceEndpoint
		if endpoint == "" {
			endpoint = "aidge.ap-southeast-1.aliyuncs.com"
		}
		client, err := aidge.NewClient(&openapi.Config{Credential: cred, Endpoint: tea.String(endpoint)})
		if err != nil {
			return gerror.Wrap(err, "初始化阿里云电商抠图客户端失败")
		}
		response, err := client.ImageMattingWithOptions(&aidge.ImageMattingRequest{ImageUrl: tea.String(aliyunSegmentBodyTestImageURL), BackGroundType: tea.String("TRANSPARENT")}, &util.RuntimeOptions{})
		if err != nil {
			return gerror.Wrap(err, "阿里云电商 ImageMatting 配置校验失败")
		}
		if response == nil || response.Body == nil || response.Body.Data == nil {
			return gerror.New("阿里云电商 ImageMatting 未返回结果")
		}
		return nil
	}
	if conf.MattingProvider == "facepp" {
		imageBytes, _, readErr := readAntiScanPreviewImage(ctx, nil, 1)
		if readErr != nil {
			return readErr
		}
		imageHash, hashErr := antiScanImageHash(imageBytes)
		if hashErr != nil {
			return hashErr
		}
		if _, callErr := facePPPortraitMatting(ctx, imageBytes, "config-"+imageHash[:16], conf); callErr != nil {
			return gerror.Wrap(callErr, "Face++ 人体抠图配置校验失败")
		}
		return nil
	}
	imageBytes, _, err := readAntiScanPreviewImage(ctx, nil, 1)
	if err != nil {
		return err
	}
	if conf.MattingProvider == "tencent" {
		imageHash, hashErr := antiScanImageHash(imageBytes)
		if hashErr != nil {
			return hashErr
		}
		_, err = tencentCOSPortraitMatting(ctx, imageBytes, "config-"+imageHash[:16], conf, true)
		if err != nil {
			return gerror.Wrap(err, "腾讯云国际版数据万象人像抠图配置校验失败")
		}
	} else if conf.FapiHubEnabled == 1 {
		if err = validateFapiHubCredential(ctx, conf, imageBytes); err != nil {
			return err
		}
	}
	return nil
}

func validateFapiHubCredential(ctx context.Context, conf *model.CloudResourceConfig, imageBytes []byte) error {
	client := newFapiHubClient(conf.FapiHubApiKey, conf.FapiHubEndpoint, conf.FapiHubModel)
	_, err := client.removeBackground(ctx, imageBytes)
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 400") {
			return gerror.Wrap(err, "FAPIHub 抠图接口请求校验失败")
		}
		return gerror.Wrap(err, "FAPIHub 抠图密钥或权限校验失败")
	}
	return nil
}
