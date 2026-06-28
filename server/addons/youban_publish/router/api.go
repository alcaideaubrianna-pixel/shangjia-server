package router

import (
	"context"

	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/addons/youban_publish/controller/api"
	"hotgo/addons/youban_publish/global"
	"hotgo/internal/consts"
	"hotgo/internal/library/addons"
	"hotgo/internal/service"
)

func Api(ctx context.Context, group *ghttp.RouterGroup) {
	prefix := addons.RouterPrefix(ctx, consts.AppApi, global.GetSkeleton().Name)
	group.Group(prefix, func(group *ghttp.RouterGroup) {
		group.Middleware(service.Middleware().ApiAuth)
		group.Bind(api.Publish)
	})
}
