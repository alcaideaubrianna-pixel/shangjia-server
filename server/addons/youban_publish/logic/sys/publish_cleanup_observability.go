package sys

import (
	"context"
	"errors"
	"net"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"hotgo/utility/sensitive"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const publishCleanupScope = "hotgo/youban_publish/cleanup"

type publishCleanupInstruments struct {
	attempts        metric.Int64Counter
	duration        metric.Float64Histogram
	tracer          trace.Tracer
	messageRequests metric.Int64Counter
	messageDuration metric.Float64Histogram
}

// OTel's global proxies bind to the configured providers when the SDK starts.
// Create instruments once; beginPublishCleanupObservationWith injects isolated test instruments.
var publishCleanupObservability = newPublishCleanupInstruments(
	otel.Meter(publishCleanupScope), otel.Tracer(publishCleanupScope))

func newPublishCleanupInstruments(meter metric.Meter, tracer trace.Tracer) publishCleanupInstruments {
	attempts, _ := meter.Int64Counter("xiaohuiji.publish.cleanup.attempts",
		metric.WithDescription("Cleanup execution invocations by final outcome; retries and settled_attempt are not unique intents"))
	duration, _ := meter.Float64Histogram("xiaohuiji.publish.cleanup.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Cleanup attempt and nested stage durations in seconds; do not sum nested stages"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120, 300))
	messageRequests, _ := meter.Int64Counter("xiaohuiji.publish.cleanup.message.requests",
		metric.WithDescription("Concrete Bot deleteMessage calls; already_absent is idempotent cleanup resolution, retries count again"))
	messageDuration, _ := meter.Float64Histogram("xiaohuiji.publish.cleanup.message.duration",
		metric.WithUnit("s"), metric.WithDescription("One concrete Bot deleteMessage call duration"),
		metric.WithExplicitBucketBoundaries(.005, .01, .05, .1, .5, 1, 5, 10, 30, 60, 120))
	return publishCleanupInstruments{attempts: attempts, duration: duration, tracer: tracer,
		messageRequests: messageRequests, messageDuration: messageDuration}
}

func (m publishCleanupInstruments) messageRequest(ctx context.Context, deleted bool, elapsed time.Duration, err error) {
	outcome := "unknown"
	switch {
	case err == nil && deleted:
		outcome = "deleted"
	case err == nil:
	case isTelegramMessageAlreadyDeletedError(err):
		outcome = "already_absent"
	default:
		outcome = normalizePublishCleanupOutcome("failed", err)
	}
	attrs := metric.WithAttributes(attribute.String("transport", "bot"), attribute.String("outcome", outcome))
	if m.messageRequests != nil {
		m.messageRequests.Add(ctx, 1, attrs)
	}
	if m.messageDuration != nil && elapsed >= 0 {
		m.messageDuration.Record(ctx, elapsed.Seconds(), attrs)
	}
}

// Never derive cleanup reason from the historical publishing Job.OperationNo.
func publishCleanupReason(operationNo string) string {
	prefix, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(operationNo)), ":")
	switch prefix {
	case "unpublish", "down", "bot_unpublish":
		return "unpublish"
	case "delete", "bot_delete":
		return "delete"
	case "republish", "ai-republish":
		return "republish"
	case "edit", "batchtext":
		return "edit"
	case "cancel":
		return "cancel"
	case "":
		return "unspecified"
	default:
		return "other"
	}
}

func publishCleanupErrorType(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	default:
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			return "timeout"
		}
		var retry *tgRetryAfterError
		if errors.As(err, &retry) {
			return "retry_after"
		}
		return "other"
	}
}

var publishCleanupCredentialPattern = regexp.MustCompile(`(?i)(\b(?:password|passwd|pwd|token|secret|authorization|api[_-]?key|credential)\b\s*[:=]\s*)(?:"[^"]*"|'[^']*'|(?:Bearer\s+)?[^\s,;]+)|\bBearer\s+[^\s,;]+`)

func publishCleanupSafeText(text string) string {
	text = sensitive.Diagnostic(text)
	text = publishCleanupCredentialPattern.ReplaceAllString(text, "[credential redacted]")
	text = strings.NewReplacer("\n", " ", "\r", " ").Replace(text)
	if len(text) > 2048 {
		text = text[:2048]
	}
	return text
}

type publishCleanupObservation struct {
	instruments publishCleanupInstruments
	ctx         context.Context
	span        trace.Span
	started     time.Time
	jobID       int64
	job         telegramJobRecord
	payload     tgQueuePayload
	outcome     string
	finished    atomic.Bool
}

func beginPublishCleanupObservation(ctx context.Context, jobID int64, payload tgQueuePayload) (context.Context, *publishCleanupObservation) {
	return beginPublishCleanupObservationWith(ctx, jobID, payload, publishCleanupObservability)
}

func beginPublishCleanupObservationWith(ctx context.Context, jobID int64, payload tgQueuePayload, instruments publishCleanupInstruments) (context.Context, *publishCleanupObservation) {
	if instruments.tracer == nil {
		instruments.tracer = trace.NewNoopTracerProvider().Tracer(publishCleanupScope)
	}
	ctx, span := instruments.tracer.Start(ctx, "publish.cleanup.attempt")
	return ctx, &publishCleanupObservation{
		instruments: instruments, ctx: ctx, span: span, started: time.Now(),
		jobID: jobID, payload: payload, outcome: "incomplete",
	}
}

func normalizePublishCleanupStage(stage string) string {
	switch stage {
	case "attempt", "hydrate_payload", "load_job", "channel_cleanup_slot", "job_cleanup_lease",
		"delete_messages", "notify_unpublish_completed", "finish_profile_state",
		"settle_profile_cleanup_progress", "complete_cleanup_intent", "requeue":
		return stage
	default:
		return "other"
	}
}

func normalizePublishCleanupOutcome(outcome string, err error) string {
	switch publishCleanupErrorType(err) {
	case "canceled":
		return "canceled"
	case "timeout":
		return "timeout"
	case "retry_after":
		return "deferred"
	}
	if err != nil {
		return "failed"
	}
	switch outcome {
	case "completed", "skipped", "deferred", "settled_attempt", "incomplete":
		return outcome
	default:
		return "incomplete"
	}
}

func (o *publishCleanupObservation) logStarted() {
	data := o.fields("attempt", "incomplete", o.started, nil)
	data["log_type"], data["event"] = "telegram_cleanup", "started"
	g.Log().Info(o.ctx, "TG资料清理链路", data)
}

func (o *publishCleanupObservation) attributes(outcome string, err error) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("reason", publishCleanupReason(o.payload.CleanupOperationNo)),
		attribute.String("outcome", outcome),
		attribute.String("error_type", publishCleanupErrorType(err)),
	}
}

func (o *publishCleanupObservation) fields(stage, outcome string, started time.Time, err error) g.Map {
	data := g.Map{
		"log_type": "telegram_cleanup_stage", "event": "finished",
		"stage": stage, "outcome": outcome,
		"jobId": o.jobID, "cleanupOutboxId": o.payload.CleanupOutboxId,
		"profileId": o.job.ProfileId, "tenantId": o.job.TenantId,
		"accountId": o.job.AccountId, "channelId": o.job.ChannelId,
		"cleanupOperationNo": publishCleanupSafeText(strings.TrimSpace(o.payload.CleanupOperationNo)),
		"reason":             publishCleanupReason(o.payload.CleanupOperationNo),
		"traceID":            o.span.SpanContext().TraceID().String(),
		"durationMs":         time.Since(started).Milliseconds(),
		"returnedError":      err != nil, "error_type": publishCleanupErrorType(err),
	}
	if err != nil {
		data["error"] = publishCleanupSafeText(err.Error())
	}
	return data
}

func (o *publishCleanupObservation) stage(stage string, started time.Time, err error) {
	outcome := "completed"
	if err != nil {
		outcome = "failed"
	}
	o.recordStage(stage, outcome, started, err)
}

func (o *publishCleanupObservation) recordStage(stage, outcome string, started time.Time, err error) {
	stage = normalizePublishCleanupStage(stage)
	outcome = normalizePublishCleanupOutcome(outcome, err)
	attrs := append(o.attributes(outcome, err), attribute.String("stage", stage))
	if o.instruments.duration != nil {
		o.instruments.duration.Record(o.ctx, time.Since(started).Seconds(), metric.WithAttributes(attrs...))
	}
	_, span := o.instruments.tracer.Start(o.ctx, "publish.cleanup."+stage, trace.WithTimestamp(started), trace.WithAttributes(attrs...))
	if err != nil {
		span.RecordError(gerror.New(publishCleanupErrorType(err)))
		if outcome != "deferred" {
			span.SetStatus(codes.Error, "cleanup stage failed")
		}
	}
	span.End()
	data := o.fields(stage, outcome, started, err)
	if err != nil {
		g.Log().Warning(o.ctx, "TG资料清理阶段", data)
	} else {
		g.Log().Info(o.ctx, "TG资料清理阶段", data)
	}
}

// Exactly one terminal observation per invocation, never one per nested stage.
// A nil return is insufficient evidence of settlement (slot requeue also returns nil).
func (o *publishCleanupObservation) finish(err error) {
	if !o.finished.CompareAndSwap(false, true) {
		return
	}
	outcome := normalizePublishCleanupOutcome(o.outcome, err)
	if outcome == "completed" || outcome == "skipped" {
		outcome = "incomplete"
	}
	attrs := o.attributes(outcome, err)
	if o.instruments.attempts != nil {
		o.instruments.attempts.Add(o.ctx, 1, metric.WithAttributes(attrs...))
	}
	if o.instruments.duration != nil {
		o.instruments.duration.Record(o.ctx, time.Since(o.started).Seconds(),
			metric.WithAttributes(append(attrs, attribute.String("stage", "attempt"))...))
	}
	o.span.SetAttributes(attrs...)
	o.span.SetAttributes(
		attribute.Int64("jobId", o.jobID),
		attribute.Int64("cleanupOutboxId", o.payload.CleanupOutboxId),
		attribute.String("cleanupOperationNo", publishCleanupSafeText(o.payload.CleanupOperationNo)))
	if err != nil {
		o.span.RecordError(gerror.New(publishCleanupErrorType(err)))
	}
	if outcome == "failed" || outcome == "incomplete" || outcome == "timeout" || outcome == "canceled" {
		o.span.SetStatus(codes.Error, "cleanup attempt did not settle")
	}
	data := o.fields("attempt", outcome, o.started, err)
	data["log_type"] = "telegram_cleanup"
	if err != nil || outcome == "incomplete" {
		g.Log().Warning(o.ctx, "TG资料清理链路", data)
	} else {
		g.Log().Info(o.ctx, "TG资料清理链路", data)
	}
	o.span.End()
}
