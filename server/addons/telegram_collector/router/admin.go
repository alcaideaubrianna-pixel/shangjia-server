package router

import (
	"context"

	"github.com/gogf/gf/v2/net/ghttp"
	"hotgo/addons/telegram_collector/global"
	"hotgo/addons/telegram_collector/router/genrouter"
	"hotgo/internal/consts"
	"hotgo/internal/library/addons"
)

func Admin(ctx context.Context, group *ghttp.RouterGroup) {
	prefix := addons.RouterPrefix(ctx, consts.AppAdmin, global.GetAddonName())
	group.Group(prefix, func(group *ghttp.RouterGroup) {
		genrouter.Register(ctx, group)
	})
}
