package router

import (
	"context"
	"hotgo/addons/youban_invite/controller/admin/sys"
	"hotgo/addons/youban_invite/global"
	"hotgo/internal/consts"
	"hotgo/internal/library/addons"
	"hotgo/internal/service"

	"github.com/gogf/gf/v2/net/ghttp"
)

func Admin(ctx context.Context, group *ghttp.RouterGroup) {
	prefix := addons.RouterPrefix(ctx, consts.AppAdmin, global.GetSkeleton().Name)
	group.Group(prefix, func(group *ghttp.RouterGroup) {
		group.Middleware(service.Middleware().AdminAuth)
		group.Bind(sys.Invite)
	})
}
