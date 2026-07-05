package genrouter

import (
	"strings"

	"github.com/gogf/gf/v2/errors/gcode"
	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/response"
	"hotgo/internal/service"
)

func publishAdminAuth(r *ghttp.Request) {
	if err := service.Middleware().DeliverUserContext(r); err != nil {
		response.JsonExit(r, gcode.CodeNotAuthorized.Code(), err.Error())
		return
	}

	user := contexts.GetUser(r.Context())
	if user == nil || user.App != consts.AppApi {
		response.JsonExit(r, gcode.CodeNotAuthorized.Code(), "请先登录上架系统")
		return
	}

	if isPublishSharedAdminPath(r.URL.Path) {
		r.Middleware.Next()
		return
	}

	if user.DeptType != sysin.PublishAccountTypeAdmin {
		response.JsonExit(r, gcode.CodeSecurityReason.Code(), "当前账号无管理权限")
		return
	}

	r.Middleware.Next()
}

// isPublishSharedAdminPath 判断管理员路由中允许上架账号共同使用的接口。
func isPublishSharedAdminPath(path string) bool {
	return strings.HasSuffix(path, "/publish/admin/antiScan/preview") ||
		strings.HasSuffix(path, "/publish-admin/antiScan/preview") ||
		strings.Contains(path, "/publish/admin/antiScan/material/") ||
		strings.Contains(path, "/publish-admin/antiScan/material/")
}
