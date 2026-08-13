package publish

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/model/input/form"
	basesysin "hotgo/internal/model/input/sysin"

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

type AccountSettingSaveReq struct {
	g.Meta          `path:"/publish/account/setting/save" method:"post" tags:"上架插件" summary:"保存当前账号推送设置"`
	EnableSuffix    int    `json:"enableSuffix" dc:"是否启用发送后缀"`
	SuffixContent   string `json:"suffixContent" dc:"发送后缀内容"`
	EnableTitleMark int    `json:"enableTitleMark" dc:"是否启用编号标识"`
	MarkMode        string `json:"markMode" dc:"前缀模式：nickname/custom"`
	NumberSource    string `json:"numberSource" dc:"编号来源：sequence/random"`
	CustomMarkText  string `json:"customMarkText" dc:"自定义前缀"`
	MarkPosition    string `json:"markPosition" dc:"显示位置：top/bottom/feeLine"`
}

type AccountSettingSaveRes struct {
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

type AdminAccountTransferPreviewReq struct {
	g.Meta `path:"/publish/admin/account/transferPreview" method:"get" tags:"上架插件管理端" summary:"账号资料转移预览"`
	sysin.AccountTransferPreviewInp
}

type AdminAccountTransferPreviewRes struct {
	*sysin.AccountTransferPreviewModel
}

type AdminAccountTransferProfilesReq struct {
	g.Meta `path:"/publish/admin/account/transferProfiles" method:"post" tags:"上架插件管理端" summary:"账号资料转移"`
	sysin.AccountTransferProfilesInp
}

type AdminAccountTransferProfilesRes struct {
	*sysin.AccountTransferProfilesModel
}

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

type AdminBotChannelCacheListReq struct {
	g.Meta `path:"/publish/admin/bot/channel/cache/list" method:"get" tags:"上架插件管理端" summary:"Bot频道缓存列表"`
	sysin.BotChannelCacheListInp
}

type AdminBotChannelCacheListRes struct {
	form.PageRes
	List []*sysin.BotChannelCacheModel `json:"list" dc:"Bot频道缓存"`
}

type AdminBotCreateReq struct {
	g.Meta `path:"/publish/admin/bot/create" method:"post" tags:"上架插件管理端" summary:"创建Bot"`
	sysin.BotCreateInp
}

type AdminBotCreateRes struct {
	*sysin.BotModel
}

type AdminBotUsernameCheckReq struct {
	g.Meta `path:"/publish/admin/bot/checkUsername" method:"post" tags:"上架插件管理端" summary:"检查Bot用户名"`
	sysin.BotUsernameCheckInp
}

type AdminBotUsernameCheckRes struct {
	*sysin.BotUsernameCheckModel
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

type AdminTgAccountPhoneStartReq struct {
	g.Meta `path:"/publish/admin/tgAccount/phone/start" method:"post" tags:"上架插件管理端" summary:"发起TG账号手机号登录"`
	sysin.TgAccountPhoneStartInp
}

type AdminTgAccountPhoneStartRes struct {
	*sysin.TgAccountModel
}

type AdminTgAccountCodeReq struct {
	g.Meta `path:"/publish/admin/tgAccount/code" method:"post" tags:"上架插件管理端" summary:"提交TG账号登录验证码"`
	sysin.TgAccountCodeInp
}

type AdminTgAccountCodeRes struct {
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

type AdminChannelCacheResolveReq struct {
	g.Meta `path:"/publish/admin/channel/cache/resolve" method:"post" tags:"上架插件管理端" summary:"解析TG账号频道缓存显示信息"`
	sysin.ChannelCacheResolveInp
}

type AdminChannelCacheResolveRes struct {
	List []*sysin.ChannelCacheResolveModel `json:"list" dc:"频道缓存显示列表"`
}

type AdminChannelCacheRefreshReq struct {
	g.Meta `path:"/publish/admin/channel/cache/refresh" method:"post" tags:"上架插件管理端" summary:"刷新TG账号频道缓存"`
	sysin.ChannelCacheRefreshInp
}

type AdminChannelCacheRefreshRes struct {
	*sysin.ChannelCacheRefreshModel
}

type AdminChannelCacheRefreshStatusReq struct {
	g.Meta `path:"/publish/admin/channel/cache/refresh/status" method:"get" tags:"上架插件管理端" summary:"查询TG账号频道缓存刷新状态"`
	sysin.ChannelCacheRefreshStatusInp
}

type AdminChannelCacheRefreshStatusRes struct {
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

type AdminChannelFullPushReq struct {
	g.Meta `path:"/publish/admin/channel/fullPush" method:"post" tags:"上架插件管理端" summary:"频道全量推送已上架资料"`
	sysin.ChannelFullPushInp
}

type AdminChannelFullPushRes struct {
	*sysin.ChannelFullPushModel
}

type AdminChannelCycleRunReq struct {
	g.Meta `path:"/publish/admin/channel/cycleRun" method:"post" tags:"上架插件管理端" summary:"手动执行频道循环曝光"`
	sysin.ChannelCycleRunInp
}

type AdminChannelCycleRunRes struct {
	*sysin.ChannelFullPushModel
}

type AdminChannelClearQueueReq struct {
	g.Meta `path:"/publish/admin/channel/clearQueue" method:"post" tags:"上架插件管理端" summary:"清空频道待发送队列"`
	sysin.ChannelClearQueueInp
}

type AdminChannelClearQueueRes struct {
	*sysin.ChannelClearQueueModel
}

type AdminMessageTemplateListReq struct {
	g.Meta `path:"/publish/admin/messageTemplate/list" method:"get" tags:"上架插件管理端" summary:"消息推送模板列表"`
	sysin.MessageTemplateListInp
}

type AdminMessageTemplateListRes struct {
	form.PageRes
	List []*sysin.MessageTemplateModel `json:"list" dc:"模板列表"`
}

type AdminMessageTemplateSaveReq struct {
	g.Meta `path:"/publish/admin/messageTemplate/save" method:"post" tags:"上架插件管理端" summary:"保存消息推送模板"`
	sysin.MessageTemplateSaveInp
}

type AdminMessageTemplateSaveRes struct {
	*sysin.MessageTemplateSaveModel
}

type AdminMessageTemplateDeleteReq struct {
	g.Meta `path:"/publish/admin/messageTemplate/delete" method:"post" tags:"上架插件管理端" summary:"删除消息推送模板"`
	sysin.MessageTemplateDeleteInp
}

type AdminMessageTemplateDeleteRes struct{}

type AdminMessageTemplateMediaUploadReq struct {
	g.Meta `path:"/publish/admin/messageTemplate/media/upload" method:"post" mime:"multipart/form-data" tags:"上架插件管理端" summary:"上传消息模板媒体"`
	sysin.MessageTemplateMediaUploadInp
}

type AdminMessageTemplateMediaUploadRes struct {
	*sysin.MessageTemplateMediaModel
}

type AdminMessageTemplatePushReq struct {
	g.Meta `path:"/publish/admin/messageTemplate/push" method:"post" tags:"上架插件管理端" summary:"推送消息模板"`
	sysin.MessageTemplatePushInp
}

type AdminMessageTemplatePushRes struct {
	*sysin.MessageTemplatePushModel
}

type AdminMessagePushPlanListReq struct {
	g.Meta `path:"/publish/admin/messagePushPlan/list" method:"get" tags:"上架插件管理端" summary:"消息自动推送计划列表"`
	sysin.MessagePushPlanListInp
}

type AdminMessagePushPlanListRes struct {
	form.PageRes
	List []*sysin.MessagePushPlanModel `json:"list" dc:"计划列表"`
}

type AdminMessagePushPlanSaveReq struct {
	g.Meta `path:"/publish/admin/messagePushPlan/save" method:"post" tags:"上架插件管理端" summary:"保存消息自动推送计划"`
	sysin.MessagePushPlanSaveInp
}

type AdminMessagePushPlanSaveRes struct {
	*sysin.MessagePushPlanSaveModel
}

type AdminMessagePushPlanDeleteReq struct {
	g.Meta `path:"/publish/admin/messagePushPlan/delete" method:"post" tags:"上架插件管理端" summary:"删除消息自动推送计划"`
	sysin.MessagePushPlanDeleteInp
}

type AdminMessagePushPlanDeleteRes struct{}

type AdminMessagePushPlanStatusReq struct {
	g.Meta `path:"/publish/admin/messagePushPlan/status" method:"post" tags:"上架插件管理端" summary:"切换消息自动推送计划状态"`
	sysin.MessagePushPlanStatusInp
}

type AdminMessagePushPlanStatusRes struct{}

type AdminQuickPushPlanListReq struct {
	g.Meta `path:"/publish/admin/quickPushPlan/list" method:"get" tags:"上架插件管理端" summary:"快速推送计划列表"`
	sysin.QuickPushPlanListInp
}

type AdminQuickPushPlanListRes struct {
	form.PageRes
	List []*sysin.QuickPushPlanModel `json:"list" dc:"计划列表"`
}

type AdminQuickPushPlanSaveReq struct {
	g.Meta `path:"/publish/admin/quickPushPlan/save" method:"post" tags:"上架插件管理端" summary:"保存快速推送计划"`
	sysin.QuickPushPlanSaveInp
}

type AdminQuickPushPlanSaveRes struct {
	*sysin.QuickPushPlanSaveModel
}

type AdminQuickPushPlanDeleteReq struct {
	g.Meta `path:"/publish/admin/quickPushPlan/delete" method:"post" tags:"上架插件管理端" summary:"删除快速推送计划"`
	sysin.QuickPushPlanDeleteInp
}

type AdminQuickPushPlanDeleteRes struct{}

type AdminQuickPushPlanStatusReq struct {
	g.Meta `path:"/publish/admin/quickPushPlan/status" method:"post" tags:"上架插件管理端" summary:"切换快速推送计划状态"`
	sysin.QuickPushPlanStatusInp
}

type AdminQuickPushPlanStatusRes struct{}

type AdminUploadMediaReq struct {
	g.Meta `path:"/publish/admin/media/upload" method:"post" mime:"multipart/form-data" tags:"上架插件管理端" summary:"上传资料媒体"`
	sysin.MediaUploadInp
}

type AdminUploadMediaRes struct {
	*sysin.MediaModel
}

type AdminMediaMultipartCheckReq struct {
	g.Meta `path:"/publish/admin/media/upload/check" method:"post" tags:"上架插件管理端" summary:"检查资料媒体分片"`
	*basesysin.CheckMultipartInp
}

type AdminMediaMultipartCheckRes struct {
	*basesysin.CheckMultipartModel
}

type AdminMediaMultipartPartReq struct {
	g.Meta `path:"/publish/admin/media/upload/part" method:"post" mime:"multipart/form-data" tags:"上架插件管理端" summary:"上传资料媒体分片"`
	*basesysin.UploadPartInp
}

type AdminMediaMultipartPartRes struct {
	*basesysin.UploadPartModel
}

type AdminMediaMultipartAttachReq struct {
	g.Meta `path:"/publish/admin/media/upload/attach" method:"post" tags:"上架插件管理端" summary:"绑定分片资料媒体"`
	sysin.MediaMultipartAttachInp
}

type AdminMediaMultipartAttachRes struct {
	*sysin.MediaModel
}

type AdminMediaDirectUploadCreateReq struct {
	g.Meta `path:"/publish/admin/media/direct-upload/create" method:"post" tags:"上架插件管理端" summary:"创建COS直传会话"`
	sysin.MediaDirectUploadCreateInp
}
type AdminMediaDirectUploadCreateRes struct {
	*sysin.MediaDirectUploadCreateModel
}
type AdminMediaDirectUploadSignReq struct {
	g.Meta `path:"/publish/admin/media/direct-upload/sign" method:"post" tags:"上架插件管理端" summary:"签发COS直传请求"`
	sysin.MediaDirectUploadSignInp
}
type AdminMediaDirectUploadSignRes struct {
	*sysin.MediaDirectUploadSignModel
}
type AdminMediaDirectUploadCompleteReq struct {
	g.Meta `path:"/publish/admin/media/direct-upload/complete" method:"post" mime:"multipart/form-data" tags:"上架插件管理端" summary:"完成COS直传"`
	sysin.MediaDirectUploadCompleteInp
}
type AdminMediaDirectUploadCompleteRes struct{ *sysin.MediaModel }

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

type AdminProfileEditReq struct {
	g.Meta `path:"/publish/admin/profile/edit" method:"post" tags:"上架插件管理端" summary:"编辑资料"`
	sysin.ProfileSaveInp
}

type AdminProfileCreateReq struct {
	g.Meta `path:"/publish/admin/profile/create" method:"post" tags:"上架插件管理端" summary:"新建资料"`
	sysin.ProfileSaveInp
}

type AdminProfileCreateRes = AdminProfileEditRes

type AdminProfileEditRes struct {
	Id   int64  `json:"id" dc:"资料ID"`
	Uuid string `json:"uuid" dc:"资料UUID"`
}

type AdminProfilePublishReq struct {
	g.Meta `path:"/publish/admin/profile/publish" method:"post" tags:"上架插件管理端" summary:"发布资料"`
	sysin.AdminProfilePublishInp
}

type AdminProfilePublishRes struct{}

type AdminProfileBatchCancelReq struct {
	g.Meta `path:"/publish/admin/profile/batch/cancel" method:"post" tags:"上架插件管理端" summary:"取消批量资料发布"`
	sysin.AdminProfileBatchCancelInp
}

type AdminProfileBatchCancelRes struct {
	*sysin.AdminProfileBatchCancelModel
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

type AdminProfileStatusRes struct {
	*sysin.ProfileStatusModel
}

type AdminProfileImageSearchReq struct {
	g.Meta `path:"/publish/admin/profile/image-search" method:"post" mime:"multipart/form-data" tags:"上架插件管理端" summary:"管理端资料图片搜索"`
	sysin.ProfileImageSearchInp
}

type AdminProfileImageSearchRes struct {
	form.PageRes
	List []*sysin.NoteModel `json:"list" dc:"笔记列表"`
}

type AdminTgMessageRepairStartReq struct {
	g.Meta `path:"/publish/admin/profile/messageRepair/start" method:"post" tags:"上架插件管理端" summary:"启动TG消息修复"`
	sysin.TgMessageRepairStartInp
}

type AdminTgMessageRepairStartRes struct {
	*sysin.TgMessageRepairModel
}

type AdminTgMessageRepairViewReq struct {
	g.Meta `path:"/publish/admin/profile/messageRepair/view" method:"get" tags:"上架插件管理端" summary:"查看TG消息修复进度"`
	sysin.TgMessageRepairViewInp
}

type AdminTgMessageRepairViewRes struct {
	*sysin.TgMessageRepairModel
}

type AdminNoteListReq struct {
	g.Meta `path:"/publish/admin/note/list" method:"get" tags:"上架插件管理端" summary:"笔记列表"`
	sysin.NoteListInp
}

type AdminNoteListRes struct {
	*sysin.AdminNotePageModel
}

type AdminNoteBatchIdsReq struct {
	g.Meta `path:"/publish/admin/note/batch/ids" method:"get" tags:"上架插件管理端" summary:"获取批量操作资料ID"`
	sysin.NoteListInp
}

type AdminNoteBatchIdsRes struct {
	*sysin.AdminNoteBatchIdsModel
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

type MediaMultipartCheckReq struct {
	g.Meta `path:"/publish/media/upload/check" method:"post" tags:"上架插件" summary:"检查资料媒体分片"`
	*basesysin.CheckMultipartInp
}

type MediaMultipartCheckRes struct {
	*basesysin.CheckMultipartModel
}

type MediaMultipartPartReq struct {
	g.Meta `path:"/publish/media/upload/part" method:"post" mime:"multipart/form-data" tags:"上架插件" summary:"上传资料媒体分片"`
	*basesysin.UploadPartInp
}

type MediaMultipartPartRes struct {
	*basesysin.UploadPartModel
}

type MediaMultipartAttachReq struct {
	g.Meta `path:"/publish/media/upload/attach" method:"post" tags:"上架插件" summary:"绑定分片资料媒体"`
	sysin.MediaMultipartAttachInp
}

type MediaMultipartAttachRes struct {
	*sysin.MediaModel
}

type MediaDirectUploadCreateReq struct {
	g.Meta `path:"/publish/media/direct-upload/create" method:"post" tags:"上架插件" summary:"创建COS直传会话"`
	sysin.MediaDirectUploadCreateInp
}
type MediaDirectUploadCreateRes struct {
	*sysin.MediaDirectUploadCreateModel
}
type MediaDirectUploadSignReq struct {
	g.Meta `path:"/publish/media/direct-upload/sign" method:"post" tags:"上架插件" summary:"签发COS直传请求"`
	sysin.MediaDirectUploadSignInp
}
type MediaDirectUploadSignRes struct {
	*sysin.MediaDirectUploadSignModel
}
type MediaDirectUploadCompleteReq struct {
	g.Meta `path:"/publish/media/direct-upload/complete" method:"post" mime:"multipart/form-data" tags:"上架插件" summary:"完成COS直传"`
	sysin.MediaDirectUploadCompleteInp
}
type MediaDirectUploadCompleteRes struct{ *sysin.MediaModel }

type MediaListReq struct {
	g.Meta `path:"/publish/media/list" method:"get" tags:"上架插件" summary:"资料媒体列表"`
	sysin.MediaListInp
}

type MediaListRes struct {
	List []*sysin.MediaModel `json:"list" dc:"媒体列表"`
}

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

type MyProfileEditReq struct {
	g.Meta `path:"/publish/profile/edit" method:"post" tags:"上架插件" summary:"编辑我的资料"`
	sysin.ProfileSaveInp
}

type MyProfileCreateReq struct {
	g.Meta `path:"/publish/profile/create" method:"post" tags:"上架插件" summary:"新建我的资料"`
	sysin.ProfileSaveInp
}

type MyProfileCreateRes = MyProfileEditRes

type MyProfileEditRes struct {
	Id   int64  `json:"id" dc:"资料ID"`
	Uuid string `json:"uuid" dc:"资料UUID"`
}

type MyProfilePublishReq struct {
	g.Meta `path:"/publish/profile/publish" method:"post" tags:"上架插件" summary:"发布我的资料"`
	sysin.ProfileViewInp
}

type MyProfilePublishRes struct{}

type MyProfileDeleteReq struct {
	g.Meta `path:"/publish/profile/delete" method:"post" tags:"上架插件" summary:"删除我的资料"`
	sysin.ProfileDeleteInp
}

type MyProfileDeleteRes struct{}

type MyProfileStatusReq struct {
	g.Meta `path:"/publish/profile/status" method:"post" tags:"上架插件" summary:"我的资料上下架状态"`
	sysin.ProfileStatusInp
}

type MyProfileStatusRes struct {
	*sysin.ProfileStatusModel
}

type MyTgMessageRepairStartReq struct {
	g.Meta `path:"/publish/profile/messageRepair/start" method:"post" tags:"上架插件" summary:"启动我的TG消息修复"`
	sysin.TgMessageRepairStartInp
}

type MyTgMessageRepairStartRes struct {
	*sysin.TgMessageRepairModel
}

type MyTgMessageRepairViewReq struct {
	g.Meta `path:"/publish/profile/messageRepair/view" method:"get" tags:"上架插件" summary:"查看我的TG消息修复进度"`
	sysin.TgMessageRepairViewInp
}

type MyTgMessageRepairViewRes struct {
	*sysin.TgMessageRepairModel
}

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
