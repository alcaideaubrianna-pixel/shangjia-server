package sys

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	scanMetricsOnce         sync.Once
	scanRequestCount        metric.Int64Counter
	scanStageDurationCount  metric.Int64Counter
	scanStageDurationSum    metric.Float64Counter
	scanStageDurationBucket metric.Int64Counter
	scanDirectLookupCount   metric.Int64Counter
	scanGetFileRequests     metric.Int64Counter
	scanGetFileDuration     metric.Float64Histogram
)

var scanDurationBuckets = []struct {
	seconds float64
	label   string
}{
	{0.1, "0.1"}, {0.25, "0.25"}, {0.5, "0.5"}, {1, "1"}, {2, "2"},
	{3, "3"}, {5, "5"}, {10, "10"}, {15, "15"}, {30, "30"}, {60, "60"},
}

func initScanMetrics() {
	scanMetricsOnce.Do(func() {
		meter := otel.Meter("hotgo/addons/youban_bot/scan")
		scanRequestCount, _ = meter.Int64Counter("xiaohuiji.bot.scan.requests")
		scanStageDurationCount, _ = meter.Int64Counter("xiaohuiji.bot.scan.stage.duration.count")
		scanStageDurationSum, _ = meter.Float64Counter("xiaohuiji.bot.scan.stage.duration.sum", metric.WithUnit("s"))
		scanStageDurationBucket, _ = meter.Int64Counter("xiaohuiji.bot.scan.stage.duration.bucket")
		scanDirectLookupCount, _ = meter.Int64Counter("xiaohuiji.bot.scan.direct_lookup")
		scanGetFileRequests, _ = meter.Int64Counter("xiaohuiji.bot.scan.telegram_get_file.requests")
		scanGetFileDuration, _ = meter.Float64Histogram("xiaohuiji.bot.scan.telegram_get_file.duration_seconds", metric.WithUnit("s"))
	})
}

func observeScanTelegramGetFile(ctx context.Context, botId int64, attempts int, startedAt time.Time, err error) {
	initScanMetrics()
	attrs := metric.WithAttributes(
		attribute.String("bot_id", strconv.FormatInt(botId, 10)),
		attribute.Int("attempts", attempts),
		attribute.String("result", scanTelegramRequestResult(err)),
		attribute.String("error_type", scanTelegramErrorType(err)),
	)
	if scanGetFileRequests != nil {
		scanGetFileRequests.Add(ctx, 1, attrs)
	}
	if scanGetFileDuration != nil {
		scanGetFileDuration.Record(ctx, time.Since(startedAt).Seconds(), attrs)
	}
}

func scanTelegramRequestResult(err error) string {
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

func scanTelegramErrorType(err error) string {
	if err == nil {
		return "none"
	}
	message := strings.ToLower(err.Error())
	for _, item := range []struct{ fragment, label string }{
		{"no such host", "dns"}, {"server misbehaving", "dns"},
		{"deadline exceeded", "timeout"}, {"i/o timeout", "timeout"},
		{"connection refused", "connection_refused"}, {"connection reset", "connection_reset"},
		{"unexpected eof", "unexpected_eof"}, {"bad request", "bad_request"},
	} {
		if strings.Contains(message, item.fragment) {
			return item.label
		}
	}
	return "other"
}

func observeScanRequest(ctx context.Context, botId int64, result string) {
	initScanMetrics()
	if scanRequestCount != nil {
		scanRequestCount.Add(ctx, 1, metric.WithAttributes(
			attribute.String("bot_id", strconv.FormatInt(botId, 10)),
			attribute.String("result", result),
		))
	}
}

func observeScanDirectLookup(ctx context.Context, botId int64, source string, hit bool) {
	initScanMetrics()
	if scanDirectLookupCount == nil {
		return
	}
	result := "miss"
	if hit {
		result = "hit"
	}
	scanDirectLookupCount.Add(ctx, 1, metric.WithAttributes(
		attribute.String("bot_id", strconv.FormatInt(botId, 10)),
		attribute.String("source", source),
		attribute.String("result", result),
	))
}

func observeScanStage(ctx context.Context, botId int64, stage string, startedAt time.Time, stageErr error) {
	initScanMetrics()
	duration := time.Since(startedAt).Seconds()
	result := "success"
	if stageErr != nil {
		result = "failed"
	}
	attrs := []attribute.KeyValue{
		attribute.String("bot_id", strconv.FormatInt(botId, 10)),
		attribute.String("stage", stage),
		attribute.String("result", result),
	}
	if scanStageDurationCount != nil {
		scanStageDurationCount.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
	if scanStageDurationSum != nil {
		scanStageDurationSum.Add(ctx, duration, metric.WithAttributes(attrs...))
	}
	if scanStageDurationBucket != nil {
		for _, bucket := range scanDurationBuckets {
			if duration > bucket.seconds {
				continue
			}
			bucketAttrs := append(append([]attribute.KeyValue{}, attrs...), attribute.String("le", bucket.label))
			scanStageDurationBucket.Add(ctx, 1, metric.WithAttributes(bucketAttrs...))
		}
	}
}
