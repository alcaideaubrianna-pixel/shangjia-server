package invite

import (
	"hotgo/addons/youban_invite/model/input/sysin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type ConfigReq struct {
	g.Meta `path:"/invite/config" method:"get" tags:"邀请返现后台" summary:"邀请返现配置"`
}

type ConfigRes struct {
	*sysin.InviteConfigModel
}

type SaveConfigReq struct {
	g.Meta `path:"/invite/saveConfig" method:"post" tags:"邀请返现后台" summary:"保存邀请返现配置"`
	sysin.InviteConfigSaveInp
}

type SaveConfigRes struct{}

type ListReq struct {
	g.Meta `path:"/invite/list" method:"get" tags:"邀请返现后台" summary:"邀请返现记录列表"`
	sysin.InviteRecordListInp
}

type ListRes struct {
	form.PageRes
	List []*sysin.InviteRecordModel `json:"list" dc:"记录列表"`
}

type SaveRecordReq struct {
	g.Meta `path:"/invite/saveRecord" method:"post" tags:"邀请返现后台" summary:"新增或编辑邀请返现记录"`
	sysin.InviteRecordSaveInp
}

type SaveRecordRes struct{}

type DeleteReq struct {
	g.Meta `path:"/invite/delete" method:"post" tags:"邀请返现后台" summary:"删除邀请返现记录"`
	sysin.InviteRecordDeleteInp
}

type DeleteRes struct{}
