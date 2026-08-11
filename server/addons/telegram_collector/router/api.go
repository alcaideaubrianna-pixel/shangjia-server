package router

import (
	"context"

	"github.com/gogf/gf/v2/net/ghttp"
	"hotgo/addons/telegram_collector/global"
	"hotgo/internal/consts"
	"hotgo/internal/library/addons"
)

func Api(ctx context.Context, group *ghttp.RouterGroup) {
	prefix := addons.RouterPrefix(ctx, consts.AppApi, global.GetAddonName())
	group.Group(prefix, func(*ghttp.RouterGroup) {})
}
