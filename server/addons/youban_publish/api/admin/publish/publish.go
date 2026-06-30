package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type TenantListReq struct {
	g.Meta `path:"/publish/tenant/list" method:"get" tags:"上架插件后台" summary:"租户列表"`
	sysin.TenantListInp
}

type TenantListRes struct {
	form.PageRes
	List []*sysin.TenantModel `json:"list" dc:"租户列表"`
}

type TenantSaveReq struct {
	g.Meta `path:"/publish/tenant/save" method:"post" tags:"上架插件后台" summary:"新增或编辑租户"`
	sysin.TenantSaveInp
}

type TenantSaveRes struct {
	Password string `json:"password" dc:"管理员初始密码"`
}

type TenantDeleteReq struct {
	g.Meta `path:"/publish/tenant/delete" method:"post" tags:"上架插件后台" summary:"删除租户"`
	sysin.TenantDeleteInp
}

type TenantDeleteRes struct{}

type AccountListReq struct {
	g.Meta `path:"/admin/account/list" method:"get" tags:"上架插件后台" summary:"上架账号列表"`
	sysin.AccountListInp
}

type AccountListRes struct {
	form.PageRes
	List []*sysin.AccountModel `json:"list" dc:"账号列表"`
}

type AccountSaveReq struct {
	g.Meta `path:"/admin/account/save" method:"post" tags:"上架插件后台" summary:"新增或编辑上架账号"`
	sysin.AccountSaveInp
}

type AccountSaveRes struct {
	Password string `json:"password" dc:"初始密码"`
}

type AccountResetPasswordReq struct {
	g.Meta `path:"/admin/account/resetPwd" method:"post" tags:"上架插件后台" summary:"重置上架账号密码"`
	sysin.AccountResetPasswordInp
}

type AccountResetPasswordRes struct {
	Password string `json:"password" dc:"重置后的密码"`
}

type AccountDeleteReq struct {
	g.Meta `path:"/admin/account/delete" method:"post" tags:"上架插件后台" summary:"删除上架账号"`
	sysin.AccountDeleteInp
}

type AccountDeleteRes struct{}

type TaskListReq struct {
	g.Meta `path:"/admin/task/list" method:"get" tags:"上架插件后台" summary:"上架任务列表"`
	sysin.TaskListInp
}

type TaskListRes struct {
	form.PageRes
	List []*sysin.TaskModel `json:"list" dc:"任务列表"`
}

type TaskSaveReq struct {
	g.Meta `path:"/admin/task/save" method:"post" tags:"上架插件后台" summary:"新增或编辑上架任务"`
	sysin.TaskSaveInp
}

type TaskSaveRes struct {
	Id int64 `json:"id" dc:"任务ID"`
}

type TaskSubmitReq struct {
	g.Meta `path:"/admin/task/submit" method:"post" tags:"上架插件后台" summary:"提交上架任务"`
	sysin.TaskSubmitInp
}

type TaskSubmitRes struct{}

type TaskCancelReq struct {
	g.Meta `path:"/admin/task/cancel" method:"post" tags:"上架插件后台" summary:"取消上架任务"`
	sysin.TaskCancelInp
}

type TaskCancelRes struct{}

type MediaListReq struct {
	g.Meta `path:"/admin/media/list" method:"get" tags:"上架插件后台" summary:"任务媒体列表"`
	sysin.MediaListInp
}

type MediaListRes struct {
	List []*sysin.MediaModel `json:"list" dc:"媒体列表"`
}

type MediaDeleteReq struct {
	g.Meta `path:"/admin/media/delete" method:"post" tags:"上架插件后台" summary:"删除任务媒体"`
	sysin.MediaDeleteInp
}

type MediaDeleteRes struct{}

type BotListReq struct {
	g.Meta `path:"/admin/bot/list" method:"get" tags:"上架插件后台" summary:"Bot列表"`
	sysin.BotListInp
}

type BotListRes struct {
	form.PageRes
	List []*sysin.BotModel `json:"list" dc:"Bot列表"`
}

type BotSaveReq struct {
	g.Meta `path:"/admin/bot/save" method:"post" tags:"上架插件后台" summary:"新增或编辑Bot"`
	sysin.BotSaveInp
}

type BotSaveRes struct{}

type BotDeleteReq struct {
	g.Meta `path:"/admin/bot/delete" method:"post" tags:"上架插件后台" summary:"删除Bot"`
	sysin.BotDeleteInp
}

type BotDeleteRes struct{}

type BotRefreshReq struct {
	g.Meta `path:"/admin/bot/refresh" method:"post" tags:"上架插件后台" summary:"刷新Bot状态"`
	sysin.BotRefreshInp
}

type BotRefreshRes struct {
	List []*sysin.BotRefreshModel `json:"list" dc:"刷新结果"`
}

type TgAccountListReq struct {
	g.Meta `path:"/admin/tgAccount/list" method:"get" tags:"上架插件后台" summary:"TG账号列表"`
	sysin.TgAccountListInp
}

type TgAccountListRes struct {
	form.PageRes
	List []*sysin.TgAccountModel `json:"list" dc:"TG账号列表"`
}

type TgAccountStartLoginReq struct {
	g.Meta `path:"/admin/tgAccount/startLogin" method:"post" tags:"上架插件后台" summary:"发起TG账号扫码登录"`
	sysin.TgAccountStartLoginInp
}

type TgAccountStartLoginRes struct {
	*sysin.TgAccountModel
}

type TgAccountLoginStatusReq struct {
	g.Meta `path:"/admin/tgAccount/loginStatus" method:"get" tags:"上架插件后台" summary:"查询TG账号扫码登录状态"`
	sysin.TgAccountLoginStatusInp
}

type TgAccountLoginStatusRes struct {
	*sysin.TgAccountModel
}

type TgAccountPasswordReq struct {
	g.Meta `path:"/admin/tgAccount/password" method:"post" tags:"上架插件后台" summary:"提交TG账号二次验证密码"`
	sysin.TgAccountPasswordInp
}

type TgAccountPasswordRes struct {
	*sysin.TgAccountModel
}

type TgAccountDeleteReq struct {
	g.Meta `path:"/admin/tgAccount/delete" method:"post" tags:"上架插件后台" summary:"删除TG账号"`
	sysin.TgAccountDeleteInp
}

type TgAccountDeleteRes struct{}

type TgAccountRefreshReq struct {
	g.Meta `path:"/admin/tgAccount/refresh" method:"post" tags:"上架插件后台" summary:"刷新TG账号状态"`
	sysin.TgAccountRefreshInp
}

type TgAccountRefreshRes struct {
	List []*sysin.TgAccountRefreshModel `json:"list" dc:"刷新结果"`
}

type ChannelListReq struct {
	g.Meta `path:"/admin/channel/list" method:"get" tags:"上架插件后台" summary:"频道列表"`
	sysin.ChannelListInp
}

type ChannelListRes struct {
	form.PageRes
	List []*sysin.ChannelModel `json:"list" dc:"频道列表"`
}

type ChannelSaveReq struct {
	g.Meta `path:"/admin/channel/save" method:"post" tags:"上架插件后台" summary:"新增或编辑频道"`
	sysin.ChannelSaveInp
}

type ChannelSaveRes struct{}

type ChannelDeleteReq struct {
	g.Meta `path:"/admin/channel/delete" method:"post" tags:"上架插件后台" summary:"删除频道"`
	sysin.ChannelDeleteInp
}

type ChannelDeleteRes struct{}

type ChannelBatchBotsReq struct {
	g.Meta `path:"/admin/channel/batchBots" method:"post" tags:"上架插件后台" summary:"批量编辑频道Bot"`
	sysin.ChannelBatchBotsInp
}

type ChannelBatchBotsRes struct{}

type ChannelRefreshReq struct {
	g.Meta `path:"/admin/channel/refresh" method:"post" tags:"上架插件后台" summary:"批量刷新频道状态"`
	sysin.ChannelRefreshInp
}

type ChannelRefreshRes struct {
	List []*sysin.ChannelRefreshModel `json:"list" dc:"刷新结果"`
}
