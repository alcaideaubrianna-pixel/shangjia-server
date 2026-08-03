package config

import (
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"
)

type GetReq struct {
	g.Meta `path:"/publish/config/get" method:"get" tags:"上架插件后台" summary:"获取插件配置"`
	sysin.GetConfigInp
}

type GetRes struct {
	*sysin.GetConfigModel
}

type UpdateReq struct {
	g.Meta `path:"/publish/config/update" method:"post" tags:"上架插件后台" summary:"更新插件配置"`
	sysin.UpdateConfigInp
}

type UpdateRes struct{}

type CloudUsageDashboardReq struct {
	g.Meta `path:"/publish/config/cloudUsage/dashboard" method:"get" tags:"上架插件后台" summary:"云资源统计大盘"`
	sysin.CloudResourceUsageDashboardInp
}

type CloudUsageDashboardRes struct {
	*sysin.CloudResourceUsageDashboardModel
}

type CloudUsageListReq struct {
	g.Meta `path:"/publish/config/cloudUsage/list" method:"get" tags:"上架插件后台" summary:"云资源调用明细"`
	sysin.CloudResourceUsageListInp
}

type CloudUsageListRes struct {
	form.PageRes
	List []*sysin.CloudResourceUsageModel `json:"list" dc:"用户调用明细"`
}
