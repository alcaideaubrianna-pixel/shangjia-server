package youban_chat

import (
	"context"
	"sync"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"

	"hotgo/addons/youban_chat/global"
	"hotgo/addons/youban_chat/install"
	_ "hotgo/addons/youban_chat/logic"
	"hotgo/addons/youban_chat/router"
	chatService "hotgo/addons/youban_chat/service"
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
			Label:       "悦伴聊天",
			Name:        "youban_chat",
			Group:       1,
			Brief:       "悦伴 Telegram 客服接入",
			Description: "负责 APP 聊天会话、消息和 Telegram topic bridge 接入",
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
	chatService.SysChat().StartRuntime(m.ctx)
	return
}

func (m *module) Stop() (err error) {
	chatService.SysChat().StopRuntime()
	return
}

func (m *module) Ctx() context.Context { return m.ctx }

func (m *module) GetSkeleton() *addons.Skeleton { return m.skeleton }

func (m *module) Install(ctx context.Context) (err error) { return install.Install(ctx) }

func (m *module) Upgrade(ctx context.Context) (err error) { return install.Upgrade(ctx) }

func (m *module) UnInstall(ctx context.Context) (err error) { return }
