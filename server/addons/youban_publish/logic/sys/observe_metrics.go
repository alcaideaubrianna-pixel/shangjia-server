package sys

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var publishObserveMeter = otel.Meter("hotgo/addons/youban_publish")

func observePublishRuntimeHeartbeat(ctx context.Context, config publishRuntimeConfig) {
	roles := map[string]bool{
		"account":           config.Account,
		"scheduler":         config.Scheduler,
		"push-worker":       config.PushWorker,
		"media-worker":      config.MediaWorker,
		"background-worker": config.BackgroundWorker,
	}
	gauge, _ := publishObserveMeter.Int64Gauge("xiaohuiji.runtime.heartbeat")
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.runtime.heartbeats")
	for role, enabled := range roles {
		if !enabled {
			continue
		}
		attrs := metric.WithAttributes(attribute.String("role", role))
		gauge.Record(ctx, 1, attrs)
		counter.Add(ctx, 1, attrs)
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

func observeTelegramLease(ctx context.Context, action string, tgAccountId int64) {
	counter, _ := publishObserveMeter.Int64Counter("xiaohuiji.tg.account_lease_events")
	counter.Add(ctx, 1, metric.WithAttributes(attribute.String("action", action)))
	_ = tgAccountId
}

func observeTelegramLeaseActive(ctx context.Context, delta int64) {
	gauge, _ := publishObserveMeter.Int64UpDownCounter("xiaohuiji.tg.account_lease_active")
	gauge.Add(ctx, delta)
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
