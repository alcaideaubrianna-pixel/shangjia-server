package config

import (
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
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
