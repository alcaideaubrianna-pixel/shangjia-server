package youban_two_way_bot

import (
	"context"
	"sync"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"

	"hotgo/addons/youban_two_way_bot/global"
	"hotgo/addons/youban_two_way_bot/install"
	_ "hotgo/addons/youban_two_way_bot/logic"
	"hotgo/addons/youban_two_way_bot/router"
	twservice "hotgo/addons/youban_two_way_bot/service"
	"hotgo/internal/library/addons"
	internalservice "hotgo/internal/service"
)

type module struct {
	skeleton *addons.Skeleton
	ctx      context.Context
	sync.Mutex
}

func init() { newModule() }

func newModule() {
	m := &module{
		skeleton: &addons.Skeleton{
			Label:       "双向机器人",
			Name:        "youban_two_way_bot",
			Group:       1,
			Brief:       "悦伴 Telegram 双向机器人",
			Description: "提供 Telegram 私聊与后台话题群的双向转发能力",
			Author:      "youban",
			Version:     "v1.0.0",
		},
		ctx: gctx.New(),
	}
	addons.RegisterModule(m)
}

func (m *module) Start(option *addons.Option) (err error) {
	global.Init(m.skeleton)
	option.Server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(internalservice.Middleware().Addon)
		router.Api(m.ctx, group)
		router.Admin(m.ctx, group)
	})
	twservice.SysTwoWayBot().StartRuntime(m.ctx)
	return
}

func (m *module) Stop() (err error) {
	twservice.SysTwoWayBot().StopRuntime()
	return
}

func (m *module) Ctx() context.Context                      { return m.ctx }
func (m *module) GetSkeleton() *addons.Skeleton             { return m.skeleton }
func (m *module) Install(ctx context.Context) (err error)   { return install.Install(ctx) }
func (m *module) Upgrade(ctx context.Context) (err error)   { return install.Upgrade(ctx) }
func (m *module) UnInstall(ctx context.Context) (err error) { return }
