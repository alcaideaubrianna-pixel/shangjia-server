package middleware

import (
	"context"
	"hotgo/internal/global"
	"hotgo/internal/library/location"
	"hotgo/internal/library/token"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/gogf/gf/v2/net/gtrace"
	"github.com/gogf/gf/v2/os/gctx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const traceIDHeader = "X-Trace-ID"

var (
	httpMetricsOnce     sync.Once
	httpRequestDuration metric.Float64Histogram
	httpRequestCount    metric.Int64Counter
)

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
	recordHTTPMetrics(r.Context(), r, duration)
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

func recordHTTPMetrics(ctx context.Context, r *ghttp.Request, duration time.Duration) {
	httpMetricsOnce.Do(func() {
		meter := otel.Meter("hotgo/http")
		httpRequestDuration, _ = meter.Float64Histogram(
			"xiaohuiji.http.server.duration",
			metric.WithDescription("HTTP server request duration"),
			metric.WithUnit("s"),
		)
		httpRequestCount, _ = meter.Int64Counter(
			"xiaohuiji.http.server.requests",
			metric.WithDescription("HTTP server request count"),
		)
	})
	route := r.URL.Path
	if matched := global.GetRequestRoute(r); matched != nil && matched.Route != "" {
		route = matched.Route
	}
	attributes := metric.WithAttributes(
		attribute.String("http_method", r.Method),
		attribute.String("http_route", route),
		attribute.Int("http_status_code", r.Response.Status),
	)
	if httpRequestDuration != nil {
		httpRequestDuration.Record(ctx, duration.Seconds(), attributes)
	}
	if httpRequestCount != nil {
		httpRequestCount.Add(ctx, 1, attributes)
	}
}
