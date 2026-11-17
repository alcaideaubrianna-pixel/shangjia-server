package sys

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"hotgo/utility/dbtime"
)

// Recovery observations reuse bounded pages already loaded by the recovery
// worker. A successful scan is not a successful repair, and page size is not
// global backlog. No SQL or new polling loop belongs in this instrumentation.
type publishRecoveryInstruments struct {
	steps     metric.Int64Counter
	duration  metric.Float64Histogram
	items     metric.Int64Counter
	pageItems metric.Int64Gauge
	oldestAge metric.Float64Gauge
	updatedAt metric.Float64Gauge
}

func newPublishRecoveryInstruments(meter metric.Meter) publishRecoveryInstruments {
	steps, _ := meter.Int64Counter("xiaohuiji.publish.recovery.steps",
		metric.WithDescription("Recovery step invocations; success means the scan returned without error, not that every item recovered"))
	duration, _ := meter.Float64Histogram("xiaohuiji.publish.recovery.duration",
		metric.WithUnit("s"), metric.WithDescription("Execution time of one bounded recovery step"),
		metric.WithExplicitBucketBoundaries(.005, .01, .05, .1, .5, 1, 5, 10, 30, 60))
	items, _ := meter.Int64Counter("xiaohuiji.publish.recovery.items",
		metric.WithDescription("Per-item confirmed-job recovery attempts; retries count again and completion is not a unique job count"))
	pageItems, _ := meter.Int64Gauge("xiaohuiji.publish.recovery.page_items",
		metric.WithDescription("Rows in the last bounded confirmed-job page, not total backlog"))
	oldestAge, _ := meter.Float64Gauge("xiaohuiji.publish.recovery.page_oldest_age",
		metric.WithUnit("s"), metric.WithDescription("Oldest unchanged confirmed job in the last page; excludes query failures"))
	updatedAt, _ := meter.Float64Gauge("xiaohuiji.publish.observation.updated_at",
		metric.WithUnit("s"), metric.WithDescription("Unix time of the last successful bounded observation; check freshness before interpreting gauges"))
	return publishRecoveryInstruments{steps, duration, items, pageItems, oldestAge, updatedAt}
}

var publishRecoveryMetrics = newPublishRecoveryInstruments(publishObserveMeter)

func publishRecoveryStepLabel(step string) string {
	switch step {
	case "publish_cancel", "permission_backlog", "permission_notification",
		"expired_message_plan", "pre_attempt", "confirmed_job", "channel_queue_recovery",
		"stale_sending", "media_fallback", "expired_attempt", "sending_attempt",
		"terminal_attempt", "failed_display", "outbox_dispatch", "stale_dispatch",
		"missing_operation_state", "operation_state":
		return step
	default:
		return "unknown"
	}
}

func publishRecoveryOutcome(err error) string {
	switch {
	case err == nil:
		return "success"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	default:
		return "failed"
	}
}

func (m publishRecoveryInstruments) step(ctx context.Context, step string, elapsed time.Duration, err error) {
	attrs := metric.WithAttributes(attribute.String("step", publishRecoveryStepLabel(step)),
		attribute.String("outcome", publishRecoveryOutcome(err)))
	if m.steps != nil {
		m.steps.Add(ctx, 1, attrs)
	}
	if m.duration != nil && elapsed >= 0 {
		m.duration.Record(ctx, elapsed.Seconds(), attrs)
	}
}

func (m publishRecoveryInstruments) item(ctx context.Context, phase string, err error) {
	if phase != "repair_verify_purpose" && phase != "complete_job" {
		phase = "unknown"
	}
	if m.items != nil {
		m.items.Add(ctx, 1, metric.WithAttributes(attribute.String("step", "confirmed_job"),
			attribute.String("phase", phase), attribute.String("outcome", publishRecoveryOutcome(err))))
	}
}

func (m publishRecoveryInstruments) confirmedPage(ctx context.Context, jobs []telegramJobRecord, now time.Time) {
	attrs := metric.WithAttributes(attribute.String("kind", "confirmed_job_page"))
	if m.pageItems != nil {
		m.pageItems.Record(ctx, int64(len(jobs)), attrs)
	}
	var oldestAge float64
	for _, job := range jobs {
		if updated := dbtime.FromTimestamp(job.UpdatedAt); updated != nil && !updated.IsZero() {
			if age := now.Sub(updated.Time).Seconds(); age > oldestAge {
				oldestAge = age
			}
		}
	}
	if m.oldestAge != nil {
		m.oldestAge.Record(ctx, oldestAge, attrs)
	}
	if m.updatedAt != nil {
		m.updatedAt.Record(ctx, float64(now.Unix()), attrs)
	}
}
