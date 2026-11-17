package sys

import (
	"context"
	"time"

	"hotgo/internal/library/collectobserve"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type collectMediaInstruments struct {
	reuses    metric.Int64Counter
	downloads metric.Int64Counter
	duration  metric.Float64Histogram
	delegated metric.Int64Counter
}

var collectMediaMetrics = newCollectMediaInstruments(publishObserveMeter)

func newCollectMediaInstruments(meter metric.Meter) collectMediaInstruments {
	reuses, _ := meter.Int64Counter("xiaohuiji.collect.media_reuses", metric.WithUnit("{media}"),
		metric.WithDescription("Successful per-media reuse branches by layer; fingerprint_storage saves storage work and does not imply an avoided download. Excludes profile reuse."))
	downloads, _ := meter.Int64Counter("xiaohuiji.collect.media_download_attempts", metric.WithUnit("{attempt}"),
		metric.WithDescription("Actual Bot network download calls completed by result and media-task initial/retry context; prior task attempts may fail before network transfer. Excludes cache hits, getFile, slot waits and delegated account tasks."))
	duration, _ := meter.Float64Histogram("xiaohuiji.collect.media_download_duration_seconds", metric.WithUnit("s"),
		metric.WithDescription("Actual Bot network download call duration in seconds, including failed attempts; excludes queue, getFile, fingerprint and upload time."),
		metric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600))
	delegated, _ := meter.Int64Counter("xiaohuiji.collect.media_delegated_results", metric.WithUnit("{operation}"),
		metric.WithDescription("Account SDK SubmitAndWait operation results; success may reuse SDK cache or existing tasks and is not an actual network download count."))
	return collectMediaInstruments{reuses: reuses, downloads: downloads, duration: duration, delegated: delegated}
}

func collectMediaObserveSource(source string) string {
	switch source {
	case "bot", "account":
		return source
	default:
		return "unknown"
	}
}

// Reuse is counted per media, at the successful branch only. fingerprint_storage
// saves an upload after downloading; it does not represent an avoided download.
func (m collectMediaInstruments) reuse(ctx context.Context, source, layer string) {
	switch layer {
	case "media_row", "message_sibling", "identity", "remote_file", "fingerprint_storage":
	default:
		layer = "unknown"
	}
	m.reuses.Add(ctx, 1, metric.WithAttributes(
		attribute.String("source_type", collectMediaObserveSource(source)),
		attribute.String("collection_mode", collectobserve.Mode(collectobserve.From(ctx).Mode)),
		attribute.String("reuse_layer", layer),
	))
}

// Called only around the actual network downloader. A failed attempt followed
// by a successful retry produces two samples, not two successful media.
// Retry describes the existing media-task attempt context: prior task attempts
// can fail during getFile before any network download starts.
func (m collectMediaInstruments) download(ctx context.Context, retry bool, err error, elapsed time.Duration) {
	result, attempt := "success", "initial"
	if err != nil {
		result = "error"
	}
	if retry {
		attempt = "retry"
	}
	options := metric.WithAttributes(attribute.String("source_type", "bot"),
		attribute.String("collection_mode", collectobserve.Mode(collectobserve.From(ctx).Mode)),
		attribute.String("result", result), attribute.String("attempt", attempt))
	m.downloads.Add(ctx, 1, options)
	if elapsed >= 0 {
		m.duration.Record(ctx, elapsed.Seconds(), options)
	}
}

// SDK task completion is not evidence of a network download. In particular,
// fairness deferrals and SDK cache hits must not inflate actual downloads.
func (m collectMediaInstruments) delegate(ctx context.Context, err error) {
	result := "success"
	if err != nil {
		result = "error"
		if collectMediaRetryErrorFrom(err) != nil {
			result = "deferred"
		}
	}
	m.delegated.Add(ctx, 1, metric.WithAttributes(
		attribute.String("source_type", "account"),
		attribute.String("collection_mode", collectobserve.Mode(collectobserve.From(ctx).Mode)),
		attribute.String("result", result)))
}

type collectMediaRetryObservationKey struct{}

func collectMediaRetryObservationContext(ctx context.Context, retry bool) context.Context {
	return context.WithValue(ctx, collectMediaRetryObservationKey{}, retry)
}
