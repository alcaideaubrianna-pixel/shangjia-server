package sys

import (
	"context"
	"errors"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// These instruments share the existing global provider. No reader, exporter,
// background work, or durable state is installed by reconciliation telemetry.
type publishReconcileInstruments struct {
	events metric.Int64Counter
}

func newPublishReconcileInstruments(meter metric.Meter) publishReconcileInstruments {
	events, _ := meter.Int64Counter("xiaohuiji.publish.reconcile.events",
		metric.WithDescription("Reconciliation resolution passes and evidence restoration attempts; retries count again; filter by event"))
	return publishReconcileInstruments{events: events}
}

var publishReconcileMetrics = newPublishReconcileInstruments(publishObserveMeter)

func publishReconcileSource(source string) string {
	switch source {
	case "webhook", "account", "response", "aggregate":
		return source
	default:
		return "unknown"
	}
}

func publishReconcileEvent(event string) string {
	switch event {
	case "resolution", "evidence_restore":
		return event
	default:
		return "unknown"
	}
}

func publishReconcileOutcome(outcome string) string {
	switch outcome {
	case "confirmed", "waiting", "retry_scheduled", "resumed", "projection_returned",
		"ownership_changed", "expired", "error", "canceled", "timeout", "incomplete":
		return outcome
	default:
		return "unknown"
	}
}

// A nil error does not establish delivery. Only callers with affirmative
// evidence may supply confirmed; an error always overrides that assertion.
func publishReconcileResult(outcome string, err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, errTelegramAttemptAuditIncomplete):
		return "incomplete"
	case err != nil:
		return "error"
	default:
		return publishReconcileOutcome(outcome)
	}
}

func publishReconcileRestorationResult(restored bool, err error) string {
	outcome := "unknown"
	if restored && err == nil {
		outcome = "confirmed"
	}
	return publishReconcileResult(outcome, err)
}

func (m publishReconcileInstruments) record(ctx context.Context, source, event, outcome string) {
	if m.events == nil {
		return
	}
	m.events.Add(ctx, 1, metric.WithAttributes(
		attribute.String("source", publishReconcileSource(source)),
		attribute.String("event", publishReconcileEvent(event)),
		attribute.String("outcome", publishReconcileOutcome(outcome)),
	))
}

// This scope belongs to one synchronous, lease-owned resolution invocation.
// It carries only observations; business decisions never read it.
type publishReconcileObservation struct {
	jobID     int64
	attemptID int64
	phase     string
	outcome   string
	started   time.Time
}

type publishReconcileObservationKey struct{}

func setPublishReconcileObservation(ctx context.Context, outcome string, attempt *telegramDeliveryAttempt) {
	observation, _ := ctx.Value(publishReconcileObservationKey{}).(*publishReconcileObservation)
	if observation == nil {
		return
	}
	observation.outcome = outcome
	if attempt != nil {
		observation.attemptID = attempt.Id
		observation.phase = attempt.Phase
	}
}

func logPublishReconcile(ctx context.Context, source, event, outcome string, observation publishReconcileObservation) {
	fields := g.Map{
		"log_type": "telegram_publish_reconcile",
		"source":   publishReconcileSource(source), "event": publishReconcileEvent(event),
		"outcome": publishReconcileOutcome(outcome), "jobId": observation.jobID,
		"phase": observation.phase,
	}
	if observation.attemptID > 0 {
		fields["attemptId"] = observation.attemptID
	}
	if !observation.started.IsZero() {
		fields["durationSeconds"] = time.Since(observation.started).Seconds()
	}
	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		fields["traceId"] = span.TraceID().String()
		fields["spanId"] = span.SpanID().String()
	}
	switch publishReconcileOutcome(outcome) {
	case "error", "timeout", "canceled", "incomplete":
		g.Log().Warning(ctx, "publish_reconcile", fields)
		return
	}
	g.Log().Info(ctx, "publish_reconcile", fields)
}

// restoreConfirmedTelegramResponse returns true,nil only after the durable
// restoration transaction committed. Its query combines confirmed attempts and
// response evidence, so its name alone cannot establish the evidence source.
func (s *sSysPublish) restoreObservedTelegramResponse(ctx context.Context, job telegramJobRecord, phase string, media []*telegramMediaItem) (bool, error) {
	started := time.Now()
	restored, err := s.restoreConfirmedTelegramResponse(ctx, job, phase, media)
	outcome := publishReconcileRestorationResult(restored, err)
	publishReconcileMetrics.record(ctx, "unknown", "evidence_restore", outcome)
	logPublishReconcile(ctx, "unknown", "evidence_restore", outcome, publishReconcileObservation{
		jobID: job.Id, phase: phase, started: started,
	})
	return restored, err
}
