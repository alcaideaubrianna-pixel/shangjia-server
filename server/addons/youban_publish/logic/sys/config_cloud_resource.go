package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func defaultCloudResourceConfig() *model.CloudResourceConfig {
	return &model.CloudResourceConfig{
		TencentVisionEnabled: 0,
		TencentCloudSite:     "mainland",
		TencentRegion:        "ap-guangzhou",
		TencentBdaEndpoint:   "bda.tencentcloudapi.com",
		TencentIaiEndpoint:   "iai.tencentcloudapi.com",
		FapiHubEnabled:       0,
		FapiHubEndpoint:      "https://fapihub.com/v2/rembg/",
		FapiHubModel:         "falcon",
	}
}

func cloudResourceConfigMap(conf *model.CloudResourceConfig) g.Map {
	return g.Map{
		"tencentVisionEnabled": conf.TencentVisionEnabled,
		"tencentCloudSite":     conf.TencentCloudSite,
		"tencentSecretId":      conf.TencentSecretId,
		"tencentSecretKey":     conf.TencentSecretKey,
		"tencentRegion":        conf.TencentRegion,
		"tencentBdaEndpoint":   conf.TencentBdaEndpoint,
		"tencentIaiEndpoint":   conf.TencentIaiEndpoint,
		"fapiHubEnabled":       conf.FapiHubEnabled,
		"fapiHubApiKey":        conf.FapiHubApiKey,
		"fapiHubEndpoint":      conf.FapiHubEndpoint,
		"fapiHubModel":         conf.FapiHubModel,
	}
}

func validateCloudResourceCredential(ctx context.Context, conf *model.CloudResourceConfig) error {
	imageBytes, _, err := readAntiScanPreviewImage(ctx, nil, 1)
	if err != nil {
		return err
	}
	if conf.FapiHubEnabled == 1 {
		if err = validateFapiHubCredential(ctx, conf, imageBytes); err != nil {
			return err
		}
	}
	return nil
}

func validateFapiHubCredential(ctx context.Context, conf *model.CloudResourceConfig, imageBytes []byte) error {
	client := newFapiHubClient(conf.FapiHubApiKey, conf.FapiHubEndpoint, conf.FapiHubModel)
	startedAt := time.Now()
	_, err := client.removeBackground(ctx, imageBytes)
	recordCloudResourceUsage(ctx, cloudResourceUsageEvent{
		ResourceType: sysin.CloudResourceTypeBackgroundMatting,
		Scene:        cloudResourceUsageSceneValidate,
		Success:      err == nil,
		Duration:     time.Since(startedAt),
	})
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 400") {
			return gerror.Wrap(err, "FAPIHub 抠图接口请求校验失败")
		}
		return gerror.Wrap(err, "FAPIHub 抠图密钥或权限校验失败")
	}
	return nil
}
