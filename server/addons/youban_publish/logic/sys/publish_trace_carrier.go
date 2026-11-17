package sys

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// publishTraceCarrier carries W3C context only. Never include baggage, task
// arguments, credentials, or timestamps in this envelope.
//
// Coverage: optional non-Unique Asynq payloads capture the enqueue parent.
// Cleanup intents capture it in the existing outbox JSON write, whose dedupe
// key is independent of payload bytes; redelivery restores that parent.
// Unique Asynq bodies (including every cleanup fallback) omit this field.
// Consumers must use the context-returning decode/hydration helpers below to
// restore a carried parent. Existing decoder APIs remain unchanged.
// The unified stream's actual worker payload is outside this carrier's path:
// its execution/stage spans do not imply enqueue-to-worker parent continuity.
// Legacy payloads remain valid; job/operation IDs still correlate their logs.
type publishTraceCarrier struct {
	Traceparent string `json:"traceparent,omitempty"`
	Tracestate  string `json:"tracestate,omitempty"`
}

const publishTraceCarrierMaxBytes = 2048

func (c *publishTraceCarrier) Get(key string) string {
	switch key {
	case "traceparent":
		return c.Traceparent
	case "tracestate":
		return c.Tracestate
	default:
		return ""
	}
}

func (c *publishTraceCarrier) Set(key, value string) {
	switch key {
	case "traceparent":
		if len(value) <= 512 {
			c.Traceparent = value
		}
	case "tracestate":
		if len(value) <= 512 {
			c.Tracestate = value
		}
	}
}

func (c *publishTraceCarrier) Keys() []string {
	return []string{"traceparent", "tracestate"}
}

// Invalid optional telemetry must never make an otherwise valid task fail.
func (c *publishTraceCarrier) UnmarshalJSON(body []byte) error {
	*c = publishTraceCarrier{}
	if len(body) > publishTraceCarrierMaxBytes {
		return nil
	}
	type wire publishTraceCarrier
	var value wire
	if json.Unmarshal(body, &value) != nil {
		return nil
	}
	c.Set("traceparent", value.Traceparent)
	c.Set("tracestate", value.Tracestate)
	if !trace.SpanContextFromContext(propagation.TraceContext{}.Extract(context.Background(), c)).IsValid() {
		*c = publishTraceCarrier{}
	}
	return nil
}

func capturePublishTrace(ctx context.Context) *publishTraceCarrier {
	if ctx == nil {
		ctx = context.Background()
	}
	carrier := &publishTraceCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	// Keep W3C interoperability when the configured composite omits it.
	propagation.TraceContext{}.Inject(ctx, carrier)
	if carrier.Traceparent == "" {
		return nil
	}
	return carrier
}

func restorePublishTrace(ctx context.Context, carrier *publishTraceCarrier) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if carrier == nil || len(carrier.Traceparent) > 512 || len(carrier.Tracestate) > 512 {
		return ctx
	}
	if !trace.SpanContextFromContext(propagation.TraceContext{}.Extract(context.Background(), carrier)).IsValid() {
		return ctx
	}
	ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)
	return propagation.TraceContext{}.Extract(ctx, carrier)
}

func telegramQueueTracePayload(ctx context.Context, payload tgQueuePayload, unique bool) tgQueuePayload {
	payload.Trace = nil
	if !unique {
		payload.Trace = capturePublishTrace(ctx)
	}
	return payload
}

// decodeTelegramQueuePayloadContext is the consumer integration point for
// transports that can carry optional metadata without changing task identity.
// Existing Unique-protected Asynq producers deliberately do not inject it.
func decodeTelegramQueuePayloadContext(ctx context.Context, task *asynq.Task) (context.Context, tgQueuePayload, error) {
	payload, err := decodeTelegramQueuePayload(task)
	if err != nil {
		return ctx, payload, err
	}
	return restorePublishTrace(ctx, payload.Trace), payload, nil
}

// Use in place of hydration at a cleanup consumer boundary. It performs only
// the pre-existing hydration read, then restores that persisted first writer.
func hydrateTelegramCleanupPayloadContext(ctx context.Context, jobID int64, payload tgQueuePayload) (context.Context, tgQueuePayload, error) {
	restored, err := hydrateTelegramCleanupPayload(ctx, jobID, payload)
	if err != nil {
		return ctx, restored, err
	}
	return restorePublishTrace(ctx, restored.Trace), restored, nil
}

// Each invocation covers one worker delivery only, including retries. End it
// before returning to the queue; queue delay/backoff is never a live span.
func beginTelegramPublishExecution(ctx context.Context, payload tgQueuePayload) (context.Context, trace.Span) {
	return beginTelegramPublishExecutionWith(restorePublishTrace(ctx, payload.Trace), payload.JobId,
		otel.Tracer("hotgo/addons/youban_publish"))
}

func beginTelegramPublishExecutionWith(ctx context.Context, jobID int64, tracer trace.Tracer) (context.Context, trace.Span) {
	retry, _ := asynq.GetRetryCount(ctx)
	return tracer.Start(ctx, "telegram.publish.execute", trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(attribute.Int64("job.id", jobID), attribute.Int("messaging.retry_count", retry)))
}

func finishTelegramPublishSpan(span trace.Span, err error) {
	if span == nil {
		return
	}
	result := publishStageResult(err, false)
	span.SetAttributes(attribute.String("execution.result", result))
	if result == "error" || result == "canceled" || result == "deadline_exceeded" {
		// Raw Telegram/DB errors can contain tokens, URLs, or message content.
		span.SetStatus(codes.Error, "publishing stage failed")
	}
	span.End()
}

type publishStageInstruments struct {
	attempts metric.Int64Counter
	duration metric.Float64Histogram
}

// Dashboard contract: xiaohuiji.publish.stage.attempts (counter) and
// xiaohuiji.publish.stage.duration_seconds (histogram, seconds). Both have only
// stage/result labels. result is returned/error/canceled/deadline_exceeded/
// duplicate/conflict/retry/panic. "returned" is not a final business outcome.
// One sample per finished invocation, including retries; nested durations must
// not be summed. Unknown stage names collapse to "unknown".
var publishStageMetrics = newPublishStageInstruments(otel.Meter("hotgo/addons/youban_publish"))

func newPublishStageInstruments(meter metric.Meter) publishStageInstruments {
	attempts, _ := meter.Int64Counter("xiaohuiji.publish.stage.attempts",
		metric.WithDescription("Finished stage invocations; returned does not imply business success"))
	duration, _ := meter.Float64Histogram("xiaohuiji.publish.stage.duration_seconds",
		metric.WithUnit("s"), metric.WithDescription("Stage execution duration; excludes queue backoff; nested stages must not be summed"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300, 600, 1200, 1800, 3600))
	return publishStageInstruments{attempts: attempts, duration: duration}
}

func normalizePublishStage(stage string) string {
	switch stage {
	case "dispatch.enqueue", "dispatch.account_availability", "dispatch.channel_order",
		"dispatch.channel_capacity", "dispatch.reserve_result",
		"queue.enqueue", "queue.redis_enqueue", "queue.save_task_id", "queue.decode_payload",
		"queue.consume", "queue.load_job", "queue.stale_job", "queue.wait_observed", "queue.deferred",
		"cleanup.intent.enqueue", "caption.profile_source", "caption.account_setting",
		"media.send_set", "media.group.prepare", "api.sendMediaGroup",
		"media.group.fallback_url", "media.group.fallback_upload", "media.send_single",
		"media.single.prepare_input", "media.video.preview", "media.video.metadata",
		"api.sendVideo", "api.sendPhoto", "media.prepare_item", "media.group.video_preview",
		"media.group.video_metadata", "media.video.poster.resolve", "media.video.poster.protection",
		"media.video.poster.open", "media.video.poster.download", "media.video.poster.generate",
		"media.video.poster.persist", "media.upload.prepare", "media.video.normalize",
		"media.video.strip_metadata", "media.video.compress", "media.cache.resolve",
		"media.cache.download", "media.query", "media.collect_file_ids",
		"vip.cache.get", "vip.cache.decode", "vip.database.load", "vip.cache.set",
		"outbox.dispatch", "outbox.decision", "outbox.lease.acquire", "outbox.lease.result",
		"outbox.deferred", "outbox.decision.inputs", "protection.channel_policy",
		"protection.vip_policy", "protection.selected", "api_success.message_group_check",
		"api_success.message_upsert", "attempt.prepare", "attempt.lock.acquire",
		"attempt.create_transaction", "attempt.project", "api_success.confirm",
		"api_success.commit_transaction", "api_success.write_audit", "api_success.write_messages",
		"api_success.project_attempt", "api_success.cache_metadata", "api_success.database_budget",
		"channel.send_lease.owner", "channel.send_lease.acquire", "channel.send_lease.release",
		"channel.send_gate", "channel.wake_next", "channel.publish_interval", "channel.order_check",
		"job.state_transition", "job.state_transition_result", "recovery.pre_attempt",
		"job.execute", "job.load", "job.media_ready", "job.deferred", "job.reconciliation_lane",
		"job.reload_before_send", "job.locked_send", "job.claim", "job.can_send",
		"job.cleanup_previous_publish", "job.project_publishing_state", "job.upsert_sending_record",
		"job.append_started_log", "job.complete", "send.execute", "prepare.bot_token",
		"prepare.caption", "prepare.display_media", "prepare.media_selection", "prepare.verify_media",
		"prepare.bot_client", "prepare.content_protection", "prepare.content_protection_all",
		"display.check_job_state", "display.attempt_create", "display.api_send",
		"display.confirm_api_success", "verify.restore_response", "verify.attempt_create",
		"verify.api_send", "verify.confirm_api_success", "account_fallback.submit",
		"job.handle_error", "job.error_decision", "job.failure_transition":
		return stage
	default:
		return "unknown"
	}
}

func publishStageResult(err error, panicked bool) string {
	var retry *tgRetryAfterError
	switch {
	case panicked:
		return "panic"
	case err == nil:
		return "returned"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, errPublishStageAbandoned):
		return "abandoned"
	case errors.Is(err, asynq.ErrDuplicateTask):
		return "duplicate"
	case errors.Is(err, asynq.ErrTaskIDConflict):
		return "conflict"
	case errors.As(err, &retry):
		return "retry"
	default:
		return "error"
	}
}

func startPublishStageObservation(ctx context.Context, stage string) func(error, bool) {
	return startPublishStageObservationWith(ctx, stage, publishStageMetrics, otel.Tracer("hotgo/addons/youban_publish"))
}

// Spans cover this invocation only. Nil means the stage returned without an
// error; deferred, stale, skipped, and completed jobs all remain distinguishable
// by the existing lifecycle instrumentation, not by these stage metrics.
func startPublishStageObservationWith(ctx context.Context, stage string, instruments publishStageInstruments, tracer trace.Tracer) func(error, bool) {
	stage = normalizePublishStage(stage)
	started := time.Now()
	kind := trace.SpanKindInternal
	if stage == "queue.consume" {
		kind = trace.SpanKindConsumer
	}
	_, span := tracer.Start(ctx, "telegram.publish.stage."+stage,
		trace.WithSpanKind(kind), trace.WithAttributes(attribute.String("stage", stage)))
	if stage == "queue.consume" || stage == "job.execute" {
		retry, _ := asynq.GetRetryCount(ctx)
		span.SetAttributes(attribute.Int("messaging.retry_count", retry))
	}
	var finished atomic.Bool
	return func(err error, panicked bool) {
		if !finished.CompareAndSwap(false, true) {
			return
		}
		result := publishStageResult(err, panicked)
		attrs := []attribute.KeyValue{attribute.String("stage", stage), attribute.String("result", result)}
		options := metric.WithAttributes(attrs...)
		if instruments.attempts != nil {
			instruments.attempts.Add(ctx, 1, options)
		}
		if instruments.duration != nil {
			instruments.duration.Record(ctx, time.Since(started).Seconds(), options)
		}
		span.SetAttributes(attrs...)
		if result == "error" || result == "panic" || result == "deadline_exceeded" || result == "canceled" || result == "abandoned" {
			span.SetStatus(codes.Error, "publishing stage failed")
		}
		span.End()
	}
}
