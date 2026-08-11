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
