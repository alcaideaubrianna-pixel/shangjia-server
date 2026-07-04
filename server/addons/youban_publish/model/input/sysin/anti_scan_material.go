package sysin

import (
	"context"
	"strings"
)

type AntiScanMaterialListInp struct {
	Type string `json:"type" dc:"素材类型：qr/sticker/background"`
}

type AntiScanMaterialUploadInp struct {
	Type string `json:"type" dc:"素材类型：qr/sticker/background"`
	Name string `json:"name" dc:"素材名称"`
}

type AntiScanMaterialModel struct {
	Id        int64  `json:"id"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	Url       string `json:"url"`
	CreatedAt string `json:"createdAt"`
}

func (in *AntiScanMaterialListInp) Filter(ctx context.Context) error {
	in.Type = normalizeAntiScanMaterialType(in.Type)
	return nil
}

func (in *AntiScanMaterialUploadInp) Filter(ctx context.Context) error {
	in.Type = normalizeAntiScanMaterialType(in.Type)
	return nil
}

func normalizeAntiScanMaterialType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "qr", "sticker", "background":
		return value
	default:
		return "sticker"
	}
}
