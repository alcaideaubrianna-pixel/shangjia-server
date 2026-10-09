package sys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	botMediaSearchMetricsOnce sync.Once
	botMediaSearchStageCount  metric.Int64Counter
	botMediaSearchStageSum    metric.Float64Counter
	botMediaDownloadRequests  metric.Int64Counter
	botMediaDownloadDuration  metric.Float64Histogram
	botMediaDownloadBytes     metric.Int64Histogram
)

func initBotMediaSearchMetrics() {
	botMediaSearchMetricsOnce.Do(func() {
		meter := otel.Meter("hotgo/addons/youban_publish/media_search")
		botMediaSearchStageCount, _ = meter.Int64Counter("xiaohuiji.bot.scan.fingerprint.stage.duration.count")
		botMediaSearchStageSum, _ = meter.Float64Counter("xiaohuiji.bot.scan.fingerprint.stage.duration.sum", metric.WithUnit("s"))
		botMediaDownloadRequests, _ = meter.Int64Counter("xiaohuiji.bot.scan.image_fetch.requests")
		botMediaDownloadDuration, _ = meter.Float64Histogram("xiaohuiji.bot.scan.image_fetch.duration_seconds", metric.WithUnit("s"))
		botMediaDownloadBytes, _ = meter.Int64Histogram("xiaohuiji.bot.scan.image_fetch.bytes", metric.WithUnit("By"))
	})
}

func observeBotMediaDownload(ctx context.Context, source string, cacheHit bool, size int64, startedAt time.Time, err error) {
	initBotMediaSearchMetrics()
	attrs := metric.WithAttributes(
		attribute.String("source", source),
		attribute.Bool("cache_hit", cacheHit),
		attribute.String("result", botMediaDownloadResult(err)),
		attribute.String("error_type", botMediaDownloadErrorType(err)),
	)
	if botMediaDownloadRequests != nil {
		botMediaDownloadRequests.Add(ctx, 1, attrs)
	}
	if botMediaDownloadDuration != nil {
		botMediaDownloadDuration.Record(ctx, time.Since(startedAt).Seconds(), attrs)
	}
	if botMediaDownloadBytes != nil && size > 0 {
		botMediaDownloadBytes.Record(ctx, size, attrs)
	}
}

func botMediaDownloadResult(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "deadline exceeded") {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	return "error"
}

func botMediaDownloadErrorType(err error) string {
	if err == nil {
		return "none"
	}
	var statusErr *mediaFileCacheHTTPStatusError
	if errors.As(err, &statusErr) {
		return fmt.Sprintf("http_%d", statusErr.statusCode)
	}
	message := strings.ToLower(err.Error())
	for _, item := range []struct{ fragment, label string }{
		{"no such host", "dns"}, {"server misbehaving", "dns"},
		{"deadline exceeded", "timeout"}, {"i/o timeout", "timeout"},
		{"connection refused", "connection_refused"}, {"connection reset", "connection_reset"},
		{"unexpected eof", "unexpected_eof"},
	} {
		if strings.Contains(message, item.fragment) {
			return item.label
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	return "other"
}

func observeBotMediaSearchStage(ctx context.Context, stage string, startedAt time.Time, stageErr error) {
	initBotMediaSearchMetrics()
	result := "success"
	if stageErr != nil {
		result = "failed"
	}
	attrs := metric.WithAttributes(attribute.String("stage", stage), attribute.String("result", result))
	if botMediaSearchStageCount != nil {
		botMediaSearchStageCount.Add(ctx, 1, attrs)
	}
	if botMediaSearchStageSum != nil {
		botMediaSearchStageSum.Add(ctx, time.Since(startedAt).Seconds(), attrs)
	}
}
