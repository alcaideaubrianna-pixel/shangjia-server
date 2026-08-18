package genrouter

import (
	"context"
	"github.com/gogf/gf/v2/net/ghttp"
	"hotgo/addons/youban_open/global"
	"hotgo/internal/consts"
	"hotgo/internal/library/addons"
	"hotgo/internal/service"
)

var LoginRequiredRouter []interface{}

func Register(ctx context.Context, group *ghttp.RouterGroup) {
	p := addons.RouterPrefix(ctx, consts.AppAdmin, global.GetSkeleton().Name)
	group.Group(p, func(g *ghttp.RouterGroup) {
		g.Middleware(service.Middleware().AdminAuth)
		if len(LoginRequiredRouter) > 0 {
			g.Bind(LoginRequiredRouter...)
		}
	})
}
