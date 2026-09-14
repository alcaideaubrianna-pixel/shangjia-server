package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model"
)

func defaultCloudResourceConfig() *model.CloudResourceConfig {
	return &model.CloudResourceConfig{
		MattingProvider:      "facepp",
		AliyunEndpoint:       "imageseg.cn-shanghai.aliyuncs.com",
		TencentVisionEnabled: 0,
		TencentCloudSite:     "intl",
		TencentRegion:        "ap-singapore",
		TencentMattingPath:   "youban-matting",
		TencentBdaEndpoint:   "bda.intl.tencentcloudapi.com",
		TencentIaiEndpoint:   "iai.intl.tencentcloudapi.com",
		FapiHubEnabled:       0,
		FapiHubEndpoint:      "https://fapihub.com/v2/rembg/",
		FapiHubModel:         "falcon",
		FacePlusEndpoint:     "https://api-cn.faceplusplus.com/humanbodypp/v2/segment",
		FacePlusConcurrency:  2,
	}
}

func cloudResourceConfigMap(conf *model.CloudResourceConfig) g.Map {
	return g.Map{
		"mattingProvider":       conf.MattingProvider,
		"aliyunAccessKeyId":     conf.AliyunAccessKeyId,
		"aliyunAccessKeySecret": conf.AliyunAccessKeySecret,
		"aliyunEndpoint":        conf.AliyunEndpoint,
		"tencentVisionEnabled":  conf.TencentVisionEnabled,
		"tencentCloudSite":      conf.TencentCloudSite,
		"tencentSecretId":       conf.TencentSecretId,
		"tencentSecretKey":      conf.TencentSecretKey,
		"tencentRegion":         conf.TencentRegion,
		"tencentMattingBucket":  conf.TencentMattingBucket,
		"tencentMattingPath":    conf.TencentMattingPath,
		"tencentBdaEndpoint":    conf.TencentBdaEndpoint,
		"tencentIaiEndpoint":    conf.TencentIaiEndpoint,
		"fapiHubEnabled":        conf.FapiHubEnabled,
		"fapiHubApiKey":         conf.FapiHubApiKey,
		"fapiHubEndpoint":       conf.FapiHubEndpoint,
		"fapiHubModel":          conf.FapiHubModel,
		"facePlusApiKey":        conf.FacePlusApiKey,
		"facePlusApiSecret":     conf.FacePlusApiSecret,
		"facePlusEndpoint":      conf.FacePlusEndpoint,
		"facePlusConcurrency":   conf.FacePlusConcurrency,
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
