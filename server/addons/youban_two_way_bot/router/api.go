package router

import (
	"context"

	"github.com/gogf/gf/v2/net/ghttp"

	adminsys "hotgo/addons/youban_two_way_bot/controller/admin/sys"
	"hotgo/addons/youban_two_way_bot/controller/api"
	"hotgo/addons/youban_two_way_bot/global"
	"hotgo/internal/consts"
	"hotgo/internal/library/addons"
)

func Api(ctx context.Context, group *ghttp.RouterGroup) {
	prefix := addons.RouterPrefix(ctx, consts.AppApi, global.GetSkeleton().Name)
	group.Group(prefix, func(group *ghttp.RouterGroup) {
		group.Bind(api.Webhook)
		withPublishAdminAuth(group, func(group *ghttp.RouterGroup) {
			group.Bind(adminsys.Bot)
		})
	})
}
