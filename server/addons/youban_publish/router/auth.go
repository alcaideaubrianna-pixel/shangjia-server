package router

import (
	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/internal/consts"
	"hotgo/internal/service"
)

func authMiddleware(appName string) func(*ghttp.Request) {
	if appName == consts.AppAdmin {
		return service.Middleware().AdminAuth
	}
	return service.Middleware().ApiAuth
}

func withAuth(group *ghttp.RouterGroup, appName string, bind func(group *ghttp.RouterGroup)) {
	group.Middleware(authMiddleware(appName))
	bind(group)
}
