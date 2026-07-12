package publish

import (
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"
)

type InviteInfoReq struct {
	g.Meta `path:"/publish/invite/info" method:"get" tags:"上架插件" summary:"我的邀请码信息"`
}

type InviteInfoRes struct {
	*sysin.InviteInfoModel
}

type InviteListReq struct {
	g.Meta `path:"/publish/invite/list" method:"get" tags:"上架插件" summary:"我的邀请码列表"`
	sysin.InviteListInp
}

type InviteListRes struct {
	form.PageRes
	List []*sysin.InviteModel `json:"list" dc:"邀请码列表"`
}

type InviteGenerateReq struct {
	g.Meta `path:"/publish/invite/generate" method:"post" tags:"上架插件" summary:"生成邀请码"`
	sysin.InviteCreateInp
}

type InviteGenerateRes struct {
	*sysin.InviteCreateModel
}
