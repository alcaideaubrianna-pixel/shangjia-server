package genrouter

import (
	"context"

	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/addons/youban_publish/global"
	"hotgo/internal/consts"
	"hotgo/internal/library/addons"
	"hotgo/internal/service"
)

var (
	NoLoginRouter       []interface{}
	AdminRequiredRouter []interface{}
	LoginRequiredRouter []interface{}
)

func Register(ctx context.Context, group *ghttp.RouterGroup) {
	adminPrefix := addons.RouterPrefix(ctx, consts.AppAdmin, global.GetSkeleton().Name)
	group.Group(adminPrefix, func(group *ghttp.RouterGroup) {
		if len(NoLoginRouter) > 0 {
			group.Bind(NoLoginRouter...)
		}
	})
	group.Group(adminPrefix, func(group *ghttp.RouterGroup) {
		group.Middleware(service.Middleware().AdminAuth)
		if len(AdminRequiredRouter) > 0 {
			group.Bind(AdminRequiredRouter...)
		}
	})
}
