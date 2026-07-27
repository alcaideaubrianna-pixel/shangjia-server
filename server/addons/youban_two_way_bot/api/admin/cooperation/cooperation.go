package cooperation

import (
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_two_way_bot/model/input/sysin"
	"hotgo/internal/model/input/form"
)

type ConfigViewReq struct {
	g.Meta `path:"/cooperation/config/view" method:"get" tags:"平台合作"`
}
type ConfigViewRes struct{ *sysin.CooperationConfigModel }
type ConfigSaveReq struct {
	g.Meta `path:"/cooperation/config/save" method:"post" tags:"平台合作"`
	sysin.CooperationConfigSaveInp
}
type ConfigSaveRes struct{ *sysin.CooperationConfigModel }
type ApplicationListReq struct {
	g.Meta `path:"/cooperation/application/list" method:"get" tags:"平台合作"`
	sysin.CooperationApplicationListInp
}
type ApplicationListRes struct {
	form.PageRes
	List []*sysin.CooperationApplicationModel `json:"list"`
}
type ApplicationApproveReq struct {
	g.Meta `path:"/cooperation/application/approve" method:"post" tags:"平台合作"`
	sysin.CooperationApplicationActionInp
}
type ApplicationApproveRes struct{}
type ApplicationRejectReq struct {
	g.Meta `path:"/cooperation/application/reject" method:"post" tags:"平台合作"`
	sysin.CooperationApplicationActionInp
}
type ApplicationRejectRes struct{}
type ApplicationCancelReq struct {
	g.Meta `path:"/cooperation/application/cancel" method:"post" tags:"平台合作"`
	sysin.CooperationApplicationActionInp
}
type ApplicationCancelRes struct{}
type ApplicationTerminateReq struct {
	g.Meta `path:"/cooperation/application/terminate" method:"post" tags:"平台合作"`
	sysin.CooperationApplicationActionInp
}
type ApplicationTerminateRes struct{}
type ApplicationRetryReq struct {
	g.Meta `path:"/cooperation/application/retry" method:"post" tags:"平台合作"`
	sysin.CooperationApplicationActionInp
}
type ApplicationRetryRes struct{}
type ApplicationBlacklistReq struct {
	g.Meta `path:"/cooperation/application/blacklist" method:"post" tags:"平台合作"`
	sysin.CooperationApplicationActionInp
}
type ApplicationBlacklistRes struct{}
type ApplicationUnblacklistReq struct {
	g.Meta `path:"/cooperation/application/unblacklist" method:"post" tags:"平台合作"`
	sysin.CooperationApplicationActionInp
}
type ApplicationUnblacklistRes struct{}
