// Package config
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package config

import (
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/lazysheep_tggo/model"
	"hotgo/addons/lazysheep_tggo/model/input/sysin"
)

type GetReq struct {
	g.Meta `path:"/config/get" method:"get" tags:"懒羊羊TGGo" summary:"获取配置"`
	sysin.GetConfigInp
}

type GetRes struct {
	*sysin.GetConfigModel
}

type UpdateReq struct {
	g.Meta `path:"/config/update" method:"post" tags:"懒羊羊TGGo" summary:"更新配置"`
	sysin.UpdateConfigInp
}

type UpdateRes struct{}

type UpsertBotReq struct {
	g.Meta `path:"/config/upsertBot" method:"post" tags:"懒羊羊TGGo" summary:"保存机器人"`
	sysin.BotUpsertInp
}

type UpsertBotRes struct {
	Key string `json:"key"`
}

type BotsReq struct {
	g.Meta `path:"/config/bots" method:"get" tags:"懒羊羊TGGo" summary:"机器人列表"`
}

type BotsRes struct {
	Bots map[string]*model.BotConfig `json:"bots"`
}

type ChannelListReq struct {
	g.Meta `path:"/config/channelList" method:"get" tags:"懒羊羊TGGo" summary:"频道使用列表"`
	sysin.ChannelListInp
}

type ChannelListRes struct {
	*sysin.ChannelListModel
}

type InspectBotReq struct {
	g.Meta `path:"/config/inspectBot" method:"post" tags:"懒羊羊TGGo" summary:"检测机器人"`
	sysin.BotInspectInp
}

type InspectBotRes struct {
	*sysin.BotInspectModel
}

type DeleteBotReq struct {
	g.Meta `path:"/config/deleteBot" method:"post" tags:"懒羊羊TGGo" summary:"删除机器人"`
	sysin.BotDeleteInp
}

type DeleteBotRes struct{}

type StartBotReq struct {
	g.Meta `path:"/config/startBot" method:"post" tags:"懒羊羊TGGo" summary:"启动机器人"`
	sysin.BotStartInp
}

type StartBotRes struct{}

type BotUsersReq struct {
	g.Meta `path:"/config/botUsers" method:"get" tags:"懒羊羊TGGo" summary:"机器人用户列表"`
	sysin.BotUserListInp
}

type BotUsersRes struct {
	List []*sysin.BotUserListModel `json:"list"`
}

type UpdateBotUserReq struct {
	g.Meta `path:"/config/updateBotUser" method:"post" tags:"懒羊羊TGGo" summary:"更新机器人用户"`
	sysin.BotUserEditInp
}

type UpdateBotUserRes struct{}

type TestTelegramProxyReq struct {
	g.Meta `path:"/config/testTelegramProxy" method:"post" tags:"懒羊羊TGGo" summary:"检测Telegram代理"`
	sysin.TelegramProxyTestInp
}

type TestTelegramProxyRes struct {
	*sysin.TelegramProxyTestModel
}

type PullMonitorReq struct {
	g.Meta `path:"/config/pullMonitor" method:"get" tags:"懒羊羊TGGo" summary:"拉取推送监控"`
	sysin.PullMonitorInp
}

type PullMonitorRes struct {
	*sysin.PullMonitorModel
}

type PullMonitorOverviewReq struct {
	g.Meta `path:"/config/pullMonitorOverview" method:"get" tags:"懒羊羊TGGo" summary:"拉取监控概览"`
	sysin.PullMonitorInp
}

type PullMonitorOverviewRes struct {
	*sysin.PullMonitorModel
}

type PullMonitorBindingsReq struct {
	g.Meta `path:"/config/pullMonitorBindings" method:"get" tags:"懒羊羊TGGo" summary:"拉取监控频道"`
	sysin.PullMonitorInp
}

type PullMonitorBindingsRes struct {
	*sysin.PullMonitorModel
}

type PullMonitorRecentReq struct {
	g.Meta `path:"/config/pullMonitorRecent" method:"get" tags:"懒羊羊TGGo" summary:"拉取监控明细"`
	sysin.PullMonitorInp
}

type PullMonitorRecentRes struct {
	*sysin.PullMonitorModel
}

type PushQueueMonitorReq struct {
	g.Meta `path:"/config/pushQueueMonitor" method:"get" tags:"懒羊羊TGGo" summary:"推送队列监控"`
	sysin.PushQueueMonitorInp
}

type PushQueueMonitorRes struct {
	*sysin.PushQueueMonitorModel
}

type PushQueueControlReq struct {
	g.Meta `path:"/config/pushQueueControl" method:"post" tags:"懒羊羊TGGo" summary:"推送队列控制"`
	sysin.PushQueueControlInp
}

type PushQueueControlRes struct{}

type BindingAutoPullControlReq struct {
	g.Meta `path:"/config/bindingAutoPullControl" method:"post" tags:"懒羊羊TGGo" summary:"绑定自动拉取控制"`
	sysin.BindingAutoPullControlInp
}

type BindingAutoPullControlRes struct{}
