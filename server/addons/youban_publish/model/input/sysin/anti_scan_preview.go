package sysin

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_publish/model"
)

type AntiScanPreviewInp struct {
	PreviewOnly     int `json:"previewOnly"`
	UseDefaultImage int `json:"useDefaultImage"`
	model.AntiScanConfig
}

type AntiScanPreviewModel struct {
	CacheHit      int      `json:"cacheHit"`
	ConfigHash    string   `json:"configHash"`
	FaceCount     int      `json:"faceCount"`
	ImageHash     string   `json:"imageHash"`
	OriginalUrl   string   `json:"originalUrl"`
	PreviewUrl    string   `json:"previewUrl"`
	Provider      string   `json:"provider"`
	Warnings      []string `json:"warnings"`
	CloudRawSaved int      `json:"cloudRawSaved"`
}

func (in *AntiScanPreviewInp) Filter(ctx context.Context) error {
	if in == nil {
		return gerror.New("防扫图预览参数不能为空")
	}
	save := &AntiScanConfigSaveInp{AntiScanConfig: in.AntiScanConfig}
	if err := save.Filter(ctx); err != nil {
		return err
	}
	in.AntiScanConfig = save.AntiScanConfig
	if in.PreviewOnly != 0 {
		in.PreviewOnly = 1
	}
	in.MaskMode = strings.ToLower(strings.TrimSpace(in.MaskMode))
	if in.UseDefaultImage != 0 {
		in.UseDefaultImage = 1
	}
	return nil
}
