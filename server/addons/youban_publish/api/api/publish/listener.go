package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type AdminListenerPlanListReq struct {
	g.Meta `path:"/publish/admin/listen/list" method:"get" tags:"上架插件管理端" summary:"监听计划列表"`
	sysin.ListenerPlanListInp
}

type AdminListenerPlanListRes struct {
	form.PageRes
	List []*sysin.ListenerPlanModel `json:"list" dc:"监听计划列表"`
}

type AdminListenerPlanSaveReq struct {
	g.Meta `path:"/publish/admin/listen/save" method:"post" tags:"上架插件管理端" summary:"新增或编辑监听计划"`
	sysin.ListenerPlanSaveInp
}

type AdminListenerPlanSaveRes struct {
	Id int64 `json:"id" dc:"监听计划ID"`
}

type AdminListenerPlanDeleteReq struct {
	g.Meta `path:"/publish/admin/listen/delete" method:"post" tags:"上架插件管理端" summary:"删除监听计划"`
	sysin.ListenerPlanDeleteInp
}

type AdminListenerPlanDeleteRes struct{}

type AdminListenerPlanStatusReq struct {
	g.Meta `path:"/publish/admin/listen/status" method:"post" tags:"上架插件管理端" summary:"切换监听计划状态"`
	sysin.ListenerPlanStatusInp
}

type AdminListenerPlanStatusRes struct{}

type AdminListenerPlanUnbindReq struct {
	g.Meta `path:"/publish/admin/listen/unbind" method:"post" tags:"上架插件管理端" summary:"解绑监听目标"`
	sysin.ListenerPlanUnbindInp
}

type AdminListenerPlanUnbindRes struct{}
