package publish

import (
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type AdminCloudResourceConfigViewReq struct {
	g.Meta `path:"/publish/admin/cloudResource/view" method:"get" tags:"上架插件管理端" summary:"查看云资源配置"`
	sysin.CloudResourceConfigViewInp
}

type AdminCloudResourceConfigViewRes struct {
	*sysin.CloudResourceConfigViewModel
}

type AdminCloudResourceConfigSaveReq struct {
	g.Meta `path:"/publish/admin/cloudResource/save" method:"post" tags:"上架插件管理端" summary:"保存云资源配置"`
	sysin.CloudResourceConfigSaveInp
}

type AdminCloudResourceConfigSaveRes struct{}

type AdminAntiScanConfigSaveTabReq struct {
	g.Meta `path:"/publish/admin/antiScan/saveTab" method:"post" tags:"上架插件管理端" summary:"按分栏保存防扫图配置"`
	sysin.AntiScanConfigSaveTabInp
}

type AdminAntiScanConfigSaveTabRes struct{}

type AdminAntiScanPreviewReq struct {
	g.Meta `path:"/publish/admin/antiScan/preview" method:"post" tags:"上架插件管理端" summary:"防扫图实时预览"`
	sysin.AntiScanPreviewInp
}

type AdminAntiScanPreviewRes struct {
	*sysin.AntiScanPreviewModel
}

type AdminAntiScanMaterialListReq struct {
	g.Meta `path:"/publish/admin/antiScan/material/list" method:"get" tags:"上架插件管理端" summary:"防扫图素材列表"`
	sysin.AntiScanMaterialListInp
}

type AdminAntiScanMaterialListRes struct {
	List []*sysin.AntiScanMaterialModel `json:"list" dc:"素材列表"`
}

type AdminAntiScanMaterialUploadReq struct {
	g.Meta `path:"/publish/admin/antiScan/material/upload" method:"post" mime:"multipart/form-data" tags:"上架插件管理端" summary:"上传防扫图素材"`
	sysin.AntiScanMaterialUploadInp
}

type AdminAntiScanMaterialUploadRes struct {
	*sysin.AntiScanMaterialModel
}
