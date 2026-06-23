package middleware

import (
	"hotgo/internal/library/location"
	"hotgo/internal/library/token"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/net/gtrace"
	"github.com/gogf/gf/v2/os/gctx"
)

const traceIDHeader = "X-Trace-ID"

// AccessLog 记录结构化访问日志。
func (s *sMiddleware) AccessLog(r *ghttp.Request) {
	start := time.Now()
	traceID := gtrace.GetTraceID(r.Context())
	if traceID == "" {
		traceID = gctx.CtxId(r.Context())
	}
	r.Response.Header().Set(traceIDHeader, traceID)

	r.Middleware.Next()

	duration := time.Since(start)
	clientIP := location.GetClientIp(r)
	memberId := int64(0)
	if user, err := token.ParseLoginUser(r); err == nil && user != nil {
		memberId = user.Id
	}

	g.Log("access").Info(r.Context(), g.Map{
		"log_type":             "http_access",
		"trace_id":             traceID,
		"member_id":            memberId,
		"ip":                   clientIP,
		"method":               r.Method,
		"path":                 r.URL.Path,
		"query":                r.URL.RawQuery,
		"status":               r.Response.Status,
		"duration_ms":          duration.Milliseconds(),
		"duration":             duration.String(),
		"user_agent":           r.UserAgent(),
		"referer":              r.Referer(),
		"host":                 r.Host,
		"request_uri":          r.RequestURI,
		"request_content_type": r.Header.Get("Content-Type"),
		"x_forwarded_for":      r.Header.Get("X-Forwarded-For"),
		"x_real_ip":            r.Header.Get("X-Real-IP"),
	})
}
