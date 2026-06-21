package youban

import (
	"context"
	"sync"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"

	_ "hotgo/addons/youban/crons"
	"hotgo/addons/youban/global"
	"hotgo/addons/youban/install"
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
			Label:       "悦伴业务",
			Name:        "youban",
			Group:       1,
			Brief:       "悦伴内容、会员、公告和同步能力",
			Description: "悦伴业务模块，负责业务表初始化、内容同步定时任务和业务能力生命周期管理",
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
	})
	return
}

func (m *module) Stop() (err error) {
	return
}

func (m *module) Ctx() context.Context {
	return m.ctx
}

func (m *module) GetSkeleton() *addons.Skeleton {
	return m.skeleton
}

func (m *module) Install(ctx context.Context) (err error) {
	return install.Install(ctx)
}

func (m *module) Upgrade(ctx context.Context) (err error) {
	return install.Upgrade(ctx)
}

func (m *module) UnInstall(ctx context.Context) (err error) {
	return
}
