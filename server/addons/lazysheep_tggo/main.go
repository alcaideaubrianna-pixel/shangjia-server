// Package lazysheep_tggo
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package lazysheep_tggo

import (
	"context"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"
	_ "hotgo/addons/lazysheep_tggo/crons"
	"hotgo/addons/lazysheep_tggo/global"
	_ "hotgo/addons/lazysheep_tggo/logic"
	_ "hotgo/addons/lazysheep_tggo/queues"
	"hotgo/addons/lazysheep_tggo/router"
	aservice "hotgo/addons/lazysheep_tggo/service"
	"hotgo/internal/library/addons"
	iservice "hotgo/internal/service"
	"sync"
)

type module struct {
	skeleton *addons.Skeleton
	ctx      context.Context
	sync.Mutex
}

func init() {
	newModule()
}

func newModule() {
	m := &module{
		skeleton: &addons.Skeleton{
			Label:       "懒羊羊TGGo",
			Name:        "lazysheep_tggo",
			Group:       1,
			Logo:        "",
			Brief:       "Telegram 机器人、采集、会员、积分与审核插件",
			Description: "面向 Telegram 机器人业务的插件，提供消息采集、频道分发、会员、积分和签到能力",
			Author:      "Codex",
			Version:     "v0.1.0",
		},
		ctx: gctx.New(),
	}

	addons.RegisterModule(m)
}

// Start 启动模块
func (m *module) Start(option *addons.Option) (err error) {
	global.Init(m.ctx, m.skeleton)
	option.Server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(iservice.Middleware().Addon)
		router.Admin(m.ctx, group)
		router.Api(m.ctx, group)
	})
	go func() {
		if err := aservice.SysLazysheepTggo().BootBots(m.ctx); err != nil {
			g.Log().Warningf(m.ctx, "懒羊羊TGGo机器人启动失败：%+v", err)
		}
	}()
	aservice.SysLazysheepTggo().StartAutoPullLoop(m.ctx)
	aservice.SysLazysheepTggo().StartPullMonitorAggregator(m.ctx)
	aservice.SysLazysheepTggo().StartPushQueueLoop(m.ctx)
	return
}

// Stop 停止模块
func (m *module) Stop() (err error) {
	return
}

// Ctx 上下文
func (m *module) Ctx() context.Context {
	return m.ctx
}

// GetSkeleton 获取模块
func (m *module) GetSkeleton() *addons.Skeleton {
	return m.skeleton
}

// Install 安装模块
func (m *module) Install(ctx context.Context) (err error) {
	global.Init(m.ctx, m.skeleton)
	return aservice.SysLazysheepTggo().Install(ctx)
}

// Upgrade 更新模块
func (m *module) Upgrade(ctx context.Context) (err error) {
	global.Init(m.ctx, m.skeleton)
	return aservice.SysLazysheepTggo().Upgrade(ctx)
}

// UnInstall 卸载模块
func (m *module) UnInstall(ctx context.Context) (err error) {
	global.Init(m.ctx, m.skeleton)
	return aservice.SysLazysheepTggo().UnInstall(ctx)
}
