package youban_bot

import (
	"context"
	"sync"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"

	_ "hotgo/addons/youban_bot/crons"
	"hotgo/addons/youban_bot/global"
	"hotgo/addons/youban_bot/install"
	_ "hotgo/addons/youban_bot/logic"
	"hotgo/addons/youban_bot/router"
	botService "hotgo/addons/youban_bot/service"
	"hotgo/internal/library/addons"
	"hotgo/internal/service"
	"hotgo/utility/runrole"
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
			Label:       "全局机器人",
			Name:        "youban_bot",
			Group:       1,
			Brief:       "悦伴全局 Telegram Bot",
			Description: "提供 Telegram 验证码登录、账号绑定、消息通知和机器人功能插件能力",
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
	if runrole.Enabled(m.ctx, runrole.Account) || runrole.Enabled(m.ctx, runrole.Runtime) {
		botService.SysBot().StartRuntime(m.ctx)
	}
	return
}

func (m *module) Stop() (err error) {
	botService.SysBot().StopRuntime()
	return
}

func (m *module) Ctx() context.Context                      { return m.ctx }
func (m *module) GetSkeleton() *addons.Skeleton             { return m.skeleton }
func (m *module) Install(ctx context.Context) (err error)   { return install.Install(ctx) }
func (m *module) Upgrade(ctx context.Context) (err error)   { return install.Upgrade(ctx) }
func (m *module) UnInstall(ctx context.Context) (err error) { return }
