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
	httpMetricsOnce           sync.Once
	httpRequestCount          metric.Int64Counter
	httpRequestDurationCount  metric.Int64Counter
	httpRequestDurationSum    metric.Float64Counter
	httpRequestDurationBucket metric.Int64Counter
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
		httpRequestCount, _ = meter.Int64Counter(
			"xiaohuiji.http.server.requests",
			metric.WithDescription("HTTP server request count"),
		)
		httpRequestDurationCount, _ = meter.Int64Counter(
			"xiaohuiji.http.server.duration.count",
			metric.WithDescription("HTTP server request duration observation count"),
		)
		httpRequestDurationSum, _ = meter.Float64Counter(
			"xiaohuiji.http.server.duration.sum",
			metric.WithDescription("HTTP server request duration sum"),
			metric.WithUnit("s"),
		)
		httpRequestDurationBucket, _ = meter.Int64Counter(
			"xiaohuiji.http.server.duration.bucket",
			metric.WithDescription("Cumulative HTTP server request duration buckets"),
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
	if httpRequestCount != nil {
		httpRequestCount.Add(ctx, 1, attributes)
	}
	if httpRequestDurationCount != nil {
		httpRequestDurationCount.Add(ctx, 1, attributes)
	}
	if httpRequestDurationSum != nil {
		httpRequestDurationSum.Add(ctx, duration.Seconds(), attributes)
	}
	if httpRequestDurationBucket != nil {
		for _, bucket := range httpDurationBuckets {
			if duration.Seconds() > bucket.seconds {
				continue
			}
			httpRequestDurationBucket.Add(ctx, 1, metric.WithAttributes(
				attribute.String("http_method", r.Method),
				attribute.String("http_route", route),
				attribute.Int("http_status_code", r.Response.Status),
				attribute.String("le", bucket.label),
			))
		}
	}
}

type httpDurationBucket struct {
	seconds float64
	label   string
}

var httpDurationBuckets = []httpDurationBucket{
	{seconds: 0.1, label: "0.1"},
	{seconds: 0.25, label: "0.25"},
	{seconds: 0.5, label: "0.5"},
	{seconds: 1, label: "1"},
	{seconds: 2, label: "2"},
	{seconds: 5, label: "5"},
	{seconds: 10, label: "10"},
	{seconds: 1<<63 - 1, label: "+Inf"},
}
