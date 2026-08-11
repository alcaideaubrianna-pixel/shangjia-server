package youban_tg_bot_gateway

import (
	"context"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"
	"hotgo/addons/youban_tg_bot_gateway/global"
	_ "hotgo/addons/youban_tg_bot_gateway/logic/sys"
	"hotgo/addons/youban_tg_bot_gateway/router"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
	"hotgo/internal/library/addons"
	internalservice "hotgo/internal/service"
	"hotgo/utility/runrole"
	"sync"
)

type module struct {
	skeleton *addons.Skeleton
	ctx      context.Context
	sync.Mutex
}

func init() { newModule() }
func newModule() {
	m := &module{skeleton: &addons.Skeleton{Label: "TG Bot Gateway", Name: "youban_tg_bot_gateway", Group: 1, Brief: "Telegram Bot统一运行网关", Description: "统一管理Bot webhook、polling和业务Update分发", Author: "youban", Version: "v1.0.0"}, ctx: gctx.New()}
	addons.RegisterModule(m)
}
func (m *module) Start(option *addons.Option) error {
	global.Init(m.skeleton)
	option.Server.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(internalservice.Middleware().Addon)
		router.Api(m.ctx, group)
	})
	if runrole.Enabled(m.ctx, runrole.Account) || runrole.Enabled(m.ctx, runrole.Runtime) {
		gatewayservice.Gateway().StartRuntime(m.ctx)
	}
	return nil
}
func (m *module) Stop() error                     { gatewayservice.Gateway().StopRuntime(); return nil }
func (m *module) Ctx() context.Context            { return m.ctx }
func (m *module) GetSkeleton() *addons.Skeleton   { return m.skeleton }
func (m *module) Install(context.Context) error   { return nil }
func (m *module) Upgrade(context.Context) error   { return nil }
func (m *module) UnInstall(context.Context) error { return nil }
