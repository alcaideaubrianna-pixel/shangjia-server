package bot

import (
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_bot/model/input/sysin"
	"hotgo/internal/model/input/form"
)

type BotListReq struct {
	g.Meta `path:"/bot/list" method:"get" tags:"全局机器人后台" summary:"Bot列表"`
	sysin.BotListInp
}

type BotListRes struct {
	form.PageRes
	List []*sysin.BotModel `json:"list" dc:"Bot列表"`
}

type BotSaveReq struct {
	g.Meta `path:"/bot/save" method:"post" tags:"全局机器人后台" summary:"新增或编辑Bot"`
	sysin.BotSaveInp
}

type BotSaveRes struct{}

type BotDeleteReq struct {
	g.Meta `path:"/bot/delete" method:"post" tags:"全局机器人后台" summary:"删除Bot"`
	sysin.BotDeleteInp
}

type BotDeleteRes struct{}

type BotRefreshReq struct {
	g.Meta `path:"/bot/refresh" method:"post" tags:"全局机器人后台" summary:"刷新Bot状态"`
	sysin.BotRefreshInp
}

type BotRefreshRes struct {
	List []*sysin.BotRefreshModel `json:"list" dc:"刷新结果"`
}

type BotRestartReq struct {
	g.Meta `path:"/bot/restart" method:"post" tags:"全局机器人后台" summary:"重启Bot"`
	sysin.BotRefreshInp
}

type BotRestartRes struct {
	List []*sysin.BotRefreshModel `json:"list" dc:"重启结果"`
}

type FeatureListReq struct {
	g.Meta `path:"/bot/feature/list" method:"get" tags:"全局机器人后台" summary:"Bot插件列表"`
	sysin.FeatureListInp
}

type FeatureListRes struct {
	form.PageRes
	List []*sysin.FeatureModel `json:"list" dc:"插件列表"`
}

type FeatureSaveReq struct {
	g.Meta `path:"/bot/feature/save" method:"post" tags:"全局机器人后台" summary:"保存Bot插件"`
	sysin.FeatureSaveInp
}

type FeatureSaveRes struct{}

type UserListReq struct {
	g.Meta `path:"/bot/user/list" method:"get" tags:"全局机器人后台" summary:"Bot用户列表"`
	sysin.UserListInp
}

type UserListRes struct {
	form.PageRes
	List []*sysin.UserModel `json:"list" dc:"用户列表"`
}

type AccountBindListReq struct {
	g.Meta `path:"/bot/binding/list" method:"get" tags:"全局机器人后台" summary:"TG绑定列表"`
	sysin.AccountBindListInp
}

type AccountBindListRes struct {
	form.PageRes
	List []*sysin.AccountBindModel `json:"list" dc:"TG绑定列表"`
}

type AccountBindUnbindReq struct {
	g.Meta `path:"/bot/binding/unbind" method:"post" tags:"全局机器人后台" summary:"解绑TG账号"`
	sysin.AccountBindUnbindInp
}

type AccountBindUnbindRes struct{}

type MessageListReq struct {
	g.Meta `path:"/bot/message/list" method:"get" tags:"全局机器人后台" summary:"Bot消息日志"`
	sysin.MessageListInp
}

type MessageListRes struct {
	form.PageRes
	List []*sysin.MessageModel `json:"list" dc:"消息日志"`
}

type BotChannelCacheListReq struct {
	g.Meta `path:"/bot/channel/cache/list" method:"get" tags:"全局机器人后台" summary:"Bot频道缓存列表"`
	sysin.BotChannelCacheListInp
}

type BotChannelCacheListRes struct {
	form.PageRes
	List []*sysin.BotChannelCacheModel `json:"list" dc:"Bot频道缓存"`
}

type UserSwitchSuperAdminReq struct {
	g.Meta `path:"/bot/user/superAdmin" method:"post" tags:"全局机器人后台" summary:"设置Bot超级管理员"`
	sysin.UserSwitchSuperAdminInp
}

type UserSwitchSuperAdminRes struct{}

type SendMessageReq struct {
	g.Meta `path:"/bot/message/send" method:"post" tags:"全局机器人后台" summary:"发送Bot消息"`
	sysin.SendMessageInp
}

type SendMessageRes struct{}
