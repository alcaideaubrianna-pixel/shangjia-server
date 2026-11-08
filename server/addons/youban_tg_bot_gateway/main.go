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
	"strings"
	"sync"
	"time"
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
	// This path intentionally lives outside Addon middleware: that middleware
	// requires the addon prefix and would reject /_internal/* as an unknown
	// module before the handler can run.
	option.Server.Group("/", func(group *ghttp.RouterGroup) {
		group.GET("/_internal/telegram-bot-api/bot/:token/getWebhookInfo", func(r *ghttp.Request) {
			token := strings.TrimSpace(r.GetRouter("token").String())
			if token == "" || !strings.Contains(token, ":") {
				r.Response.Status = 400
				r.Response.WriteJson(map[string]interface{}{"ok": false, "error_code": 400, "description": "bot token is required"})
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			bot, err := gatewayservice.Gateway().Client(ctx, token)
			if err != nil {
				r.Response.Status = 502
				r.Response.WriteJson(map[string]interface{}{"ok": false, "error_code": 502, "description": err.Error()})
				return
			}
			info, err := bot.GetWebhookInfo(ctx)
			if err != nil {
				r.Response.Status = 502
				r.Response.WriteJson(map[string]interface{}{"ok": false, "error_code": 502, "description": err.Error()})
				return
			}
			r.Response.WriteJson(map[string]interface{}{"ok": true, "result": info})
		})
	})
	if runrole.Enabled(m.ctx, runrole.Runtime) {
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
