package sys

import (
	"context"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/collectobserve"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type collectDeliverySkipReason string

func normalizeCollectDeliverySkipReason(reason string) collectDeliverySkipReason {
	switch reason {
	case "global_disabled", "source_unavailable", "self_loop_publish_target",
		"approval_blocked", "channel_access_blocked":
		return collectDeliverySkipReason(reason)
	default:
		return "unknown"
	}
}

var collectDeliverySkipped = newCollectDeliverySkipCounter(publishObserveMeter)

func newCollectDeliverySkipCounter(meter metric.Meter) metric.Int64Counter {
	counter, _ := meter.Int64Counter("xiaohuiji.collect.delivery.skipped",
		metric.WithUnit("{delivery}"),
		metric.WithDescription("Publish adapter policy skip decisions, including repeated delivery attempts; not successful ingestion"))
	return counter
}

func recordCollectDeliverySkipped(ctx context.Context, counter metric.Int64Counter, delivery *collectorin.CollectorDelivery, reason string) {
	if counter == nil {
		return
	}
	meta := collectobserve.From(ctx)
	if delivery != nil {
		meta.SourceType = delivery.SourceType
		meta.Mode = collectobserve.ModeForHistory(delivery.History)
	}
	counter.Add(ctx, 1, metric.WithAttributes(
		attribute.String("reason", string(normalizeCollectDeliverySkipReason(reason))),
		attribute.String("source_type", collectobserve.SourceType(meta.SourceType)),
		attribute.String("collection_mode", collectobserve.Mode(meta.Mode))))
}
