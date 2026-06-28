package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type MerchantListReq struct {
	g.Meta `path:"/publish/merchant/list" method:"get" tags:"上架插件后台" summary:"商家列表"`
	sysin.MerchantListInp
}

type MerchantListRes struct {
	form.PageRes
	List []*sysin.MerchantModel `json:"list" dc:"商家列表"`
}

type MerchantSaveReq struct {
	g.Meta `path:"/publish/merchant/save" method:"post" tags:"上架插件后台" summary:"新增或编辑商家"`
	sysin.MerchantSaveInp
}

type MerchantSaveRes struct{}

type MerchantDeleteReq struct {
	g.Meta `path:"/publish/merchant/delete" method:"post" tags:"上架插件后台" summary:"删除商家"`
	sysin.MerchantDeleteInp
}

type MerchantDeleteRes struct{}

type AccountListReq struct {
	g.Meta `path:"/publish/account/list" method:"get" tags:"上架插件后台" summary:"上架账号列表"`
	sysin.AccountListInp
}

type AccountListRes struct {
	form.PageRes
	List []*sysin.AccountModel `json:"list" dc:"账号列表"`
}

type AccountSaveReq struct {
	g.Meta `path:"/publish/account/save" method:"post" tags:"上架插件后台" summary:"新增或编辑上架账号"`
	sysin.AccountSaveInp
}

type AccountSaveRes struct{}

type AccountDeleteReq struct {
	g.Meta `path:"/publish/account/delete" method:"post" tags:"上架插件后台" summary:"删除上架账号"`
	sysin.AccountDeleteInp
}

type AccountDeleteRes struct{}

type TaskListReq struct {
	g.Meta `path:"/publish/task/list" method:"get" tags:"上架插件后台" summary:"上架任务列表"`
	sysin.TaskListInp
}

type TaskListRes struct {
	form.PageRes
	List []*sysin.TaskModel `json:"list" dc:"任务列表"`
}

type TaskSaveReq struct {
	g.Meta `path:"/publish/task/save" method:"post" tags:"上架插件后台" summary:"新增或编辑上架任务"`
	sysin.TaskSaveInp
}

type TaskSaveRes struct {
	Id int64 `json:"id" dc:"任务ID"`
}

type TaskSubmitReq struct {
	g.Meta `path:"/publish/task/submit" method:"post" tags:"上架插件后台" summary:"提交上架任务"`
	sysin.TaskSubmitInp
}

type TaskSubmitRes struct{}

type TaskCancelReq struct {
	g.Meta `path:"/publish/task/cancel" method:"post" tags:"上架插件后台" summary:"取消上架任务"`
	sysin.TaskCancelInp
}

type TaskCancelRes struct{}

type MediaListReq struct {
	g.Meta `path:"/publish/media/list" method:"get" tags:"上架插件后台" summary:"任务媒体列表"`
	sysin.MediaListInp
}

type MediaListRes struct {
	List []*sysin.MediaModel `json:"list" dc:"媒体列表"`
}

type MediaDeleteReq struct {
	g.Meta `path:"/publish/media/delete" method:"post" tags:"上架插件后台" summary:"删除任务媒体"`
	sysin.MediaDeleteInp
}

type MediaDeleteRes struct{}

type BotListReq struct {
	g.Meta `path:"/publish/bot/list" method:"get" tags:"上架插件后台" summary:"Bot列表"`
	sysin.BotListInp
}

type BotListRes struct {
	form.PageRes
	List []*sysin.BotModel `json:"list" dc:"Bot列表"`
}

type BotSaveReq struct {
	g.Meta `path:"/publish/bot/save" method:"post" tags:"上架插件后台" summary:"新增或编辑Bot"`
	sysin.BotSaveInp
}

type BotSaveRes struct{}

type BotDeleteReq struct {
	g.Meta `path:"/publish/bot/delete" method:"post" tags:"上架插件后台" summary:"删除Bot"`
	sysin.BotDeleteInp
}

type BotDeleteRes struct{}
