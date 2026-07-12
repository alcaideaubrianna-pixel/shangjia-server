package publish

import (
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"
)

type InviteListReq struct {
	g.Meta `path:"/publish/admin/invite/list" method:"get" tags:"上架插件后台" summary:"邀请关系列表"`
	sysin.InviteListInp
}

type InviteListRes struct {
	form.PageRes
	List []*sysin.InviteModel `json:"list" dc:"邀请关系列表"`
}
