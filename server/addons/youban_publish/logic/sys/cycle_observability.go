package sys

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	profileCycleDispatches, _ = publishObserveMeter.Int64Counter(
		"xiaohuiji.publish.cycle.dispatches",
		metric.WithDescription("Profile cycle scheduler decisions; one observation per due candidate"),
	)
	profileCycleDueScans, _ = publishObserveMeter.Int64Counter(
		"xiaohuiji.publish.cycle.due_scans",
		metric.WithDescription("Profile cycle due scan results"),
	)
)

func recordProfileCycleDispatch(ctx context.Context, outcome string) {
	if profileCycleDispatches == nil {
		return
	}
	profileCycleDispatches.Add(ctx, 1, metric.WithAttributes(
		attribute.String("outcome", outcome),
	))
}

func recordProfileCycleDueScan(ctx context.Context, outcome string, count int) {
	if profileCycleDueScans == nil {
		return
	}
	profileCycleDueScans.Add(ctx, int64(count), metric.WithAttributes(
		attribute.String("outcome", outcome),
	))
}
