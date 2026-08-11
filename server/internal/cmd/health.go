package cmd

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"

	"hotgo/utility/runrole"
)

func registerHealthHandlers(server *ghttp.Server) {
	server.BindHandler("/healthz", func(r *ghttp.Request) {
		r.Response.WriteJsonExit(g.Map{
			"status": "ok",
			"role":   strings.Join(runrole.Roles(r.Context()), ","),
		})
	})
	server.BindHandler("/readyz", func(r *ghttp.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if _, err := g.DB().GetValue(ctx, "SELECT 1"); err != nil {
			writeReadinessFailure(r, "database")
			return
		}
		if _, err := g.Redis().Do(ctx, "PING"); err != nil {
			writeReadinessFailure(r, "redis")
			return
		}
		r.Response.WriteJsonExit(g.Map{
			"status": "ready",
			"role":   strings.Join(runrole.Roles(ctx), ","),
		})
	})
}

func writeReadinessFailure(r *ghttp.Request, dependency string) {
	r.Response.WriteHeader(http.StatusServiceUnavailable)
	r.Response.WriteJsonExit(g.Map{
		"status":     "not_ready",
		"dependency": dependency,
		"role":       strings.Join(runrole.Roles(r.Context()), ","),
	})
}
