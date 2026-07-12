package bot

import (
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_bot/model/input/sysin"
	"hotgo/internal/model/input/form"
)

type InviteInfoReq struct {
	g.Meta `path:"/bot/invite/info" method:"get" tags:"全局机器人" summary:"我的邀请码信息"`
}

type InviteInfoRes struct {
	*sysin.InviteInfoModel
}

type InviteListReq struct {
	g.Meta `path:"/bot/invite/list" method:"get" tags:"全局机器人" summary:"我的邀请码列表"`
	sysin.InviteListInp
}

type InviteListRes struct {
	form.PageRes
	List []*sysin.InviteModel `json:"list" dc:"邀请码列表"`
}

type InviteGenerateReq struct {
	g.Meta `path:"/bot/invite/generate" method:"post" tags:"全局机器人" summary:"生成邀请码"`
	sysin.InviteCreateInp
}

type InviteGenerateRes struct {
	*sysin.InviteCreateModel
}
