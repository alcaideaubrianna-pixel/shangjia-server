package sys

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

func observeTelegramAPI(ctx context.Context, operation, uploadMode, result string, duration time.Duration, statusCode int) {
	attrs := []attribute.KeyValue{attribute.String("operation", operation), attribute.String("upload_mode", uploadMode), attribute.String("result", result)}
	if statusCode > 0 {
		attrs = append(attrs, attribute.Int64("status_code", int64(statusCode)))
	}
	metricAttrs := metric.WithAttributes(attrs...)
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.api.requests")
	durations, _ := publishObserveMeter.Float64Histogram("xiaohuiji.tg.api.duration_seconds")
	counter.Add(ctx, 1, metricAttrs)
	durations.Record(ctx, duration.Seconds(), metricAttrs)
}

// telegramAPIStatusCode extracts the Telegram Bot API error code for metrics.
// A successful SDK call is represented as HTTP 200; transport failures have no
// Telegram response and are intentionally left without a numeric status code.
func telegramAPIStatusCode(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if code := telegramSendErrorCode(err); code != "" {
		var statusCode int
		if _, scanErr := fmt.Sscanf(code, "%d", &statusCode); scanErr == nil {
			return statusCode
		}
	}
	return 0
}

func observeRedisLua(ctx context.Context, scriptName, result string, duration time.Duration) {
	attrs := metric.WithAttributes(
		attribute.String("script", scriptName),
		attribute.String("result", result),
	)
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.redis.lua.calls")
	durations, _ := publishObserveMeter.Float64Histogram("xiaohuiji.redis.lua.duration_ms")
	counter.Add(ctx, 1, attrs)
	durations.Record(ctx, float64(duration.Milliseconds()), attrs)
	if duration >= 100*time.Millisecond {
		// This duration covers the client call (pool/network/server queue included),
		// not the server-side Lua execution time measured by Redis SLOWLOG.
		g.Log().Warning(ctx, "Redis 调用耗时超过100ms", g.Map{
			// Keep the event/metric names for existing dashboards and alert rules.
			"event":                   "redis_lua_slow",
			"measurement":             "client_round_trip",
			"serverExecutionMeasured": false,
			"script":                  scriptName,
			"result":                  result,
			"durationMs":              duration.Milliseconds(),
		})
	}
}

func observeTelegramBotGetFile(ctx context.Context, endpoint, result string, duration time.Duration) {
	attrs := metric.WithAttributes(attribute.String("endpoint", endpoint), attribute.String("result", result))
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.bot_api.get_file_requests")
	histogram, _ := publishObserveMeter.Float64Histogram("xiaohuiji.tg.bot_api.get_file_duration_seconds")
	counter.Add(ctx, 1, attrs)
	histogram.Record(ctx, duration.Seconds(), attrs)
}

func observeTelegramBotFileDownload(ctx context.Context, endpoint string, err error, duration time.Duration) {
	result, statusCode := "success", "2xx"
	if err != nil {
		result, statusCode = "error", "transport"
		var statusErr *mediaFileCacheHTTPStatusError
		if errors.As(err, &statusErr) {
			statusCode = http.StatusText(statusErr.statusCode)
			if statusCode == "" {
				statusCode = "other"
			} else {
				statusCode = fmt.Sprintf("%d", statusErr.statusCode)
			}
		}
	}
	attrs := metric.WithAttributes(attribute.String("endpoint", endpoint), attribute.String("result", result), attribute.String("status_code", statusCode))
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.bot_api.file_download_requests")
	histogram, _ := publishObserveMeter.Float64Histogram("xiaohuiji.tg.bot_api.file_download_duration_seconds")
	counter.Add(ctx, 1, attrs)
	histogram.Record(ctx, duration.Seconds(), attrs)
}

var publishObserveMeter = otel.Meter("hotgo/addons/youban_publish")

// Keep publishing heartbeats separate from the global runtime heartbeat, whose
// value is a Unix timestamp. Reusing that name for a constant 1 made freshness
// queries ambiguous after the two instrumentation scopes were exported.
var publishRuntimeLastSeen, _ = publishObserveMeter.Int64Gauge("xiaohuiji.publish.runtime.last_seen",
	metric.WithUnit("s"), metric.WithDescription("Unix timestamp of the last publish-runtime heartbeat by enabled role"))
var publishRuntimeHeartbeats, _ = publishObserveMeter.Int64Counter("xiaohuiji.publish.runtime.heartbeats",
	metric.WithDescription("Publish-runtime heartbeat observations; these do not imply queue progress"))

func observeDurableLogWriteFailure(ctx context.Context, logType string) {
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.durable_log.write_failures")
	counter.Add(ctx, 1, metric.WithAttributes(attribute.String("log_type", logType)))
}

func observePublishRuntimeHeartbeat(ctx context.Context, config publishRuntimeConfig) {
	roles := map[string]bool{
		"account":           config.Account,
		"scheduler":         config.Scheduler,
		"push-worker":       config.PushWorker,
		"media-worker":      config.MediaWorker,
		"background-worker": config.BackgroundWorker,
	}
	for role, enabled := range roles {
		if !enabled {
			continue
		}
		attrs := metric.WithAttributes(attribute.String("role", role))
		if publishRuntimeLastSeen != nil {
			publishRuntimeLastSeen.Record(ctx, time.Now().Unix(), attrs)
		}
		if publishRuntimeHeartbeats != nil {
			publishRuntimeHeartbeats.Add(ctx, 1, attrs)
		}
	}
}

func observeRecoveryRun(ctx context.Context, step string, startedAt time.Time, scanned int, err error) {
	result := "success"
	if err != nil {
		result = "failed"
	}
	attrs := metric.WithAttributes(attribute.String("step", step), attribute.String("result", result))
	runs, _ := publishObserveMeter.Int64Counter("xiaohuiji.recovery.runs")
	duration, _ := publishObserveMeter.Float64Histogram("xiaohuiji.recovery.duration_ms")
	items, _ := publishObserveMeter.Int64Counter("xiaohuiji.recovery.scanned_items")
	runs.Add(ctx, 1, attrs)
	duration.Record(ctx, float64(time.Since(startedAt).Milliseconds()), attrs)
	if scanned > 0 {
		items.Add(ctx, int64(scanned), metric.WithAttributes(attribute.String("step", step)))
	}
}

func observeAntiScanMattingTask(ctx context.Context, provider string, startedAt time.Time, err error) {
	result := "success"
	if err != nil {
		result = "failed"
	}
	attrs := metric.WithAttributes(
		attribute.String("provider", provider),
		attribute.String("result", result),
	)
	tasks, _ := publishObserveMeter.Int64Counter("xiaohuiji.anti_scan.matting.tasks")
	duration, _ := publishObserveMeter.Float64Histogram("xiaohuiji.anti_scan.matting.duration_seconds")
	tasks.Add(ctx, 1, attrs)
	duration.Record(ctx, time.Since(startedAt).Seconds(), attrs)
}

func observeTelegramLease(ctx context.Context, action string, tgAccountId int64) {
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.account_lease_events")
	counter.Add(ctx, 1, metric.WithAttributes(attribute.String("action", action)))
	_ = tgAccountId
}

func observeTelegramLeaseActive(ctx context.Context, delta int64) {
	gauge, _ := publishObserveMeter.Int64UpDownCounter("xiaohuiji.tg.account_lease_active")
	gauge.Add(ctx, delta)
}

func observeTelegramMediaPurposeViolation(ctx context.Context, stage string) {
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.media_purpose_violations")
	counter.Add(ctx, 1, metric.WithAttributes(attribute.String("stage", stage)))
}

func observeTelegramPublishFailure(ctx context.Context, stage string) {
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.publish_failures")
	counter.Add(ctx, 1, metric.WithAttributes(attribute.String("stage", stage)))
}

func observeTelegramProfilePublish(ctx context.Context, job telegramJobRecord, result string, duration time.Duration, publishErr error) {
	source := "auto"
	operationNo := strings.ToLower(strings.TrimSpace(job.OperationNo))
	if strings.HasPrefix(operationNo, "bot_publish:") {
		source = "bot"
	} else if strings.HasPrefix(operationNo, "profile:") || strings.HasPrefix(operationNo, "edit:") || strings.HasPrefix(operationNo, "batchtext:") {
		source = "web"
	}
	errorType := "none"
	if publishErr != nil {
		errorType = telegramPublishErrorType(publishErr)
	}
	attrs := []attribute.KeyValue{attribute.String("result", result), attribute.String("source", source), attribute.String("error_type", errorType)}
	if job.ChannelId > 0 {
		attrs = append(attrs, attribute.Int64("channel_id", job.ChannelId))
	}
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.profile_publish_total")
	durations, _ := publishObserveMeter.Float64Histogram("xiaohuiji.tg.profile_publish_duration_seconds")
	metricAttrs := metric.WithAttributes(attrs...)
	counter.Add(ctx, 1, metricAttrs)
	durations.Record(ctx, duration.Seconds(), metricAttrs)
}

func telegramPublishErrorType(err error) string {
	if err == nil {
		return "none"
	}
	if code := telegramSendErrorCode(err); code != "" {
		return "telegram_" + code
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "deadline exceeded") || strings.Contains(message, "timeout") {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	return "transport_or_unknown"
}

func observeTelegramProfilePublishQueueWait(ctx context.Context, job telegramJobRecord, wait time.Duration) {
	source := "auto"
	operationNo := strings.ToLower(strings.TrimSpace(job.OperationNo))
	if strings.HasPrefix(operationNo, "bot_publish:") {
		source = "bot"
	} else if strings.HasPrefix(operationNo, "profile:") || strings.HasPrefix(operationNo, "edit:") || strings.HasPrefix(operationNo, "batchtext:") {
		source = "web"
	}
	attrs := []attribute.KeyValue{attribute.String("source", source)}
	if job.ChannelId > 0 {
		attrs = append(attrs, attribute.Int64("channel_id", job.ChannelId))
	}
	histogram, _ := publishObserveMeter.Float64Histogram("xiaohuiji.tg.profile_publish_queue_wait_seconds")
	histogram.Record(ctx, wait.Seconds(), metric.WithAttributes(attrs...))
}

func observeTelegramProfileUnpublish(ctx context.Context, operationNo, result string, duration time.Duration, publishErr error) {
	attrs := []attribute.KeyValue{
		attribute.String("source", telegramProfileOperationSource(operationNo)),
		attribute.String("result", result),
		attribute.String("error_type", telegramPublishErrorType(publishErr)),
	}
	metricAttrs := metric.WithAttributes(attrs...)
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.profile_unpublish_total")
	durations, _ := publishObserveMeter.Float64Histogram("xiaohuiji.tg.profile_unpublish_duration_seconds")
	counter.Add(ctx, 1, metricAttrs)
	durations.Record(ctx, duration.Seconds(), metricAttrs)
}

func telegramProfileOperationSource(operationNo string) string {
	value := strings.ToLower(strings.TrimSpace(operationNo))
	if strings.HasPrefix(value, "bot_publish:") || strings.HasPrefix(value, "bot_unpublish:") || strings.HasPrefix(value, "bot_delete:") {
		return "bot"
	}
	if strings.HasPrefix(value, "profile:") || strings.HasPrefix(value, "edit:") || strings.HasPrefix(value, "batchtext:") || strings.HasPrefix(value, "unpublish:") {
		return "web"
	}
	return "auto"
}

func observeTelegramMediaSend(ctx context.Context, job *telegramJobRecord, phase string, err error, duration time.Duration) {
	result := "success"
	if err != nil {
		result = "error"
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "deadline exceeded") {
			result = "timeout"
		} else if errors.Is(err, context.Canceled) {
			result = "canceled"
		}
	}
	attrs := []attribute.KeyValue{attribute.String("phase", phase), attribute.String("result", result)}
	if job != nil {
		if job.ChannelId > 0 {
			attrs = append(attrs, attribute.Int64("channel_id", job.ChannelId))
		}
	}
	metricAttrs := metric.WithAttributes(attrs...)
	requests, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.media_send_requests")
	durations, _ := publishObserveMeter.Float64Histogram("xiaohuiji.tg.media_send_duration_seconds")
	requests.Add(ctx, 1, metricAttrs)
	durations.Record(ctx, duration.Seconds(), metricAttrs)
}

func observeTelegramLateDelivery(ctx context.Context, phase, action string, count int) {
	attrs := metric.WithAttributes(attribute.String("phase", phase), attribute.String("action", action))
	events, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.late_delivery_events")
	messages, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.late_delivery_messages")
	events.Add(ctx, 1, attrs)
	if count > 0 {
		messages.Add(ctx, int64(count), attrs)
	}
}

func observeMediaFileCacheSource(ctx context.Context, source, result, errorClass string) {
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.media.download_source_events")
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("source", source),
		attribute.String("result", result),
		attribute.String("error_class", errorClass),
	))
}

func observeCollectMediaScheduler(ctx context.Context, action string, tenantID, tgAccountID int64) {
	attrs := metric.WithAttributes(
		attribute.String("action", action),
		attribute.Int64("tenant_id", tenantID),
		attribute.Int64("tg_account_id", tgAccountID),
	)
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.collect.media_scheduler_events")
	counter.Add(ctx, 1, attrs)
}

// observeCollectMediaOwnerWait records queue-side contention separately from
// download time. Source/account dimensions make a hot account or source
// visible without retaining per-event state in memory.
func observeCollectMediaOwnerWait(ctx context.Context, tenantID, tgAccountID, sourceID int64, wait time.Duration, result string) {
	attrs := metric.WithAttributes(
		attribute.Int64("tenant_id", tenantID),
		attribute.Int64("tg_account_id", tgAccountID),
		attribute.Int64("source_id", sourceID),
		attribute.String("result", result),
	)
	histogram, _ := publishObserveMeter.Float64Histogram("xiaohuiji.collect.media_owner_wait_seconds")
	histogram.Record(ctx, wait.Seconds(), attrs)
}

func observeCollectMediaEnqueue(ctx context.Context, reason, result string) {
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.collect.media_queue_enqueues")
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("reason", reason),
		attribute.String("result", result),
	))
}

func observeTelegramPublishRecovery(ctx context.Context, result string) {
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.publish_recoveries")
	counter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func observeTelegramReconcile(ctx context.Context, result string) {
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.reconcile_events")
	counter.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func observeTelegramDeleteFallback(ctx context.Context, result string, tgAccountID int64, value int64) {
	attrs := metric.WithAttributes(
		attribute.String("result", result),
		attribute.Int64("tg_account_id", tgAccountID),
	)
	events, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.delete_fallback_events")
	events.Add(ctx, 1, attrs)
	if value > 0 {
		messages, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.delete_fallback_messages")
		messages.Add(ctx, value, attrs)
	}
}

func observeTelegramDeleteFallbackWait(ctx context.Context, kind string, tgAccountID int64, wait time.Duration) {
	attrs := metric.WithAttributes(
		attribute.String("kind", kind),
		attribute.Int64("tg_account_id", tgAccountID),
	)
	histogram, _ := publishObserveMeter.Float64Histogram("xiaohuiji.tg.delete_fallback_wait_seconds")
	histogram.Record(ctx, wait.Seconds(), attrs)
}

func observeTelegramProfileDeleteDuration(ctx context.Context, job telegramJobRecord, result string, duration time.Duration) {
	attrs := []attribute.KeyValue{attribute.String("result", result)}
	if job.ChannelId > 0 {
		attrs = append(attrs, attribute.Int64("channel_id", job.ChannelId))
	}
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.profile_delete_total")
	durations, _ := publishObserveMeter.Float64Histogram("xiaohuiji.tg.profile_delete_duration_seconds")
	metricAttrs := metric.WithAttributes(attrs...)
	counter.Add(ctx, 1, metricAttrs)
	durations.Record(ctx, duration.Seconds(), metricAttrs)
}

func observeCollectHistoryBackpressure(ctx context.Context, sourceID int64, stats collectHistoryPendingStats, limit int) {
	attrs := metric.WithAttributes(attribute.Int64("source_id", sourceID))
	pending, _ := publishObserveMeter.Int64Gauge("xiaohuiji.collect.history_pending")
	pending.Record(ctx, int64(stats.Total), attrs)
	threshold, _ := publishObserveMeter.Int64Gauge("xiaohuiji.collect.history_pending_limit")
	threshold.Record(ctx, int64(limit), attrs)
	for status, count := range stats.ByStatus {
		statusAttrs := metric.WithAttributes(attribute.Int64("source_id", sourceID), attribute.String("status", status))
		gauge, _ := publishObserveMeter.Int64Gauge("xiaohuiji.collect.history_pending_by_status")
		gauge.Record(ctx, int64(count), statusAttrs)
	}
}

func observeCollectHistoryPage(ctx context.Context, sourceID int64, fetched int, offsetID int, duration time.Duration) {
	attrs := metric.WithAttributes(attribute.Int64("source_id", sourceID))
	pages, _ := publishObserveMeter.Int64Counter("xiaohuiji.collect.history_pages")
	pages.Add(ctx, 1, attrs)
	messages, _ := publishObserveMeter.Int64Counter("xiaohuiji.collect.history_messages")
	messages.Add(ctx, int64(fetched), attrs)
	offset, _ := publishObserveMeter.Int64Gauge("xiaohuiji.collect.history_offset")
	offset.Record(ctx, int64(offsetID), attrs)
	pageDuration, _ := publishObserveMeter.Float64Histogram("xiaohuiji.collect.history_page_duration_ms")
	pageDuration.Record(ctx, float64(duration.Milliseconds()), attrs)
}
