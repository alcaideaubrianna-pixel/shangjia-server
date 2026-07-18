// Package sys
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sys

import (
	"context"
	"hotgo/addons/lazysheep_tggo/api/admin/config"
	"hotgo/addons/lazysheep_tggo/model"
	"hotgo/addons/lazysheep_tggo/service"
)

var Config = cConfig{}

type cConfig struct{}

func (c *cConfig) GetConfig(ctx context.Context, req *config.GetReq) (res *config.GetRes, err error) {
	data, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return
	}
	res = &config.GetRes{GetConfigModel: req.GetConfigInp.ToModel(data)}
	return
}

func (c *cConfig) UpdateConfig(ctx context.Context, req *config.UpdateReq) (res *config.UpdateRes, err error) {
	err = service.SysLazysheepTggo().SaveConfig(ctx, &req.UpdateConfigInp)
	if err != nil {
		return
	}
	res = new(config.UpdateRes)
	return
}

func (c *cConfig) UpsertBot(ctx context.Context, req *config.UpsertBotReq) (res *config.UpsertBotRes, err error) {
	key, err := service.SysLazysheepTggo().UpsertBot(ctx, &req.BotUpsertInp)
	if err != nil {
		return
	}
	res = &config.UpsertBotRes{Key: key}
	return
}

func (c *cConfig) Bots(ctx context.Context, req *config.BotsReq) (res *config.BotsRes, err error) {
	state, err := service.SysLazysheepTggo().GetState(ctx)
	if err != nil {
		return
	}
	bots := make(map[string]*model.BotConfig, len(state.Bots))
	for key, item := range state.Bots {
		if item == nil {
			continue
		}
		next := *item
		next.Plugins = nil
		bots[key] = &next
	}
	res = &config.BotsRes{Bots: bots}
	return
}

func (c *cConfig) ChannelList(ctx context.Context, req *config.ChannelListReq) (res *config.ChannelListRes, err error) {
	data, err := service.SysLazysheepTggo().ChannelList(ctx, &req.ChannelListInp)
	if err != nil {
		return
	}
	res = &config.ChannelListRes{ChannelListModel: data}
	return
}

func (c *cConfig) InspectBot(ctx context.Context, req *config.InspectBotReq) (res *config.InspectBotRes, err error) {
	data, err := service.SysLazysheepTggo().InspectBot(ctx, &req.BotInspectInp)
	if err != nil {
		return
	}
	res = &config.InspectBotRes{BotInspectModel: data}
	return
}

func (c *cConfig) DeleteBot(ctx context.Context, req *config.DeleteBotReq) (res *config.DeleteBotRes, err error) {
	err = service.SysLazysheepTggo().DeleteBot(ctx, &req.BotDeleteInp)
	if err != nil {
		return
	}
	res = new(config.DeleteBotRes)
	return
}

func (c *cConfig) StartBot(ctx context.Context, req *config.StartBotReq) (res *config.StartBotRes, err error) {
	err = service.SysLazysheepTggo().StartBot(ctx, &req.BotStartInp)
	if err != nil {
		return
	}
	res = new(config.StartBotRes)
	return
}

func (c *cConfig) BotUsers(ctx context.Context, req *config.BotUsersReq) (res *config.BotUsersRes, err error) {
	list, err := service.SysLazysheepTggo().BotUsers(ctx, &req.BotUserListInp)
	if err != nil {
		return
	}
	res = &config.BotUsersRes{List: list}
	return
}

func (c *cConfig) UpdateBotUser(ctx context.Context, req *config.UpdateBotUserReq) (res *config.UpdateBotUserRes, err error) {
	err = service.SysLazysheepTggo().UpdateBotUser(ctx, &req.BotUserEditInp)
	if err != nil {
		return
	}
	res = new(config.UpdateBotUserRes)
	return
}

func (c *cConfig) TestTelegramProxy(ctx context.Context, req *config.TestTelegramProxyReq) (res *config.TestTelegramProxyRes, err error) {
	data, err := service.SysLazysheepTggo().TestTelegramProxy(ctx, &req.TelegramProxyTestInp)
	if err != nil {
		return
	}
	res = &config.TestTelegramProxyRes{TelegramProxyTestModel: data}
	return
}

func (c *cConfig) PullMonitor(ctx context.Context, req *config.PullMonitorReq) (res *config.PullMonitorRes, err error) {
	data, err := service.SysLazysheepTggo().PullMonitor(ctx, &req.PullMonitorInp)
	if err != nil {
		return
	}
	res = &config.PullMonitorRes{PullMonitorModel: data}
	return
}

func (c *cConfig) PullMonitorOverview(ctx context.Context, req *config.PullMonitorOverviewReq) (res *config.PullMonitorOverviewRes, err error) {
	req.PullMonitorInp.Section = "overview"
	data, err := service.SysLazysheepTggo().PullMonitor(ctx, &req.PullMonitorInp)
	if err != nil {
		return
	}
	res = &config.PullMonitorOverviewRes{PullMonitorModel: data}
	return
}

func (c *cConfig) PullMonitorBindings(ctx context.Context, req *config.PullMonitorBindingsReq) (res *config.PullMonitorBindingsRes, err error) {
	req.PullMonitorInp.Section = "bindings"
	data, err := service.SysLazysheepTggo().PullMonitor(ctx, &req.PullMonitorInp)
	if err != nil {
		return
	}
	res = &config.PullMonitorBindingsRes{PullMonitorModel: data}
	return
}

func (c *cConfig) PullMonitorRecent(ctx context.Context, req *config.PullMonitorRecentReq) (res *config.PullMonitorRecentRes, err error) {
	req.PullMonitorInp.Section = "recent"
	data, err := service.SysLazysheepTggo().PullMonitor(ctx, &req.PullMonitorInp)
	if err != nil {
		return
	}
	res = &config.PullMonitorRecentRes{PullMonitorModel: data}
	return
}

func (c *cConfig) PushQueueMonitor(ctx context.Context, req *config.PushQueueMonitorReq) (res *config.PushQueueMonitorRes, err error) {
	data, err := service.SysLazysheepTggo().PushQueueMonitor(ctx, &req.PushQueueMonitorInp)
	if err != nil {
		return
	}
	res = &config.PushQueueMonitorRes{PushQueueMonitorModel: data}
	return
}

func (c *cConfig) PushQueueControl(ctx context.Context, req *config.PushQueueControlReq) (res *config.PushQueueControlRes, err error) {
	err = service.SysLazysheepTggo().UpdatePushQueueControl(ctx, &req.PushQueueControlInp)
	if err != nil {
		return
	}
	res = new(config.PushQueueControlRes)
	return
}

func (c *cConfig) BindingAutoPullControl(ctx context.Context, req *config.BindingAutoPullControlReq) (res *config.BindingAutoPullControlRes, err error) {
	err = service.SysLazysheepTggo().UpdateBindingAutoPull(ctx, &req.BindingAutoPullControlInp)
	if err != nil {
		return
	}
	res = new(config.BindingAutoPullControlRes)
	return
}
