package sys

import (
	"context"
	"time"

	"hotgo/internal/library/collectobserve"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// A process queue sample measures producer submission to consumer receipt,
// including scheduled delay and Asynq retries. It does not measure pairing or
// media wait, and it must never substitute Telegram time or DB wall-clock time.
type collectProcessQueueSample string

const (
	collectProcessQueueSampleValid   collectProcessQueueSample = "valid"
	collectProcessQueueSampleMissing collectProcessQueueSample = "missing"
	collectProcessQueueSampleFuture  collectProcessQueueSample = "future"
)

type collectProcessQueueInstruments struct {
	wait    metric.Float64Histogram
	samples metric.Int64Counter
}

var collectProcessQueueMetrics = newCollectProcessQueueInstruments(publishObserveMeter)

func newCollectProcessQueueInstruments(meter metric.Meter) collectProcessQueueInstruments {
	wait, _ := meter.Float64Histogram("xiaohuiji.collect.process.queue.wait_seconds",
		metric.WithUnit("s"),
		metric.WithDescription("Producer submission to process consumer receipt; includes scheduled delay and retries"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600, 21600, 86400))
	samples, _ := meter.Int64Counter("xiaohuiji.collect.process.queue.wait_samples",
		metric.WithUnit("{sample}"),
		metric.WithDescription("Process queue clock sample availability; missing and future samples are not zero-duration waits"))
	return collectProcessQueueInstruments{wait: wait, samples: samples}
}

func (instruments collectProcessQueueInstruments) record(ctx context.Context, enqueuedUnixNano int64, consumedAt time.Time) {
	sample := collectProcessQueueSampleMissing
	var elapsed time.Duration
	if enqueuedUnixNano > 0 {
		elapsed = consumedAt.Sub(time.Unix(0, enqueuedUnixNano))
		sample = collectProcessQueueSampleValid
		if elapsed < 0 {
			sample = collectProcessQueueSampleFuture
		}
	}
	meta := collectobserve.From(ctx)
	attrs := []attribute.KeyValue{
		attribute.String("source_type", collectobserve.SourceType(meta.SourceType)),
		attribute.String("collection_mode", collectobserve.Mode(meta.Mode)),
	}
	if instruments.samples != nil {
		instruments.samples.Add(ctx, 1, metric.WithAttributes(append(attrs, attribute.String("sample", string(sample)))...))
	}
	if sample == collectProcessQueueSampleValid && instruments.wait != nil {
		instruments.wait.Record(ctx, elapsed.Seconds(), metric.WithAttributes(attrs...))
	}
}

func restoreCollectProcessObservation(ctx context.Context, payload collectProcessQueuePayload, consumedAt time.Time) context.Context {
	parent := ctx
	// An old/Unique payload is a fresh execution with no trusted observation.
	// Clear any worker-parent event clock rather than inheriting its age.
	ctx = (collectobserve.Carrier{}).RestoreAt(ctx, consumedAt)
	// Source-wake tasks process a batch, so no one event's ingress clock is
	// applicable. Each event will bind its own identity at its business stage.
	if payload.EventId > 0 && len(payload.Observation) > 0 {
		restored, eventID, err := collectobserve.DecodeTask(parent, payload.Observation, consumedAt)
		if err == nil && eventID == payload.EventId {
			ctx = restored
		}
	}
	return collectobserve.With(ctx, collectobserve.Metadata{
		TenantID: payload.TenantId, AccountID: payload.AccountId, SourceID: payload.SourceId,
		EventID: payload.EventId, RuleID: payload.RuleId, ChatID: payload.SourceChatId,
		Mode: collectobserve.ModeForHistory(payload.History),
	})
}

func captureCollectProcessObservation(ctx context.Context, payload collectProcessQueuePayload, unique bool, enqueuedAt time.Time) collectProcessQueuePayload {
	// Requeueing captures this submission time, not the previous queue wait.
	payload.Observation = nil
	if payload.EventId <= 0 || unique && payload.OutboxOwner == "" {
		return payload
	}
	observationCtx := collectobserve.With(ctx, collectobserve.Metadata{
		TenantID: payload.TenantId, AccountID: payload.AccountId,
		SourceID: payload.SourceId, EventID: payload.EventId, RuleID: payload.RuleId,
		ChatID: payload.SourceChatId, Mode: collectobserve.ModeForHistory(payload.History),
	})
	// Observability cannot turn a valid business submission into a failure.
	payload.Observation, _ = collectobserve.EncodeTask(observationCtx, payload.EventId, enqueuedAt)
	return payload
}
