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

type AccountListenerPlanListReq struct {
	g.Meta `path:"/publish/account/listen/list" method:"get" tags:"上架插件" summary:"上架账号监听计划列表"`
	sysin.ListenerPlanListInp
}
type AccountListenerPlanListRes struct {
	form.PageRes
	List []*sysin.ListenerPlanModel `json:"list"`
}

type AccountListenerPlanSaveReq struct {
	g.Meta `path:"/publish/account/listen/save" method:"post" tags:"上架插件" summary:"保存监听计划"`
	sysin.ListenerPlanSaveInp
}
type AccountListenerPlanSaveRes struct {
	Id int64 `json:"id"`
}

type AccountListenerPlanDeleteReq struct {
	g.Meta `path:"/publish/account/listen/delete" method:"post" tags:"上架插件" summary:"删除监听计划"`
	sysin.ListenerPlanDeleteInp
}
type AccountListenerPlanDeleteRes struct{}

type AccountListenerPlanStatusReq struct {
	g.Meta `path:"/publish/account/listen/status" method:"post" tags:"上架插件" summary:"切换监听计划状态"`
	sysin.ListenerPlanStatusInp
}
type AccountListenerPlanStatusRes struct{}

type AccountListenerPlanUnbindReq struct {
	g.Meta `path:"/publish/account/listen/unbind" method:"post" tags:"上架插件" summary:"解绑监听目标"`
	sysin.ListenerPlanUnbindInp
}
type AccountListenerPlanUnbindRes struct{}

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
