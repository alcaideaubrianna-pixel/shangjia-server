package youban_feiniu_sync

import (
	"context"
	"sync"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/os/gctx"

	_ "hotgo/addons/youban_feiniu_sync/crons"
	"hotgo/addons/youban_feiniu_sync/global"
	"hotgo/addons/youban_feiniu_sync/install"
	_ "hotgo/addons/youban_feiniu_sync/logic"
	"hotgo/addons/youban_feiniu_sync/router"
	"hotgo/internal/library/addons"
	"hotgo/internal/service"
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
			Label:       "FeiNiu同步",
			Name:        "youban_feiniu_sync",
			Group:       1,
			Brief:       "定时同步 FeiNiu 数据到上架系统",
			Description: "同步 FeiNiu 频道、资料、文本、媒体和验证资料到 youban_publish 上架系统",
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
		router.Admin(m.ctx, group)
	})
	return
}
func (m *module) Stop() (err error)                         { return }
func (m *module) Ctx() context.Context                      { return m.ctx }
func (m *module) GetSkeleton() *addons.Skeleton             { return m.skeleton }
func (m *module) Install(ctx context.Context) (err error)   { return install.Install(ctx) }
func (m *module) Upgrade(ctx context.Context) (err error)   { return install.Upgrade(ctx) }
func (m *module) UnInstall(ctx context.Context) (err error) { return }
