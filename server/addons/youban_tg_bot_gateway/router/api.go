package router

import (
	"context"
	"github.com/gogf/gf/v2/net/ghttp"
	"hotgo/addons/youban_tg_bot_gateway/controller/api"
	"hotgo/addons/youban_tg_bot_gateway/global"
	"hotgo/internal/consts"
	"hotgo/internal/library/addons"
)

func Api(ctx context.Context, group *ghttp.RouterGroup) {
	prefix := addons.RouterPrefix(ctx, consts.AppApi, global.GetSkeleton().Name)
	group.Group(prefix, func(group *ghttp.RouterGroup) { group.Bind(api.Webhook) })
}
