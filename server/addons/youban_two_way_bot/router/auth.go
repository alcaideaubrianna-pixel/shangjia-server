package router

import (
	"net/http"

	"github.com/gogf/gf/v2/errors/gcode"
	"github.com/gogf/gf/v2/net/ghttp"

	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/response"
	"hotgo/internal/service"
)

func withPublishAdminAuth(group *ghttp.RouterGroup, bind func(group *ghttp.RouterGroup)) {
	group.Group("/", func(group *ghttp.RouterGroup) {
		group.Middleware(publishAdminAuth)
		bind(group)
	})
}

func publishAdminAuth(r *ghttp.Request) {
	if err := service.Middleware().DeliverUserContext(r); err != nil {
		r.Response.Status = http.StatusUnauthorized
		response.JsonExit(r, gcode.CodeNotAuthorized.Code(), err.Error())
		return
	}
	user := contexts.GetUser(r.Context())
	if user == nil || user.App != consts.AppApi {
		r.Response.Status = http.StatusUnauthorized
		response.JsonExit(r, gcode.CodeNotAuthorized.Code(), "请先登录上架系统")
		return
	}
	if user.DeptType != publishsysin.PublishAccountTypeAdmin {
		response.JsonExit(r, gcode.CodeSecurityReason.Code(), "当前账号无管理权限")
		return
	}
	r.Middleware.Next()
}
