package sys

import (
	"context"
	"errors"
	"hotgo/internal/library/collectobserve"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type collectPipelineStage string
type collectPipelineResult string

// Instruments are registered once during package initialization. The OTel
// global meter delegates them when the runtime provider is installed.
var collectPipelineMetrics = newCollectPipelineInstruments(publishObserveMeter)

const (
	collectPipelineSubmit         collectPipelineStage = "delivery_submit"
	collectPipelineProcess        collectPipelineStage = "process"
	collectPipelineDispatch       collectPipelineStage = "dispatch_handoff"
	collectPipelineAdmission      collectPipelineStage = "publish_admission"
	collectPipelineQueue          collectPipelineStage = "publish_queue_enqueue"
	collectPipelineEventPersist   collectPipelineStage = "event_persist"
	collectPipelineSourceAttempt  collectPipelineStage = "source_attempt"
	collectPipelineMediaCache     collectPipelineStage = "media_cache"
	collectPipelineMediaQuota     collectPipelineStage = "media_quota_wait"
	collectPipelineWindowPrimary  collectPipelineStage = "window_primary"
	collectPipelineWindowRetry    collectPipelineStage = "window_retry"
	collectPipelineAuthorization  collectPipelineStage = "authorization"
	collectPipelineRulePreflight  collectPipelineStage = "rule_preflight"
	collectPipelineMaterialPair   collectPipelineStage = "material_pair"
	collectPipelineRuleMatch      collectPipelineStage = "rule_match"
	collectPipelineMediaReady     collectPipelineStage = "media_ready"
	collectPipelineContentExtract collectPipelineStage = "content_extract"
	collectPipelineProfilePersist collectPipelineStage = "profile_persist"

	collectPipelineQueued      collectPipelineResult = "queued"
	collectPipelineFailed      collectPipelineResult = "failed"
	collectPipelineRetryable   collectPipelineResult = "retryable"
	collectPipelineIgnored     collectPipelineResult = "ignored"
	collectPipelineCompleted   collectPipelineResult = "completed"
	collectPipelinePending     collectPipelineResult = "pending"
	collectPipelineUnconfirmed collectPipelineResult = "unconfirmed"
	collectPipelineUnknown     collectPipelineResult = "unknown"
	collectPipelinePersisted   collectPipelineResult = "persisted"
	collectPipelineDuplicate   collectPipelineResult = "duplicate"
	collectPipelineConflict    collectPipelineResult = "conflict"
)

// Only these two bounded attributes enter metrics. Business identifiers belong
// to spans/logs; detailed errors remain exclusively in existing business logs.
func collectPipelineMetricAttributes(stage collectPipelineStage, result collectPipelineResult) []attribute.KeyValue {
	switch stage {
	case collectPipelineSubmit, collectPipelineProcess, collectPipelineDispatch, collectPipelineAdmission, collectPipelineQueue,
		collectPipelineEventPersist, collectPipelineSourceAttempt, collectPipelineMediaCache, collectPipelineMediaQuota, collectPipelineWindowPrimary, collectPipelineWindowRetry,
		collectPipelineAuthorization, collectPipelineRulePreflight, collectPipelineMaterialPair, collectPipelineRuleMatch, collectPipelineMediaReady, collectPipelineContentExtract, collectPipelineProfilePersist:
	default:
		stage = "unknown"
	}
	switch result {
	case collectPipelineQueued, collectPipelineFailed, collectPipelineRetryable,
		collectPipelineIgnored, collectPipelineCompleted, collectPipelinePending,
		collectPipelineUnconfirmed, collectPipelinePersisted, collectPipelineDuplicate, collectPipelineConflict:
	default:
		result = collectPipelineUnknown
	}
	return []attribute.KeyValue{attribute.String("stage", string(stage)), attribute.String("result", string(result))}
}

// Admission is counted per returned durable TGJob ID, not per profile and not
// as newly inserted/unique jobs: idempotent preparation can return existing IDs.
// No preparation-duration sample is repeated over all jobs in a batch.
func observeCollectPublishAdmission(ctx context.Context, jobID, profileID int64) {
	if jobID <= 0 {
		return
	}
	recordCollectPipelineMetrics(ctx, publishObserveMeter, collectPipelineAdmission, collectPipelinePersisted, -1)
	_, span := otel.Tracer("hotgo/addons/youban_publish").Start(ctx, "collect.publish_admission",
		trace.WithAttributes(append(collectPipelineMetricAttributes(collectPipelineAdmission, collectPipelinePersisted),
			attribute.Int64("job.id", jobID), attribute.Int64("profile.id", profileID))...))
	span.End()
}

func collectPublishQueueResult(receipt bool, err error) collectPipelineResult {
	switch {
	case errors.Is(err, asynq.ErrDuplicateTask):
		return collectPipelineDuplicate
	case errors.Is(err, asynq.ErrTaskIDConflict):
		return collectPipelineConflict
	case err != nil:
		return collectPipelineFailed
	case receipt:
		return collectPipelineQueued
	default:
		return collectPipelineUnconfirmed
	}
}

// Count queue submission attempts for one job, not profiles or distinct jobs.
// Each Asynq client call (including fallback) is an attempt; the channel queue
// helper folds its internal Redis retries into one final acknowledged/failed
// attempt. Recovery can observe the same job again. Duplicate/conflict is
// never a fresh receipt. These shared publish stages include all publish jobs.
func observeCollectPublishQueue(ctx context.Context, jobID, channelID int64, transport string, startedAt time.Time, receipt bool, err error) {
	observeCollectPipeline(ctx, collectPipelineQueue, collectPublishQueueResult(receipt, err), startedAt, err,
		attribute.Int64("job.id", jobID), attribute.Int64("channel.id", channelID),
		attribute.String("messaging.system", transport))
}

// This wrapper is used only at authoritative channel-queue call sites, never
// at the optional shadow queue write.
func observeCollectChannelEnqueue(ctx context.Context, channelID, jobID int64, priority int, availableAt time.Time) error {
	startedAt := time.Now()
	err := enqueueTelegramChannelJob(ctx, channelID, jobID, priority, availableAt)
	observeCollectPublishQueue(ctx, jobID, channelID, "redis_channel", startedAt, err == nil, err)
	return err
}

func recordCollectPipelineMetrics(ctx context.Context, meter metric.Meter, stage collectPipelineStage, result collectPipelineResult, duration time.Duration) {
	attrs := collectPipelineMetricAttributes(stage, result)
	meta := collectobserve.From(ctx)
	attrs = append(attrs, attribute.String("source_type", collectobserve.SourceType(meta.SourceType)), attribute.String("collection_mode", collectobserve.Mode(meta.Mode)))
	options := metric.WithAttributes(attrs...)
	var instruments collectPipelineInstruments
	if meter == publishObserveMeter {
		instruments = collectPipelineMetrics
	} else {
		instruments = newCollectPipelineInstruments(meter)
	}
	if instruments.count != nil {
		instruments.count.Add(ctx, 1, options)
	}
	if instruments.duration != nil && duration >= 0 {
		instruments.duration.Record(ctx, duration.Seconds(), options)
	}
}

func observeCollectPipeline(ctx context.Context, stage collectPipelineStage, result collectPipelineResult, startedAt time.Time, err error, attrs ...attribute.KeyValue) {
	finishedAt := time.Now()
	recordCollectPipelineMetrics(ctx, publishObserveMeter, stage, result, finishedAt.Sub(startedAt))
	recordCollectPipelineSpan(ctx, otel.Tracer("hotgo/addons/youban_publish"), stage, result, startedAt, finishedAt, err, attrs...)
	logCollectPipelineObservation(ctx, stage, result, startedAt, finishedAt, err, attrs...)
}

// Use the same whitelist as metrics, including the span name. Do not copy raw
// error messages, exception events, SQL, URLs or payloads into telemetry.
func recordCollectPipelineSpan(ctx context.Context, tracer trace.Tracer, stage collectPipelineStage, result collectPipelineResult, startedAt, finishedAt time.Time, err error, attrs ...attribute.KeyValue) {
	spanAttrs := collectPipelineMetricAttributes(stage, result)
	spanAttrs = append(spanAttrs, collectContextTraceAttributes(ctx)...)
	spanName := "collect." + spanAttrs[0].Value.AsString()
	for _, attr := range attrs {
		switch attr.Key {
		case "stage", "result", "error.type":
			// Caller attributes cannot override normalized dimensions.
			continue
		}
		spanAttrs = append(spanAttrs, attr)
	}
	_, span := tracer.Start(ctx, spanName,
		trace.WithTimestamp(startedAt), trace.WithAttributes(spanAttrs...))
	if err != nil {
		span.SetAttributes(attribute.String("error.type", collectPipelineErrorType(err)))
		span.SetStatus(codes.Error, "collection stage failed")
	}
	span.End(trace.WithTimestamp(finishedAt))
}

func collectPipelineErrorType(err error) string {
	var retry *collectProcessRetryError
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.As(err, &retry):
		return "retryable"
	case errors.Is(err, asynq.ErrDuplicateTask):
		return "duplicate"
	case errors.Is(err, asynq.ErrTaskIDConflict):
		return "conflict"
	default:
		return "other"
	}
}

func collectPipelineEventAttributes(event gdb.Record) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Int64("collect.event_id", event["id"].Int64()),
		attribute.Int64("tenant.id", event["tenant_id"].Int64()),
		attribute.Int64("account.id", event["account_id"].Int64()),
		attribute.Int64("collect.source_id", event["source_id"].Int64()),
		attribute.String("collect.chat_id", event["source_chat_id"].String()),
	}
}

func observeCollectPipelineSubmit(ctx context.Context, startedAt time.Time, result collectorSubmitResult, delivery *collectorin.CollectorDelivery) {
	outcome := collectPipelineFailed
	if result.Outcome == collectorSubmitOutcomeQueued {
		outcome = collectPipelineQueued
	}
	attrs := []attribute.KeyValue{
		attribute.Int64("collect.event_id", result.EventID),
		attribute.String("collect.submit_boundary", string(result.Stage)),
		attribute.String("collect.submit_reason", result.Reason),
	}
	if delivery != nil {
		attrs = append(attrs,
			attribute.Int64("collect.delivery_id", delivery.ID),
			attribute.Int64("collect.upstream_event_id", delivery.EventID),
			attribute.Int64("tenant.id", delivery.TenantID),
			attribute.Int64("account.id", delivery.AccountID),
			attribute.Int64("collect.source_id", delivery.SourceID),
			attribute.String("collect.chat_id", delivery.SourceChatID))
	}
	observeCollectPipeline(ctx, collectPipelineSubmit, outcome, startedAt, result.Err, attrs...)
}

func collectPipelineProcessResult(status string, processed bool, err error) collectPipelineResult {
	if err != nil {
		var retry *collectProcessRetryError
		if errors.As(err, &retry) {
			return collectPipelineRetryable
		}
		return collectPipelineFailed
	}
	if status == sysin.CollectEventStatusIgnored {
		return collectPipelineIgnored
	}
	if !processed {
		return collectPipelinePending
	}
	return collectPipelineCompleted
}

// observeCollectDispatchHandoff deliberately reports nil as unconfirmed:
// submitCollectProfileDispatch can return nil for waiting media or a skip,
// and its downstream enqueue helper has no error/receipt return value.
// This is an invocation result, never a successful queue admission counter.
func observeCollectDispatchHandoff(ctx context.Context, event gdb.Record, dispatchID, profileID int64, submit func() error) error {
	startedAt := time.Now()
	err := submit()
	result := collectPipelineDispatchResult(err)
	attrs := append(collectPipelineEventAttributes(event),
		attribute.Int64("collect.dispatch_id", dispatchID), attribute.Int64("profile.id", profileID))
	observeCollectPipeline(ctx, collectPipelineDispatch, result, startedAt, err, attrs...)
	return err
}

func collectPipelineDispatchResult(err error) collectPipelineResult {
	if err != nil {
		return collectPipelineFailed
	}
	return collectPipelineUnconfirmed
}
