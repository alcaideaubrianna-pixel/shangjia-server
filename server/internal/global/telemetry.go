package global

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"hotgo/internal/consts"
	"hotgo/utility/runrole"
	"hotgo/utility/simple"
)

func initOTel(ctx context.Context) error {
	return initOTelWithConfig(ctx, "telemetry.endpoint", "telemetry.secure", "telemetry.serviceName")
}

func initOTelWithConfig(ctx context.Context, endpointKey, secureKey, serviceKey string) error {
	endpoint := normalizeOTLPEndpoint(strings.TrimSpace(g.Cfg().MustGet(ctx, endpointKey).String()))
	if endpoint == "" {
		return fmt.Errorf("OTLP endpoint is empty")
	}
	secure := g.Cfg().MustGet(ctx, secureKey).Bool()
	serviceName := strings.TrimSpace(g.Cfg().MustGet(ctx, serviceKey, "xiaohuiji").String())
	if railwayServiceName := strings.TrimSpace(os.Getenv("RAILWAY_SERVICE_NAME")); railwayServiceName != "" {
		serviceName = railwayServiceName
	}
	if serviceName == "" {
		serviceName = "xiaohuiji"
	}
	hostName, _ := os.Hostname()
	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("service.name", serviceName),
		attribute.String("service.instance.id", hostName),
		attribute.String("service.version", consts.VersionApp),
		attribute.String("deployment.environment.name", gmodeName(ctx)),
		attribute.String("runtime.role", strings.Join(runrole.Roles(ctx), ",")),
		attribute.String("railway.service.name", os.Getenv("RAILWAY_SERVICE_NAME")),
		attribute.String("railway.deployment.id", os.Getenv("RAILWAY_DEPLOYMENT_ID")),
	))
	if err != nil {
		return fmt.Errorf("create resource: %w", err)
	}

	headers := otlpHeaders()
	traceOptions := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
	metricOptions := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(endpoint)}
	if len(headers) > 0 {
		traceOptions = append(traceOptions, otlptracegrpc.WithHeaders(headers))
		metricOptions = append(metricOptions, otlpmetricgrpc.WithHeaders(headers))
	}
	if !secure {
		traceOptions = append(traceOptions, otlptracegrpc.WithInsecure())
		metricOptions = append(metricOptions, otlpmetricgrpc.WithInsecure())
	}
	traceExporter, err := otlptracegrpc.New(ctx, traceOptions...)
	if err != nil {
		return fmt.Errorf("create trace exporter: %w", err)
	}
	metricExporter, err := otlpmetricgrpc.New(ctx, metricOptions...)
	if err != nil {
		_ = traceExporter.Shutdown(ctx)
		return fmt.Errorf("create metric exporter: %w", err)
	}

	ratio := g.Cfg().MustGet(ctx, "telemetry.sampleRatio", 0.15).Float64()
	if ratio <= 0 || ratio > 1 {
		ratio = 0.15
	}
	redisRatio := telemetryRatioFromEnv("OTEL_REDIS_SAMPLE_RATIO", 0.2)
	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(newTelemetrySampler(ratio, redisRatio)),
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
	)
	interval := time.Duration(g.Cfg().MustGet(ctx, "telemetry.metricsIntervalSeconds", 15).Int()) * time.Second
	if interval < 5*time.Second {
		interval = 15 * time.Second
	}
	metricProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter, sdkmetric.WithInterval(interval))),
		sdkmetric.WithResource(res),
	)
	otel.SetTracerProvider(traceProvider)
	otel.SetMeterProvider(metricProvider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	registerTelemetryShutdown(traceProvider, metricProvider)
	startRuntimeMetrics(ctx, metricProvider)
	g.Log().Infof(ctx, "通用 OpenTelemetry 已启用 endpoint:%s service:%s role:%s", endpoint, serviceName, strings.Join(runrole.Roles(ctx), ","))
	return nil
}

// telemetrySampler keeps the normal trace sampling policy while reducing the
// volume of high-frequency Redis client spans. Redis calls are still traced
// with the same trace context, but only 20% are exported by default. The
// ratio is controlled by OTEL_REDIS_SAMPLE_RATIO.
type telemetrySampler struct {
	parentBased sdktrace.Sampler
	redis       sdktrace.Sampler
}

func newTelemetrySampler(ratio, redisRatio float64) sdktrace.Sampler {
	return telemetrySampler{
		parentBased: sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio)),
		redis:       sdktrace.TraceIDRatioBased(redisRatio),
	}
}

func (s telemetrySampler) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	if isRedisSpan(p.Name) {
		return s.redis.ShouldSample(p)
	}
	return s.parentBased.ShouldSample(p)
}

func (s telemetrySampler) Description() string {
	return "parent-based trace sampling with Redis spans sampled at 20%"
}

func isRedisSpan(name string) bool {
	return strings.HasPrefix(name, "Redis.") || strings.HasPrefix(name, "redis.")
}

func telemetryRatioFromEnv(key string, fallback float64) float64 {
	ratio, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv(key)), 64)
	if err != nil || ratio <= 0 || ratio > 1 {
		return fallback
	}
	return ratio
}

func normalizeOTLPEndpoint(endpoint string) string {
	if endpoint == "" {
		return endpoint
	}
	if strings.Contains(endpoint, "://") {
		parsed, err := url.Parse(endpoint)
		if err == nil && parsed.Host != "" {
			endpoint = parsed.Host
		}
	}
	if !strings.Contains(endpoint, ":") {
		endpoint += ":443"
	}
	return endpoint
}

func otlpHeaders() map[string]string {
	result := make(map[string]string)
	for _, item := range strings.Split(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"), ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(item), "=")
		if ok && strings.TrimSpace(key) != "" {
			result[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return result
}

func registerTelemetryShutdown(traceProvider *sdktrace.TracerProvider, metricProvider *sdkmetric.MeterProvider) {
	simple.Event().Register(consts.EventServerClose, func(ctx context.Context, args ...interface{}) {
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		_ = traceProvider.Shutdown(shutdownCtx)
		_ = metricProvider.Shutdown(shutdownCtx)
	})
}

func startRuntimeMetrics(ctx context.Context, provider *sdkmetric.MeterProvider) {
	meter := provider.Meter("hotgo/runtime")
	processUp, _ := meter.Int64ObservableGauge("xiaohuiji.process.up")
	heartbeat, _ := meter.Int64ObservableGauge("xiaohuiji.runtime.heartbeat")
	goMetric, _ := meter.Int64ObservableGauge("xiaohuiji.go.goroutines")
	heapMetric, _ := meter.Int64ObservableGauge("xiaohuiji.go.heap_alloc_bytes")
	_, _ = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		observer.ObserveInt64(processUp, 1)
		observer.ObserveInt64(heartbeat, gtime.Now().Unix(), metric.WithAttributes(attribute.String("role", strings.Join(runrole.Roles(ctx), ","))))
		observer.ObserveInt64(goMetric, int64(runtime.NumGoroutine()))
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		observer.ObserveInt64(heapMetric, int64(mem.HeapAlloc))
		return nil
	}, processUp, heartbeat, goMetric, heapMetric)
}

func gmodeName(ctx context.Context) string {
	value := g.Cfg().MustGet(ctx, "system.mode").String()
	if value == "" {
		value = "unknown"
	}
	return value
}
