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

type AccountSettingViewReq struct {
	g.Meta `path:"/publish/account/setting/view" method:"get" tags:"上架插件" summary:"当前账号推送设置详情"`
}

type AccountSettingViewRes struct {
	*sysin.AccountSettingModel
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

type AdminAccountSettingViewReq struct {
	g.Meta `path:"/publish/admin/account/setting/view" method:"get" tags:"上架插件管理端" summary:"账号推送设置详情"`
	sysin.AccountSettingViewInp
}

type AdminAccountSettingViewRes struct {
	*sysin.AccountSettingModel
}

type AdminAccountSettingSaveReq struct {
	g.Meta `path:"/publish/admin/account/setting/save" method:"post" tags:"上架插件管理端" summary:"保存账号推送设置"`
	sysin.AccountSettingSaveInp
}

type AdminAccountSettingSaveRes struct {
	*sysin.AccountSettingModel
}

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

type AdminChannelCacheListReq struct {
	g.Meta `path:"/publish/admin/channel/cache/list" method:"get" tags:"上架插件管理端" summary:"TG账号频道缓存列表"`
	sysin.ChannelCacheListInp
}

type AdminChannelCacheListRes struct {
	form.PageRes
	List []*sysin.ChannelCacheModel `json:"list" dc:"频道缓存列表"`
}

type AdminChannelCacheRefreshReq struct {
	g.Meta `path:"/publish/admin/channel/cache/refresh" method:"post" tags:"上架插件管理端" summary:"刷新TG账号频道缓存"`
	sysin.ChannelCacheRefreshInp
}

type AdminChannelCacheRefreshRes struct {
	*sysin.ChannelCacheRefreshModel
}

type AdminChannelCheckReq struct {
	g.Meta `path:"/publish/admin/channel/check" method:"post" tags:"上架插件管理端" summary:"检测频道Bot权限"`
	sysin.ChannelCheckInp
}

type AdminChannelCheckRes struct {
	*sysin.ChannelCheckModel
}

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

type AdminUploadMediaReq struct {
	g.Meta `path:"/publish/admin/media/upload" method:"post" mime:"multipart/form-data" tags:"上架插件管理端" summary:"上传资料媒体"`
	sysin.MediaUploadInp
}

type AdminUploadMediaRes struct {
	*sysin.MediaModel
}

type AdminSortMediaReq struct {
	g.Meta `path:"/publish/admin/media/sort" method:"post" tags:"上架插件管理端" summary:"保存资料媒体排序"`
	sysin.MediaSortInp
}

type AdminSortMediaRes struct{}

type AdminDeleteMediaReq struct {
	g.Meta `path:"/publish/admin/media/delete" method:"post" tags:"上架插件管理端" summary:"删除资料媒体"`
	sysin.MediaDeleteInp
}

type AdminDeleteMediaRes struct{}

type AdminProfileListReq struct {
	g.Meta `path:"/publish/admin/profile/list" method:"get" tags:"上架插件管理端" summary:"资料列表"`
	sysin.ProfileListInp
}

type AdminProfileListRes struct {
	form.PageRes
	List []*sysin.ProfileModel `json:"list" dc:"资料列表"`
}

type AdminProfileViewReq struct {
	g.Meta `path:"/publish/admin/profile/view" method:"get" tags:"上架插件管理端" summary:"资料详情"`
	sysin.ProfileViewInp
}

type AdminProfileViewRes struct {
	*sysin.ProfileViewModel
}

type AdminProfileSaveReq struct {
	g.Meta `path:"/publish/admin/profile/save" method:"post" tags:"上架插件管理端" summary:"保存资料"`
	sysin.ProfileSaveInp
}

type AdminProfileSaveRes struct {
	Id     int64  `json:"id" dc:"资料ID"`
	Uuid   string `json:"uuid" dc:"资料UUID"`
	TaskId int64  `json:"taskId" dc:"任务ID"`
}

type AdminProfileDeleteReq struct {
	g.Meta `path:"/publish/admin/profile/delete" method:"post" tags:"上架插件管理端" summary:"删除资料"`
	sysin.ProfileDeleteInp
}

type AdminProfileDeleteRes struct{}

type AdminProfileStatusReq struct {
	g.Meta `path:"/publish/admin/profile/status" method:"post" tags:"上架插件管理端" summary:"资料上下架状态"`
	sysin.ProfileStatusInp
}

type AdminProfileStatusRes struct{}

type AdminNoteListReq struct {
	g.Meta `path:"/publish/admin/note/list" method:"get" tags:"上架插件管理端" summary:"笔记列表"`
	sysin.NoteListInp
}

type AdminNoteListRes struct {
	form.PageRes
	List []*sysin.NoteModel `json:"list" dc:"笔记列表"`
}

type AdminTagListReq struct {
	g.Meta `path:"/publish/admin/tag/list" method:"get" tags:"上架插件管理端" summary:"标签列表"`
	sysin.TagListInp
}

type AdminTagListRes struct {
	form.PageRes
	List []*sysin.TagModel `json:"list" dc:"标签列表"`
}

type AdminTagSaveReq struct {
	g.Meta `path:"/publish/admin/tag/save" method:"post" tags:"上架插件管理端" summary:"新增标签"`
	sysin.TagSaveInp
}

type AdminTagSaveRes struct{}

type AdminTagDeleteReq struct {
	g.Meta `path:"/publish/admin/tag/delete" method:"post" tags:"上架插件管理端" summary:"删除标签"`
	sysin.TagDeleteInp
}

type AdminTagDeleteRes struct{}

type AdminCityForwardReq struct {
	g.Meta `path:"/publish/admin/city/forward" method:"get" tags:"上架插件管理端" summary:"城市转发"`
	sysin.CityForwardInp
}

type AdminCityForwardRes struct {
	*sysin.CityForwardModel
}

type AdminProfileStatsReq struct {
	g.Meta `path:"/publish/admin/profile/stats" method:"get" tags:"上架插件管理端" summary:"个人中心统计趋势"`
	sysin.TrendInp
}

type AdminProfileStatsRes struct {
	*sysin.ProfileStatsModel
}

type AdminPublishConfigViewReq struct {
	g.Meta `path:"/publish/admin/config/view" method:"get" tags:"上架插件管理端" summary:"查看全局推送配置"`
	sysin.PublishConfigViewInp
}

type AdminPublishConfigViewRes struct {
	*sysin.PublishConfigViewModel
}

type AdminPublishConfigSaveReq struct {
	g.Meta `path:"/publish/admin/config/save" method:"post" tags:"上架插件管理端" summary:"保存全局推送配置"`
	sysin.PublishConfigSaveInp
}

type AdminPublishConfigSaveRes struct{}

type AdminAutoDeleteConfigViewReq struct {
	g.Meta `path:"/publish/admin/autoDelete/view" method:"get" tags:"上架插件管理端" summary:"查看频道自动删除配置"`
	sysin.AutoDeleteConfigViewInp
}

type AdminAutoDeleteConfigViewRes struct {
	*sysin.AutoDeleteConfigViewModel
}

type AdminAutoDeleteConfigSaveReq struct {
	g.Meta `path:"/publish/admin/autoDelete/save" method:"post" tags:"上架插件管理端" summary:"保存频道自动删除配置"`
	sysin.AutoDeleteConfigSaveInp
}

type AdminAutoDeleteConfigSaveRes struct{}

type AdminAntiScanConfigViewReq struct {
	g.Meta `path:"/publish/admin/antiScan/view" method:"get" tags:"上架插件管理端" summary:"查看防扫图配置"`
	sysin.AntiScanConfigViewInp
}

type AdminAntiScanConfigViewRes struct {
	*sysin.AntiScanConfigViewModel
}

type AdminAntiScanConfigSaveReq struct {
	g.Meta `path:"/publish/admin/antiScan/save" method:"post" tags:"上架插件管理端" summary:"保存防扫图配置"`
	sysin.AntiScanConfigSaveInp
}

type AdminAntiScanConfigSaveRes struct{}

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

type MyImportTaskListReq struct {
	g.Meta `path:"/publish/importTask/list" method:"get" tags:"上架插件" summary:"我的旧站导入任务列表"`
	sysin.ImportTaskListInp
}

type MyImportTaskListRes struct {
	form.PageRes
	List []*sysin.ImportTaskModel `json:"list" dc:"任务列表"`
}

type MyImportTaskViewReq struct {
	g.Meta `path:"/publish/importTask/view" method:"get" tags:"上架插件" summary:"我的旧站导入任务详情"`
	sysin.ImportTaskViewInp
}

type MyImportTaskViewRes struct {
	*sysin.ImportTaskModel
}

type MyImportRunListReq struct {
	g.Meta `path:"/publish/importRun/list" method:"get" tags:"上架插件" summary:"我的旧站导入执行记录列表"`
	sysin.ImportRunListInp
}

type MyImportRunListRes struct {
	form.PageRes
	List []*sysin.ImportRunModel `json:"list" dc:"执行记录列表"`
}

type MyImportRunLogListReq struct {
	g.Meta `path:"/publish/importRun/logs" method:"get" tags:"上架插件" summary:"我的旧站导入执行日志"`
	sysin.ImportRunLogListInp
}

type MyImportRunLogListRes struct {
	form.PageRes
	List []*sysin.ImportRunLogModel `json:"list" dc:"日志列表"`
}

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

type SortMediaReq struct {
	g.Meta `path:"/publish/media/sort" method:"post" tags:"上架插件" summary:"保存任务媒体排序"`
	sysin.MediaSortInp
}

type SortMediaRes struct{}

type MyProfileListReq struct {
	g.Meta `path:"/publish/profile/list" method:"get" tags:"上架插件" summary:"我的资料列表"`
	sysin.ProfileListInp
}

type MyProfileListRes struct {
	form.PageRes
	List []*sysin.ProfileModel `json:"list" dc:"资料列表"`
}

type MyChannelListReq struct {
	g.Meta `path:"/publish/channel/list" method:"get" tags:"上架插件" summary:"我的可选推送频道"`
	sysin.ChannelListInp
}

type MyChannelListRes struct {
	form.PageRes
	List []*sysin.ChannelModel `json:"list" dc:"频道列表"`
}

type MyProfileViewReq struct {
	g.Meta `path:"/publish/profile/view" method:"get" tags:"上架插件" summary:"我的资料详情"`
	sysin.ProfileViewInp
}

type MyProfileViewRes struct {
	*sysin.ProfileViewModel
}

type MyProfileOptionsReq struct {
	g.Meta `path:"/publish/profile/options" method:"get" tags:"上架插件" summary:"我的资料页面选项"`
}

type MyProfileOptionsRes struct {
	*sysin.ProfileOptionsModel
}

type MyProfileSaveReq struct {
	g.Meta `path:"/publish/profile/save" method:"post" tags:"上架插件" summary:"保存我的资料"`
	sysin.ProfileSaveInp
}

type MyProfileSaveRes struct {
	Id     int64  `json:"id" dc:"资料ID"`
	Uuid   string `json:"uuid" dc:"资料UUID"`
	TaskId int64  `json:"taskId" dc:"任务ID"`
}

type MyProfileDeleteReq struct {
	g.Meta `path:"/publish/profile/delete" method:"post" tags:"上架插件" summary:"删除我的资料"`
	sysin.ProfileDeleteInp
}

type MyProfileDeleteRes struct{}

type MyProfileStatusReq struct {
	g.Meta `path:"/publish/profile/status" method:"post" tags:"上架插件" summary:"我的资料上下架状态"`
	sysin.ProfileStatusInp
}

type MyProfileStatusRes struct{}

type MyProfileImageSearchReq struct {
	g.Meta `path:"/publish/profile/image-search" method:"post" mime:"multipart/form-data" tags:"上架插件" summary:"我的资料图片搜索"`
	sysin.ProfileImageSearchInp
}

type MyProfileImageSearchRes struct {
	form.PageRes
	List []*sysin.NoteModel `json:"list" dc:"笔记列表"`
}

type MyNoteListReq struct {
	g.Meta `path:"/publish/note/list" method:"get" tags:"上架插件" summary:"我的笔记列表"`
	sysin.NoteListInp
}

type MyNoteListRes struct {
	form.PageRes
	List []*sysin.NoteModel `json:"list" dc:"笔记列表"`
}

type MyTagListReq struct {
	g.Meta `path:"/publish/tag/list" method:"get" tags:"上架插件" summary:"标签列表"`
	sysin.TagListInp
}

type MyTagListRes struct {
	form.PageRes
	List []*sysin.TagModel `json:"list" dc:"标签列表"`
}

type MyTagSaveReq struct {
	g.Meta `path:"/publish/tag/save" method:"post" tags:"上架插件" summary:"新增标签"`
	sysin.TagSaveInp
}

type MyTagSaveRes struct{}

type MyTagDeleteReq struct {
	g.Meta `path:"/publish/tag/delete" method:"post" tags:"上架插件" summary:"删除标签"`
	sysin.TagDeleteInp
}

type MyTagDeleteRes struct{}

type MyCityForwardReq struct {
	g.Meta `path:"/publish/city/forward" method:"get" tags:"上架插件" summary:"城市转发"`
	sysin.CityForwardInp
}

type MyCityForwardRes struct {
	*sysin.CityForwardModel
}

type MyProfileStatsReq struct {
	g.Meta `path:"/publish/profile/stats" method:"get" tags:"上架插件" summary:"个人中心统计趋势"`
	sysin.TrendInp
}

type MyProfileStatsRes struct {
	*sysin.ProfileStatsModel
}

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
