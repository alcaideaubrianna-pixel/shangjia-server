package genrouter

import (
	"context"

	"github.com/gogf/gf/v2/net/ghttp"
	"hotgo/addons/telegram_collector/global"
	"hotgo/internal/consts"
	"hotgo/internal/library/addons"
	"hotgo/internal/service"
)

var LoginRequiredRouter []interface{}

func Register(ctx context.Context, group *ghttp.RouterGroup) {
	prefix := addons.RouterPrefix(ctx, consts.AppAdmin, global.GetAddonName())
	group.Group(prefix, func(group *ghttp.RouterGroup) {
		group.Middleware(service.Middleware().AdminAuth)
		if len(LoginRequiredRouter) > 0 {
			group.Bind(LoginRequiredRouter...)
		}
	})
}
