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
	in.TencentRegion = strings.TrimSpace(in.TencentRegion)
	in.TencentBdaEndpoint = strings.TrimSpace(in.TencentBdaEndpoint)
	in.TencentIaiEndpoint = strings.TrimSpace(in.TencentIaiEndpoint)
	if err := checkSwitch(in.TencentVisionEnabled, "腾讯云视觉开关"); err != nil {
		return err
	}
	if in.TencentRegion == "" {
		in.TencentRegion = "ap-guangzhou"
	}
	if in.TencentBdaEndpoint == "" {
		in.TencentBdaEndpoint = "bda.tencentcloudapi.com"
	}
	if in.TencentIaiEndpoint == "" {
		in.TencentIaiEndpoint = "iai.tencentcloudapi.com"
	}
	if in.TencentVisionEnabled == 1 && (in.TencentSecretId == "" || in.TencentSecretKey == "") {
		return gerror.New("启用腾讯云视觉后必须配置 SecretId 和 SecretKey")
	}
	return nil
}
