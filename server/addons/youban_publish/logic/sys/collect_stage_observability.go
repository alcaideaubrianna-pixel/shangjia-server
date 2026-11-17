package sys

import (
	"context"
	"sync/atomic"
	"time"

	"hotgo/internal/library/collectobserve"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/gtrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type collectPipelineInstruments struct {
	count             metric.Int64Counter
	duration          metric.Float64Histogram
	profilesPersisted metric.Int64Counter
	lifecycleDuration metric.Float64Histogram
	lifecycleMissing  metric.Int64Counter
}

type collectPipelineErrorKind string
type collectPipelineWaitReason string

const (
	collectErrorNone      collectPipelineErrorKind = "none"
	collectErrorCanceled  collectPipelineErrorKind = "canceled"
	collectErrorDeadline  collectPipelineErrorKind = "deadline_exceeded"
	collectErrorTimeout   collectPipelineErrorKind = "timeout"
	collectErrorRetryable collectPipelineErrorKind = "retryable"
	collectErrorDuplicate collectPipelineErrorKind = "duplicate"
	collectErrorConflict  collectPipelineErrorKind = "conflict"
	collectErrorOther     collectPipelineErrorKind = "other"
	collectErrorUnknown   collectPipelineErrorKind = "unknown"

	collectWaitNone         collectPipelineWaitReason = "none"
	collectWaitUnspecified  collectPipelineWaitReason = "unspecified"
	collectWaitUnknown      collectPipelineWaitReason = "unknown"
	collectWaitRescheduled  collectPipelineWaitReason = "rescheduled"
	collectWaitPair         collectPipelineWaitReason = "pending_pair"
	collectWaitMediaCache   collectPipelineWaitReason = "media_cache_pending"
	collectWaitVerifyMedia  collectPipelineWaitReason = "verify_media_pending"
	collectWaitMedia        collectPipelineWaitReason = "media_incomplete"
	collectWaitLease        collectPipelineWaitReason = "lease_busy"
	collectWaitLock         collectPipelineWaitReason = "lock_busy"
	collectWaitSourcePaused collectPipelineWaitReason = "source_paused"
	collectWaitQuota        collectPipelineWaitReason = "media_quota"
	collectWaitVerify       collectPipelineWaitReason = "waiting_verify"
	collectWaitRateLimit    collectPipelineWaitReason = "rate_limited"
	collectWaitBackoff      collectPipelineWaitReason = "backoff"
	collectWaitError        collectPipelineWaitReason = "error"
)

type collectPipelineObservation struct {
	result     collectPipelineResult
	errorKind  collectPipelineErrorKind
	waitReason collectPipelineWaitReason
}

func normalizeCollectPipelineWaitReason(value string) collectPipelineWaitReason {
	switch value {
	case "none":
		return collectWaitNone
	case "unspecified", "":
		return collectWaitUnspecified
	case "rescheduled":
		return collectWaitRescheduled
	case "pending_pair":
		return collectWaitPair
	case "media_cache_pending":
		return collectWaitMediaCache
	case "verify_media_pending":
		return collectWaitVerifyMedia
	case "media_incomplete":
		return collectWaitMedia
	case "lease_busy":
		return collectWaitLease
	case "lock_busy":
		return collectWaitLock
	case "source_paused":
		return collectWaitSourcePaused
	case "media_quota":
		return collectWaitQuota
	case "waiting_verify":
		return collectWaitVerify
	case "rate_limited":
		return collectWaitRateLimit
	case "backoff":
		return collectWaitBackoff
	case "error":
		return collectWaitError
	default:
		return collectWaitUnknown
	}
}

// Classification affects telemetry only. A nil error preserves the explicit
// caller outcome, never fabricates a queue receipt or a final business success.
// Context cancellation after a confirmed receipt must not undo that receipt.
func classifyCollectPipelineObservation(result collectPipelineResult, err error, attrs ...attribute.KeyValue) collectPipelineObservation {
	outcome := collectPipelineObservation{result: result, errorKind: collectErrorNone, waitReason: collectWaitNone}
	if err != nil {
		switch collectPipelineErrorType(err) {
		case "canceled":
			outcome.result, outcome.errorKind = collectPipelineCanceled, collectErrorCanceled
		case "deadline_exceeded":
			outcome.result, outcome.errorKind = collectPipelineTimeout, collectErrorDeadline
		case "timeout":
			outcome.result, outcome.errorKind = collectPipelineTimeout, collectErrorTimeout
		case "retryable":
			outcome.result, outcome.errorKind = collectPipelineRetryable, collectErrorRetryable
		case "duplicate":
			outcome.result, outcome.errorKind = collectPipelineDuplicate, collectErrorDuplicate
		case "conflict":
			outcome.result, outcome.errorKind = collectPipelineConflict, collectErrorConflict
		default:
			outcome.result, outcome.errorKind = collectPipelineFailed, collectErrorOther
		}
	} else {
		switch result {
		case collectPipelineFailed:
			outcome.errorKind = collectErrorUnknown
		case collectPipelineTimeout:
			outcome.errorKind = collectErrorTimeout
		case collectPipelineCanceled:
			outcome.errorKind = collectErrorCanceled
		case collectPipelineRetryable:
			outcome.errorKind = collectErrorRetryable
		case collectPipelineDuplicate:
			outcome.errorKind = collectErrorDuplicate
		case collectPipelineConflict:
			outcome.errorKind = collectErrorConflict
		case collectPipelineQueued, collectPipelineIgnored, collectPipelineCompleted, collectPipelinePending,
			collectPipelineUnconfirmed, collectPipelineUnknown, collectPipelinePersisted:
		default:
			outcome.result = collectPipelineUnknown
		}
	}
	if outcome.result == collectPipelinePending || outcome.result == collectPipelineRetryable {
		outcome.waitReason = collectWaitUnspecified
		if outcome.result == collectPipelineRetryable {
			outcome.waitReason = collectWaitBackoff
		}
		for _, attr := range attrs {
			if attr.Key == "collect.wait_reason" {
				// Empty optional attributes carry no reason. Preserve backoff
				// for retryable outcomes and unspecified for ordinary waiting.
				if attr.Value.AsString() == "" {
					continue
				}
				outcome.waitReason = normalizeCollectPipelineWaitReason(attr.Value.AsString())
			}
		}
	}
	return outcome
}

func collectPipelineOutcomeAttributes(stage collectPipelineStage, outcome collectPipelineObservation) []attribute.KeyValue {
	return append(collectPipelineMetricAttributes(stage, outcome.result),
		attribute.String("error_kind", string(outcome.errorKind)),
		attribute.String("wait_reason", string(outcome.waitReason)))
}

func (instruments collectPipelineInstruments) record(ctx context.Context, stage collectPipelineStage, outcome collectPipelineObservation, duration time.Duration) {
	attrs := collectPipelineOutcomeAttributes(stage, outcome)
	meta := collectobserve.From(ctx)
	attrs = append(attrs,
		attribute.String("source_type", collectobserve.SourceType(meta.SourceType)),
		attribute.String("collection_mode", collectobserve.Mode(meta.Mode)))
	options := metric.WithAttributes(attrs...)
	// Metrics are deliberately independent of Span.IsRecording/IsSampled.
	if instruments.count != nil {
		instruments.count.Add(ctx, 1, options)
	}
	// Negative duration is the existing count-only admission convention.
	if instruments.duration != nil && duration >= 0 {
		instruments.duration.Record(ctx, duration.Seconds(), options)
	}
}

func setCollectPipelineSpanStatus(span trace.Span, outcome collectPipelineObservation) {
	switch outcome.result {
	case collectPipelineFailed:
		span.SetStatus(codes.Error, "collection stage failed")
	case collectPipelineTimeout:
		span.SetStatus(codes.Error, "collection stage timed out")
	case collectPipelineCanceled:
		span.SetStatus(codes.Error, "collection stage canceled")
	}
}

func newCollectPipelineInstruments(meter metric.Meter) collectPipelineInstruments {
	count, _ := meter.Int64Counter("xiaohuiji.collect.pipeline.operations",
		metric.WithDescription("Collection stage observations; retries are separate attempts"))
	duration, _ := meter.Float64Histogram("xiaohuiji.collect.pipeline.duration_seconds",
		metric.WithUnit("s"), metric.WithDescription("Collection stage execution time; nested stages must not be summed"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120))
	profilesPersisted, _ := meter.Int64Counter("xiaohuiji.collect.profiles.persisted",
		metric.WithDescription("Top-level committed profile persistence outcomes; reused includes idempotent retries"))
	lifecycleDuration, _ := meter.Float64Histogram("xiaohuiji.collect.lifecycle.duration_seconds",
		metric.WithUnit("s"), metric.WithDescription("Trusted local ingress to top-level profile creation commit; excludes reuse"),
		metric.WithExplicitBucketBoundaries(0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600, 1200, 3600, 7200, 21600, 43200, 86400, 259200, 604800))
	lifecycleMissing, _ := meter.Int64Counter("xiaohuiji.collect.lifecycle.missing",
		metric.WithDescription("Missing trusted creation lifecycle samples or unconfirmed outer transaction outcomes"))
	return collectPipelineInstruments{
		count: count, duration: duration, profilesPersisted: profilesPersisted,
		lifecycleDuration: lifecycleDuration, lifecycleMissing: lifecycleMissing,
	}
}

// Persistence is counted only after an owned top-level Transaction returns nil.
// It counts commit outcomes, not distinct profiles: a later retry can be reused.
// Never substitute Telegram received_at or a naive database CreatedAt for the
// trusted ingress instant carried by collectobserve.
func (instruments collectPipelineInstruments) recordProfilePersistence(ctx context.Context, profileID int64, created, topLevel bool, commitErr error, finishedAt time.Time) collectPipelineResult {
	if commitErr != nil {
		return classifyCollectPipelineObservation(collectPipelineFailed, commitErr).result
	}
	if profileID <= 0 {
		return collectPipelineUnconfirmed
	}
	action := "reused"
	if created {
		action = "created"
	}
	meta := collectobserve.From(ctx)
	attrs := []attribute.KeyValue{
		attribute.String("action", action),
		attribute.String("source_type", collectobserve.SourceType(meta.SourceType)),
		attribute.String("collection_mode", collectobserve.Mode(meta.Mode)),
	}
	lifecycleAttrs := append(attrs, attribute.String("time_basis", collectobserve.TimeBasis(meta.TimeBasis)))
	missingReason := ""
	if !topLevel {
		missingReason = "transaction_unconfirmed"
	} else {
		if instruments.profilesPersisted != nil {
			instruments.profilesPersisted.Add(ctx, 1, metric.WithAttributes(attrs...))
		}
		// Reuse has no new creation boundary. Do not repeatedly sample lifecycle
		// from an older event or pretend that the profile was newly created.
		if !created {
			return collectPipelinePersisted
		}
		switch {
		case meta.ReceivedAt.IsZero():
			missingReason = "missing_received_at"
		case meta.ReceivedAt.After(finishedAt):
			missingReason = "future_received_at"
		default:
			if instruments.lifecycleDuration != nil {
				instruments.lifecycleDuration.Record(ctx, finishedAt.Sub(meta.ReceivedAt).Seconds(), metric.WithAttributes(lifecycleAttrs...))
			}
			return collectPipelinePersisted
		}
	}
	if instruments.lifecycleMissing != nil {
		instruments.lifecycleMissing.Add(ctx, 1, metric.WithAttributes(append(lifecycleAttrs, attribute.String("reason", missingReason))...))
	}
	if !topLevel {
		return collectPipelineUnconfirmed
	}
	return collectPipelinePersisted
}

func observeCollectProfilePersistence(ctx context.Context, profileID int64, created, topLevel bool, commitErr error) {
	finishedAt := time.Now()
	result := collectPipelineMetrics.recordProfilePersistence(ctx, profileID, created, topLevel, commitErr, finishedAt)
	logCollectProfileLifecycle(ctx, profileID, created, topLevel, commitErr, finishedAt)
	if commitErr == nil && result == collectPipelineUnconfirmed {
		logCollectPipelineObservation(ctx, collectPipelineProfilePersist, result, finishedAt, finishedAt, nil,
			attribute.Int64("profile.id", profileID))
	}
}

func collectSourceAttemptWaitReason(pending bool, err error) string {
	if err != nil {
		return "error"
	}
	if pending {
		return "rescheduled"
	}
	return "none"
}

// bindCollectEventObservation only consumes the record already read by the
// business operation. received_at is Telegram's date and is not an ingress clock.
func bindCollectEventObservation(ctx context.Context, event gdb.Record) context.Context {
	return collectobserve.With(ctx, collectobserve.Metadata{
		TenantID: event["tenant_id"].Int64(), AccountID: event["account_id"].Int64(),
		SourceID: event["source_id"].Int64(), EventID: event["id"].Int64(),
		BotID: event["bot_id"].Int64(), TgAccountID: event["tg_account_id"].Int64(),
		ChatID: event["source_chat_id"].String(), SourceType: event["source_type"].String(),
	})
}

func collectContextTraceAttributes(ctx context.Context) []attribute.KeyValue {
	meta := collectobserve.From(ctx)
	return []attribute.KeyValue{
		attribute.String("source_type", collectobserve.SourceType(meta.SourceType)),
		attribute.String("collection_mode", collectobserve.Mode(meta.Mode)),
		attribute.Int64("tenant.id", meta.TenantID), attribute.Int64("account.id", meta.AccountID),
		attribute.Int64("collect.source_id", meta.SourceID), attribute.String("collect.chat_id", meta.ChatID),
		attribute.String("collect.canonical_chat_id", canonicalCollectSourceChatID(meta.ChatID)),
		attribute.Int64("collect.bot_id", meta.BotID), attribute.Int64("collect.tg_account_id", meta.TgAccountID),
		attribute.Int64("collect.event_id", meta.EventID), attribute.Int64("collect.upstream_event_id", meta.UpstreamEventID),
		attribute.Int64("collect.delivery_id", meta.DeliveryID), attribute.Int64("collect.rule_id", meta.RuleID),
		attribute.Int64("profile.id", meta.ProfileID),
	}
}

// startCollectPipelineStage creates a real parent span while the operation is
// running. The first finish wins, including concurrent/deferred/error exits.
func startCollectPipelineStage(ctx context.Context, stage collectPipelineStage) (context.Context, func(collectPipelineResult, error, ...attribute.KeyValue)) {
	return startCollectPipelineStageWith(ctx, stage, collectPipelineMetrics, otel.Tracer("hotgo/addons/youban_publish"))
}

// Inject the same factory-created instruments/tracer in isolated ManualReader
// tests without changing global providers or starting exporters.
func startCollectPipelineStageWith(ctx context.Context, stage collectPipelineStage, instruments collectPipelineInstruments, tracer trace.Tracer) (context.Context, func(collectPipelineResult, error, ...attribute.KeyValue)) {
	startedAt := time.Now()
	normalized := collectPipelineMetricAttributes(stage, collectPipelineUnknown)
	attrs := append(collectContextTraceAttributes(ctx), normalized...)
	ctx, span := tracer.Start(ctx,
		"collect."+normalized[0].Value.AsString(), trace.WithAttributes(attrs...))
	var finished atomic.Bool
	return ctx, func(result collectPipelineResult, err error, extra ...attribute.KeyValue) {
		if !finished.CompareAndSwap(false, true) {
			return
		}
		finishedAt := time.Now()
		outcome := classifyCollectPipelineObservation(result, err, extra...)
		instruments.record(ctx, stage, outcome, finishedAt.Sub(startedAt))
		for _, attr := range extra {
			switch attr.Key {
			case "stage", "result", "error.type", "error_kind", "wait_reason", "collect.wait_reason", "source_type", "collection_mode":
				continue
			}
			span.SetAttributes(attr)
		}
		span.SetAttributes(collectPipelineOutcomeAttributes(stage, outcome)...)
		span.SetAttributes(attribute.String("error.type", collectPipelineErrorType(err)))
		setCollectPipelineSpanStatus(span, outcome)
		span.End(trace.WithTimestamp(finishedAt))
		logCollectPipelineObservation(ctx, stage, result, startedAt, finishedAt, err, extra...)
	}
}

func logCollectPipelineObservation(ctx context.Context, stage collectPipelineStage, result collectPipelineResult, startedAt, finishedAt time.Time, err error, attrs ...attribute.KeyValue) {
	outcome := classifyCollectPipelineObservation(result, err, attrs...)
	result = outcome.result
	// Counters/histograms retain every attempt. Fast successful substeps do
	// not need an additional SLS row on top of the event terminal record.
	if err == nil && result == collectPipelineCompleted && finishedAt.Sub(startedAt) < 100*time.Millisecond && stage != collectPipelineProcess && stage != collectPipelineProcessConsume && stage != collectPipelineSourceAttempt {
		return
	}
	meta := collectobserve.From(ctx)
	normalized := collectPipelineMetricAttributes(stage, result)
	fields := g.Map{
		"log_type": "telegram_collect_pipeline", "event": "collect_stage_completed",
		"stage": normalized[0].Value.AsString(), "result": normalized[1].Value.AsString(),
		"durationMs": finishedAt.Sub(startedAt).Milliseconds(), "durationUs": finishedAt.Sub(startedAt).Microseconds(),
		"sourceType": collectobserve.SourceType(meta.SourceType), "collectionMode": collectobserve.Mode(meta.Mode),
		"tenantId": meta.TenantID, "accountId": meta.AccountID, "sourceId": meta.SourceID,
		"sourceChatId": meta.ChatID, "canonicalChatId": canonicalCollectSourceChatID(meta.ChatID), "botId": meta.BotID, "tgAccountId": meta.TgAccountID,
		"collectEventId": meta.EventID, "upstreamEventId": meta.UpstreamEventID, "deliveryId": meta.DeliveryID,
		"ruleId": meta.RuleID, "profileId": meta.ProfileID, "traceId": gtrace.GetTraceID(ctx),
		"errorType": collectPipelineErrorType(err),
		"errorKind": string(outcome.errorKind), "waitReason": string(outcome.waitReason),
	}
	// Retain the age of still-pending work for bounded log TOP queries. This is
	// elapsed local-ingress age, not the duration of this one stage execution.
	if !meta.ReceivedAt.IsZero() && !meta.ReceivedAt.After(finishedAt) {
		fields["ingressAgeSeconds"] = finishedAt.Sub(meta.ReceivedAt).Seconds()
		fields["receivedAtUnixMs"] = meta.ReceivedAt.UnixMilli()
		fields["ageTimeBasis"] = collectobserve.TimeBasis(meta.TimeBasis)
	}
	for _, attr := range attrs {
		// Only bounded diagnostic facts are copied, never text/error messages,
		// URLs, tokens, or caller-controlled label names.
		switch attr.Key {
		case "collect.event_id":
			fields["collectEventId"] = attr.Value.AsInt64()
		case "profile.id":
			fields["profileId"] = attr.Value.AsInt64()
		case "collect.rule_id":
			fields["ruleId"] = attr.Value.AsInt64()
		case "collect.wait_reason":
			fields["collect.wait_reason"] = string(outcome.waitReason)
		case "collect.match_count", "collect.row_count", "collect.reused", "collect.wait_sample":
			fields[string(attr.Key)] = attr.Value.AsInterface()
		}
	}
	if result == collectPipelineFailed || result == collectPipelineTimeout || result == collectPipelineCanceled {
		g.Log().Warning(ctx, "采集阶段执行失败", fields)
		return
	}
	g.Log().Info(ctx, "采集阶段执行完成", fields)
}
