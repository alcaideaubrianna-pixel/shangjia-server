package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/frame/g"
)

type AdminAccountOptionsReq struct {
	g.Meta `path:"/publish/admin/account/options" method:"get" tags:"上架插件管理端" summary:"账号筛选选项"`
	sysin.AccountOptionsInp
}

type AdminAccountOptionsRes struct {
	List []*sysin.AccountOptionModel `json:"list" dc:"账号筛选选项"`
}

type AdminCollectSourceOptionsReq struct {
	g.Meta `path:"/publish/admin/collect/source/options" method:"get" tags:"上架插件管理端" summary:"采集源筛选选项"`
}

type AdminCollectSourceOptionsRes struct {
	List []*sysin.CollectSourceOptionModel `json:"list" dc:"采集源筛选选项"`
}
