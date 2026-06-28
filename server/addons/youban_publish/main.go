package youban_publish

import (
	"context"
	"sync"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"

	"hotgo/addons/youban_publish/global"
	"hotgo/addons/youban_publish/install"
	_ "hotgo/addons/youban_publish/logic"
	"hotgo/addons/youban_publish/router"
	publishService "hotgo/addons/youban_publish/service"
	"hotgo/internal/library/addons"
	"hotgo/internal/service"
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
			Label:       "上架系统",
			Name:        "youban_publish",
			Group:       1,
			Brief:       "悦伴租户资料上架插件",
			Description: "提供 SaaS 租户账号、上架子账号、资料上架任务和 Telegram 发布能力",
			Author:      "youban",
			Version:     "v1.0.0",
		},
		ctx: gctx.New(),
	}
	addons.RegisterModule(m)
}

func (m *module) Start(option *addons.Option) (err error) {
	global.Init(m.ctx, m.skeleton)
	option.Server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(service.Middleware().Addon)
		router.Api(m.ctx, group)
		router.Admin(m.ctx, group)
	})
	publishService.SysPublish().StartRuntime(m.ctx)
	return
}

func (m *module) Stop() (err error) {
	publishService.SysPublish().StopRuntime()
	return
}

func (m *module) Ctx() context.Context { return m.ctx }

func (m *module) GetSkeleton() *addons.Skeleton { return m.skeleton }

func (m *module) Install(ctx context.Context) (err error) { return install.Install(ctx) }

func (m *module) Upgrade(ctx context.Context) (err error) { return install.Upgrade(ctx) }

func (m *module) UnInstall(ctx context.Context) (err error) { return }
