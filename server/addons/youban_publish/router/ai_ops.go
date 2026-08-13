package router

import (
	"crypto/subtle"
	"os"
	"strings"

	"github.com/gogf/gf/v2/errors/gcode"
	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/addons/youban_publish/controller/api"
	"hotgo/internal/library/response"
)

func registerAIOps(group *ghttp.RouterGroup) {
	group.Group("/ai-ops", func(group *ghttp.RouterGroup) {
		group.Middleware(aiOpsAuth)
		group.Bind(api.AIOps)
	})
}

func aiOpsAuth(r *ghttp.Request) {
	expected := strings.TrimSpace(os.Getenv("YOUBAN_AI_OPS_TOKEN"))
	provided := strings.TrimSpace(r.Header.Get("X-AI-Ops-Token"))
	if !aiOpsTokenMatches(expected, provided) {
		r.Response.Status = 401
		response.JsonExit(r, gcode.CodeNotAuthorized.Code(), "AI运维凭证无效")
		return
	}
	r.Middleware.Next()
}

func aiOpsTokenMatches(expected, provided string) bool {
	return expected != "" && len(expected) == len(provided) && subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1
}
