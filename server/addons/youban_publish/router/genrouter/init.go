package genrouter

import (
	"context"

	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/addons/youban_publish/global"
	"hotgo/internal/consts"
	"hotgo/internal/library/addons"
)

var (
	NoLoginRouter       []interface{}
	LoginRequiredRouter []interface{}
)

func Register(ctx context.Context, group *ghttp.RouterGroup) {
	prefix := addons.RouterPrefix(ctx, consts.AppAdmin, global.GetSkeleton().Name)
	group.Group(prefix, func(group *ghttp.RouterGroup) {
		if len(NoLoginRouter) > 0 {
			group.Bind(NoLoginRouter...)
		}
		group.Middleware(publishAdminAuth)
		if len(LoginRequiredRouter) > 0 {
			group.Bind(LoginRequiredRouter...)
		}
	})
}
