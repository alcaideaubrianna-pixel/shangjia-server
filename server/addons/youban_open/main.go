package youban_open

import (
	"context"
	"sync"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"

	"hotgo/addons/youban_open/global"
	"hotgo/addons/youban_open/install"
	_ "hotgo/addons/youban_open/logic"
	"hotgo/addons/youban_open/router"
	"hotgo/internal/library/addons"
	"hotgo/internal/service"
)

type module struct {
	skeleton *addons.Skeleton
	ctx      context.Context
	sync.Mutex
}

func init() {
	addons.RegisterModule(&module{
		skeleton: &addons.Skeleton{
			Label:       "开放平台",
			Name:        "youban_open",
			Group:       1,
			Brief:       "XC-CMS 开放平台授权与资料接口",
			Description: "管理 CMS 应用、实例授权、绑定码和开放资料接口",
			Author:      "youban",
			Version:     "v1.0.0",
		},
		ctx: gctx.New(),
	})
}

func (m *module) Start(option *addons.Option) error {
	global.Init(m.skeleton)
	option.Server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(service.Middleware().Addon)
		router.Admin(m.ctx, group)
		router.Api(m.ctx, group)
	})
	g.Log().Info(m.ctx, "youban_open 插件已启动")
	return nil
}

func (m *module) Stop() error                       { return nil }
func (m *module) Ctx() context.Context              { return m.ctx }
func (m *module) GetSkeleton() *addons.Skeleton     { return m.skeleton }
func (m *module) Install(ctx context.Context) error { return install.Install(ctx) }
func (m *module) Upgrade(ctx context.Context) error { return install.Upgrade(ctx) }
func (m *module) UnInstall(context.Context) error   { return nil }
