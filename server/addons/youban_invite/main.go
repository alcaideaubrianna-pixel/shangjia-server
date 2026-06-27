package youban_invite

import (
	"context"
	"sync"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"

	"hotgo/addons/youban_invite/global"
	"hotgo/addons/youban_invite/install"
	_ "hotgo/addons/youban_invite/logic"
	"hotgo/addons/youban_invite/router"
	"hotgo/internal/library/addons"
	internalService "hotgo/internal/service"
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
			Label:       "邀请返现",
			Name:        "youban_invite",
			Group:       1,
			Brief:       "悦伴邀请返现插件",
			Description: "提供邀请链接、返现配置、返现账单和后台管理能力",
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
		group.Middleware(internalService.Middleware().Addon)
		router.Api(m.ctx, group)
		router.Admin(m.ctx, group)
	})
	return
}

func (m *module) Stop() (err error) { return }

func (m *module) Ctx() context.Context { return m.ctx }

func (m *module) GetSkeleton() *addons.Skeleton { return m.skeleton }

func (m *module) Install(ctx context.Context) (err error) { return install.Install(ctx) }

func (m *module) Upgrade(ctx context.Context) (err error) { return install.Upgrade(ctx) }

func (m *module) UnInstall(ctx context.Context) (err error) { return }
