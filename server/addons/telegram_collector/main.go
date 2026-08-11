package telegram_collector

import (
	"context"
	"sync"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"

	"hotgo/addons/telegram_collector/global"
	"hotgo/addons/telegram_collector/install"
	_ "hotgo/addons/telegram_collector/logic"
	"hotgo/addons/telegram_collector/router"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/internal/library/addons"
	"hotgo/internal/service"
	"hotgo/utility/runrole"
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
			Label:       "Telegram采集中心",
			Name:        "telegram_collector",
			Group:       1,
			Brief:       "Telegram实时、历史、媒体采集基础插件",
			Description: "提供Telegram事件采集、媒体缓存、账号租约和标准化交付能力",
			Author:      "youban",
			Version:     "v0.1.0",
		},
		ctx: gctx.New(),
	}
	addons.RegisterModule(m)
}

func (m *module) Start(option *addons.Option) error {
	global.Init(m.ctx, m.skeleton)
	option.Server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(service.Middleware().Addon)
		router.Admin(m.ctx, group)
		router.Api(m.ctx, group)
	})
	if runrole.Enabled(m.ctx, runrole.Worker) || runrole.Enabled(m.ctx, runrole.CollectorWorker) {
		collectorservice.Collector().StartRuntime(m.ctx)
	}
	if runrole.Enabled(m.ctx, runrole.Worker) || runrole.Enabled(m.ctx, runrole.PushWorker) {
		collectorservice.Collector().StartDeliveryRuntime(m.ctx)
	}
	if runrole.Enabled(m.ctx, runrole.Account) || runrole.Enabled(m.ctx, runrole.Runtime) {
		collectorservice.AccountRuntime().Start(m.ctx)
	}
	return nil
}

func (m *module) Stop() error {
	collectorservice.AccountRuntime().Stop()
	collectorservice.Collector().StopRuntime()
	return nil
}

func (m *module) Ctx() context.Context { return m.ctx }

func (m *module) GetSkeleton() *addons.Skeleton { return m.skeleton }

func (m *module) Install(ctx context.Context) error { return install.Install(ctx) }

func (m *module) Upgrade(ctx context.Context) error { return install.Upgrade(ctx) }

func (m *module) UnInstall(context.Context) error { return nil }
