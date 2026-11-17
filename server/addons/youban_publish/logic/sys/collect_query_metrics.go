package sys

import (
	"context"
	"errors"
	"net"
	"time"

	"hotgo/internal/library/collectobserve"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Instruments use the existing global meter, which delegates to the runtime
// MeterProvider. No reader, exporter, polling or database access belongs here.
type collectQueryInstruments struct {
	executions metric.Int64Counter
	duration   metric.Float64Histogram
	rows       metric.Int64Histogram
	reuse      metric.Int64Counter
}

var collectQueryMetrics = newCollectQueryInstruments(publishObserveMeter)

func newCollectQueryInstruments(meter metric.Meter) collectQueryInstruments {
	var instruments collectQueryInstruments
	instruments.executions, _ = meter.Int64Counter("xiaohuiji.collect.query.executions", metric.WithUnit("{query}"),
		metric.WithDescription("Window ORM query calls, including failures, classified by bounded query name and result."))
	instruments.duration, _ = meter.Float64Histogram("xiaohuiji.collect.query.duration_seconds", metric.WithUnit("s"),
		metric.WithDescription("Window ORM client round-trip duration in seconds, including connection waiting and result decoding, for all results."))
	instruments.rows, _ = meter.Int64Histogram("xiaohuiji.collect.query.returned_rows", metric.WithUnit("{row}"),
		metric.WithDescription("Rows returned by successful window ORM queries, including empty results; failed calls are excluded."))
	instruments.reuse, _ = meter.Int64Counter("xiaohuiji.collect.batch_reuse", metric.WithUnit("{decision}"),
		metric.WithDescription("Evaluated window batch reuse hits and misses; unevaluated decisions are excluded."))
	return instruments
}

func collectQueryContextAttributes(ctx context.Context) []attribute.KeyValue {
	meta := collectobserve.From(ctx)
	return []attribute.KeyValue{
		attribute.String("source_type", collectobserve.SourceType(meta.SourceType)),
		attribute.String("collection_mode", collectobserve.Mode(meta.Mode)),
	}
}

func collectQueryResult(err error) string {
	if err == nil {
		return "success"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
		return "timeout"
	}
	return "failed"
}

func (instruments collectQueryInstruments) recordQuery(ctx context.Context, kind collectWindowQueryKind, elapsed time.Duration, rows int, err error) {
	queryName := "unknown"
	switch kind {
	case collectWindowQueryPrimary:
		queryName = "window_primary"
	case collectWindowQueryRetry:
		queryName = "window_retry"
	}
	attrs := append(collectQueryContextAttributes(ctx), attribute.String("query_name", queryName),
		attribute.String("result", collectQueryResult(err)))
	options := metric.WithAttributes(attrs...)
	if instruments.executions != nil {
		instruments.executions.Add(ctx, 1, options)
	}
	if instruments.duration != nil && elapsed >= 0 {
		instruments.duration.Record(ctx, elapsed.Seconds(), options)
	}
	if instruments.rows != nil && err == nil && rows >= 0 {
		instruments.rows.Record(ctx, int64(rows), options)
	}
}

func (instruments collectQueryInstruments) recordReuse(ctx context.Context, outcome collectBatchReuseOutcome) {
	// Only evaluated outcomes enter the hit-rate denominator. Cache failures,
	// busy leases and invalid/terminal targets do not imply a reuse miss.
	if outcome != collectBatchReuseHit && outcome != collectBatchReuseMiss {
		return
	}
	if instruments.reuse != nil {
		attrs := append(collectQueryContextAttributes(ctx), attribute.String("reuse_result", string(outcome)))
		instruments.reuse.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}
