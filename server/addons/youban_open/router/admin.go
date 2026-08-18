package router

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"hotgo/addons/youban_open/global"
	"hotgo/addons/youban_open/router/genrouter"
	"hotgo/internal/consts"
	"hotgo/internal/library/addons"
)

func Admin(ctx context.Context, group *ghttp.RouterGroup) {
	prefix := addons.RouterPrefix(ctx, consts.AppAdmin, global.GetSkeleton().Name)
	group.Group(prefix, func(group *ghttp.RouterGroup) {
		group.GET("/health", func(r *ghttp.Request) { r.Response.WriteJson(g.Map{"status": "ok"}) })
	})
	genrouter.Register(ctx, group)
}
