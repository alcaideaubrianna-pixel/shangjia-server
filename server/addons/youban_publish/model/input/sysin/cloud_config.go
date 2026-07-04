package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_publish/model"
)

type CloudResourceConfigViewInp struct{}

type CloudResourceConfigViewModel struct {
	*model.CloudResourceConfig
}

type CloudResourceConfigSaveInp struct {
	model.CloudResourceConfig
}

func (in *CloudResourceConfigSaveInp) Filter(ctx context.Context) error {
	in.TencentSecretId = strings.TrimSpace(in.TencentSecretId)
	in.TencentSecretKey = strings.TrimSpace(in.TencentSecretKey)
	in.TencentCloudSite = strings.TrimSpace(in.TencentCloudSite)
	in.TencentRegion = strings.TrimSpace(in.TencentRegion)
	in.TencentBdaEndpoint = strings.TrimSpace(in.TencentBdaEndpoint)
	in.TencentIaiEndpoint = strings.TrimSpace(in.TencentIaiEndpoint)
	in.FapiHubApiKey = strings.TrimSpace(in.FapiHubApiKey)
	in.FapiHubEndpoint = strings.TrimSpace(in.FapiHubEndpoint)
	in.FapiHubModel = strings.TrimSpace(in.FapiHubModel)
	if err := checkSwitch(in.TencentVisionEnabled, "腾讯云视觉开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.FapiHubEnabled, "FAPIHub 抠图开关"); err != nil {
		return err
	}
	if in.TencentCloudSite == "" {
		in.TencentCloudSite = "mainland"
	}
	if in.TencentCloudSite != "mainland" && in.TencentCloudSite != "intl" {
		return gerror.New("腾讯云站点不合法")
	}
	if in.TencentCloudSite == "intl" && (in.TencentRegion == "" || in.TencentRegion == "ap-guangzhou") {
		in.TencentRegion = "ap-singapore"
	}
	if in.TencentCloudSite == "mainland" && in.TencentRegion == "" {
		in.TencentRegion = "ap-guangzhou"
	}
	if in.TencentBdaEndpoint == "" {
		in.TencentBdaEndpoint = "bda.tencentcloudapi.com"
	}
	if in.TencentCloudSite == "intl" {
		if in.TencentIaiEndpoint == "" || in.TencentIaiEndpoint == "iai.tencentcloudapi.com" {
			in.TencentIaiEndpoint = "iai.intl.tencentcloudapi.com"
		}
	} else {
		if in.TencentIaiEndpoint == "" || in.TencentIaiEndpoint == "iai.intl.tencentcloudapi.com" {
			in.TencentIaiEndpoint = "iai.tencentcloudapi.com"
		}
	}
	if in.FapiHubEndpoint == "" {
		in.FapiHubEndpoint = "https://fapihub.com/v2/rembg/"
	}
	if in.FapiHubModel == "" {
		in.FapiHubModel = "falcon"
	}
	if in.TencentVisionEnabled == 1 && (in.TencentSecretId == "" || in.TencentSecretKey == "") {
		return gerror.New("启用腾讯云视觉后必须配置 SecretId 和 SecretKey")
	}
	if in.FapiHubEnabled == 1 && in.FapiHubApiKey == "" {
		return gerror.New("启用 FAPIHub 抠图后必须配置 API Key")
	}
	return nil
}
