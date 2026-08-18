package router

import (
	"context"

	"github.com/gogf/gf/v2/net/ghttp"
	"hotgo/addons/youban_open/controller/api"
	"hotgo/addons/youban_open/global"
	"hotgo/internal/consts"
	"hotgo/internal/library/addons"
	"hotgo/internal/service"
)

func Api(ctx context.Context, group *ghttp.RouterGroup) {
	prefix := addons.RouterPrefix(ctx, consts.AppApi, global.GetSkeleton().Name)
	group.Group(prefix, func(group *ghttp.RouterGroup) {
		group.GET("/health", func(r *ghttp.Request) { r.Response.WriteJson(map[string]string{"status": "ok"}) })
		group.Bind(api.CmsInstance)
		group.Group("/", func(auth *ghttp.RouterGroup) {
			auth.Middleware(service.Middleware().ApiAuth)
			auth.Bind(api.CmsBinding)
		})
		group.Group("/", func(open *ghttp.RouterGroup) {
			open.Middleware(openAPIAuth)
			open.Bind(api.OpenProfile, api.OpenChat)
		})
	})
	// Backward-compatible endpoint for existing XC-CMS installations.
	group.Group("/api/youban_publish", func(legacy *ghttp.RouterGroup) {
		legacy.Bind(api.CmsInstance)
		legacy.Group("/", func(open *ghttp.RouterGroup) {
			open.Middleware(openAPIAuth)
			open.Bind(api.OpenProfile, api.OpenChat)
		})
	})
}
