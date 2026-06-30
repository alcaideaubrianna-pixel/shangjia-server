package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type CurrentAccountReq struct {
	g.Meta `path:"/publish/account/current" method:"get" tags:"上架插件" summary:"当前上架账号"`
}

type CurrentAccountRes struct {
	*sysin.CurrentAccountModel
}

type UpdateAccountPasswordReq struct {
	g.Meta `path:"/publish/account/password" method:"post" tags:"上架插件" summary:"修改当前账号密码"`
	sysin.UpdateAccountPasswordInp
}

type UpdateAccountPasswordRes struct{}

type UpdateAccountProfileReq struct {
	g.Meta `path:"/publish/account/profile" method:"post" tags:"上架插件" summary:"修改当前账号基本信息"`
	sysin.UpdateAccountProfileInp
}

type UpdateAccountProfileRes struct {
	*sysin.CurrentAccountModel
}

type AccountLoginReq struct {
	g.Meta `path:"/publish/auth/login" method:"post" tags:"上架插件" summary:"上架账号登录"`
	sysin.AccountLoginInp
}

type AccountLoginRes struct {
	*sysin.AccountLoginModel
}

type AccountRegisterReq struct {
	g.Meta `path:"/publish/auth/register" method:"post" tags:"上架插件" summary:"上架管理员注册"`
	sysin.AccountRegisterInp
}

type AccountRegisterRes struct {
	*sysin.AccountRegisterModel
}

type AdminAccountListReq struct {
	g.Meta `path:"/publish/admin/account/list" method:"get" tags:"上架插件管理端" summary:"账号列表"`
	sysin.AccountListInp
}

type AdminAccountListRes struct {
	form.PageRes
	List []*sysin.AccountModel `json:"list" dc:"账号列表"`
}

type AdminAccountSaveReq struct {
	g.Meta `path:"/publish/admin/account/save" method:"post" tags:"上架插件管理端" summary:"新增或编辑账号"`
	sysin.AccountSaveInp
}

type AdminAccountSaveRes struct {
	Password string `json:"password" dc:"随机密码"`
}

type AdminAccountResetPasswordReq struct {
	g.Meta `path:"/publish/admin/account/resetPwd" method:"post" tags:"上架插件管理端" summary:"重置账号密码"`
	sysin.AccountResetPasswordInp
}

type AdminAccountResetPasswordRes struct {
	Password string `json:"password" dc:"随机密码"`
}

type AdminAccountDeleteReq struct {
	g.Meta `path:"/publish/admin/account/delete" method:"post" tags:"上架插件管理端" summary:"删除账号"`
	sysin.AccountDeleteInp
}

type AdminAccountDeleteRes struct{}

type AdminBotListReq struct {
	g.Meta `path:"/publish/admin/bot/list" method:"get" tags:"上架插件管理端" summary:"Bot列表"`
	sysin.BotListInp
}

type AdminBotListRes struct {
	form.PageRes
	List []*sysin.BotModel `json:"list" dc:"Bot列表"`
}

type AdminBotSaveReq struct {
	g.Meta `path:"/publish/admin/bot/save" method:"post" tags:"上架插件管理端" summary:"保存Bot"`
	sysin.BotSaveInp
}

type AdminBotSaveRes struct{}

type AdminBotDeleteReq struct {
	g.Meta `path:"/publish/admin/bot/delete" method:"post" tags:"上架插件管理端" summary:"删除Bot"`
	sysin.BotDeleteInp
}

type AdminBotDeleteRes struct{}

type AdminBotRefreshReq struct {
	g.Meta `path:"/publish/admin/bot/refresh" method:"post" tags:"上架插件管理端" summary:"刷新Bot状态"`
	sysin.BotRefreshInp
}

type AdminBotRefreshRes struct {
	List []*sysin.BotRefreshModel `json:"list" dc:"刷新结果"`
}

type AdminTgAccountListReq struct {
	g.Meta `path:"/publish/admin/tgAccount/list" method:"get" tags:"上架插件管理端" summary:"TG账号列表"`
	sysin.TgAccountListInp
}

type AdminTgAccountListRes struct {
	form.PageRes
	List []*sysin.TgAccountModel `json:"list" dc:"TG账号列表"`
}

type AdminTgAccountStartLoginReq struct {
	g.Meta `path:"/publish/admin/tgAccount/startLogin" method:"post" tags:"上架插件管理端" summary:"发起TG账号扫码登录"`
	sysin.TgAccountStartLoginInp
}

type AdminTgAccountStartLoginRes struct {
	*sysin.TgAccountModel
}

type AdminTgAccountLoginStatusReq struct {
	g.Meta `path:"/publish/admin/tgAccount/loginStatus" method:"get" tags:"上架插件管理端" summary:"查询TG账号扫码登录状态"`
	sysin.TgAccountLoginStatusInp
}

type AdminTgAccountLoginStatusRes struct {
	*sysin.TgAccountModel
}

type AdminTgAccountPasswordReq struct {
	g.Meta `path:"/publish/admin/tgAccount/password" method:"post" tags:"上架插件管理端" summary:"提交TG账号二次验证密码"`
	sysin.TgAccountPasswordInp
}

type AdminTgAccountPasswordRes struct {
	*sysin.TgAccountModel
}

type AdminTgAccountDeleteReq struct {
	g.Meta `path:"/publish/admin/tgAccount/delete" method:"post" tags:"上架插件管理端" summary:"删除TG账号"`
	sysin.TgAccountDeleteInp
}

type AdminTgAccountDeleteRes struct{}

type AdminTgAccountRefreshReq struct {
	g.Meta `path:"/publish/admin/tgAccount/refresh" method:"post" tags:"上架插件管理端" summary:"刷新TG账号状态"`
	sysin.TgAccountRefreshInp
}

type AdminTgAccountRefreshRes struct {
	List []*sysin.TgAccountRefreshModel `json:"list" dc:"刷新结果"`
}

type AdminChannelListReq struct {
	g.Meta `path:"/publish/admin/channel/list" method:"get" tags:"上架插件管理端" summary:"频道列表"`
	sysin.ChannelListInp
}

type AdminChannelListRes struct {
	form.PageRes
	List []*sysin.ChannelModel `json:"list" dc:"频道列表"`
}

type AdminChannelSaveReq struct {
	g.Meta `path:"/publish/admin/channel/save" method:"post" tags:"上架插件管理端" summary:"新增或编辑频道"`
	sysin.ChannelSaveInp
}

type AdminChannelSaveRes struct{}

type AdminChannelDeleteReq struct {
	g.Meta `path:"/publish/admin/channel/delete" method:"post" tags:"上架插件管理端" summary:"删除频道"`
	sysin.ChannelDeleteInp
}

type AdminChannelDeleteRes struct{}

type AdminChannelBatchBotsReq struct {
	g.Meta `path:"/publish/admin/channel/batchBots" method:"post" tags:"上架插件管理端" summary:"批量编辑频道Bot"`
	sysin.ChannelBatchBotsInp
}

type AdminChannelBatchBotsRes struct{}

type AdminChannelRefreshReq struct {
	g.Meta `path:"/publish/admin/channel/refresh" method:"post" tags:"上架插件管理端" summary:"批量刷新频道状态"`
	sysin.ChannelRefreshInp
}

type AdminChannelRefreshRes struct {
	List []*sysin.ChannelRefreshModel `json:"list" dc:"刷新结果"`
}

type MyTaskListReq struct {
	g.Meta `path:"/publish/task/list" method:"get" tags:"上架插件" summary:"我的上架任务列表"`
	sysin.TaskListInp
}

type MyTaskListRes struct {
	form.PageRes
	List []*sysin.TaskModel `json:"list" dc:"任务列表"`
}

type SaveTaskReq struct {
	g.Meta `path:"/publish/task/save" method:"post" tags:"上架插件" summary:"保存我的上架任务"`
	sysin.TaskSaveInp
}

type SaveTaskRes struct {
	Id int64 `json:"id" dc:"任务ID"`
}

type SubmitTaskReq struct {
	g.Meta `path:"/publish/task/submit" method:"post" tags:"上架插件" summary:"提交我的上架任务"`
	sysin.TaskSubmitInp
}

type SubmitTaskRes struct{}

type CancelTaskReq struct {
	g.Meta `path:"/publish/task/cancel" method:"post" tags:"上架插件" summary:"取消我的上架任务"`
	sysin.TaskCancelInp
}

type CancelTaskRes struct{}

type UploadMediaReq struct {
	g.Meta `path:"/publish/media/upload" method:"post" mime:"multipart/form-data" tags:"上架插件" summary:"上传任务媒体"`
	sysin.MediaUploadInp
}

type UploadMediaRes struct {
	*sysin.MediaModel
}

type MediaListReq struct {
	g.Meta `path:"/publish/media/list" method:"get" tags:"上架插件" summary:"任务媒体列表"`
	sysin.MediaListInp
}

type MediaListRes struct {
	List []*sysin.MediaModel `json:"list" dc:"媒体列表"`
}

type DeleteMediaReq struct {
	g.Meta `path:"/publish/media/delete" method:"post" tags:"上架插件" summary:"删除任务媒体"`
	sysin.MediaDeleteInp
}

type DeleteMediaRes struct{}

type TelegramLoginStartReq struct {
	g.Meta `path:"/telegram/login/start" method:"post" tags:"上架插件" summary:"发起Telegram扫码登录"`
	sysin.TelegramLoginStartInp
}

type TelegramLoginStartRes struct {
	*sysin.TelegramLoginModel
}

type TelegramLoginStatusReq struct {
	g.Meta `path:"/telegram/login/status" method:"get" tags:"上架插件" summary:"查询Telegram扫码登录状态"`
	sysin.TelegramLoginStatusInp
}

type TelegramLoginStatusRes struct {
	*sysin.TelegramLoginModel
}

type TelegramLoginPasswordReq struct {
	g.Meta `path:"/telegram/login/password" method:"post" tags:"上架插件" summary:"提交Telegram二次验证密码"`
	sysin.TelegramLoginPasswordInp
}

type TelegramLoginPasswordRes struct {
	*sysin.TelegramLoginModel
}
