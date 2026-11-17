package sys

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func newPublishCleanupTestInstruments(t *testing.T) (publishCleanupInstruments, *sdkmetric.ManualReader, *tracetest.SpanRecorder) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	recorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		_ = tracerProvider.Shutdown(context.Background())
	})
	return newPublishCleanupInstruments(provider.Meter(publishCleanupScope), tracerProvider.Tracer(publishCleanupScope)), reader, recorder
}

func collectPublishCleanupTestMetrics(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	result := make(map[string]metricdata.Metrics)
	for _, scope := range data.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			result[measurement.Name] = measurement
		}
	}
	return result
}

func TestPublishDeleteRequestDoesNotTreatFalseAsDeleted(t *testing.T) {
	m, reader, _ := newPublishCleanupTestInstruments(t)
	ctx := context.Background()
	m.messageRequest(ctx, false, time.Second, nil)
	m.messageRequest(ctx, true, time.Second, nil)
	m.messageRequest(ctx, true, time.Second, context.DeadlineExceeded)
	data := collectPublishCleanupTestMetrics(t, reader)
	sum := data["xiaohuiji.publish.cleanup.message.requests"].Data.(metricdata.Sum[int64])
	if len(sum.DataPoints) != 3 {
		t.Fatalf("delete result series=%v", sum)
	}
	outcomes := map[string]int64{}
	for _, point := range sum.DataPoints {
		if point.Attributes.Len() != 2 {
			t.Fatalf("unbounded delete labels=%v", point.Attributes)
		}
		outcome, _ := point.Attributes.Value("outcome")
		outcomes[outcome.AsString()] += point.Value
	}
	if outcomes["unknown"] != 1 || outcomes["deleted"] != 1 || outcomes["timeout"] != 1 {
		t.Fatalf("incorrect delete outcomes=%v", outcomes)
	}
	if data["xiaohuiji.publish.cleanup.message.duration"].Unit != "s" {
		t.Fatal("delete request latency must use seconds")
	}
}

func cleanupTestAttribute(t *testing.T, attrs attribute.Set, key, want string) {
	t.Helper()
	got, ok := attrs.Value(attribute.Key(key))
	if !ok || got.AsString() != want {
		t.Fatalf("attribute %s = %v, want %q", key, got, want)
	}
}

func TestPublishCleanupReasonUsesOnlyCurrentOperation(t *testing.T) {
	for _, tc := range []struct{ operation, want string }{
		{"unpublish:12:current", "unpublish"},
		{" DOWN:12:current ", "unpublish"},
		{"bot_unpublish:20:current", "unpublish"},
		{"delete:12:current", "delete"},
		{"bot_delete:20:current", "delete"},
		{"republish:12:current", "republish"},
		{"ai-republish:12:current", "republish"},
		{"edit:12:current", "edit"},
		{"batchtext:12:current", "edit"},
		{"cancel:hash:initial", "cancel"},
		{"", "unspecified"},
		{"unrecognized:arbitrary-secret", "other"},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			if got := publishCleanupReason(tc.operation); got != tc.want {
				t.Fatalf("reason = %q, want %q", got, tc.want)
			}
		})
	}
	inst, reader, _ := newPublishCleanupTestInstruments(t)
	_, observation := beginPublishCleanupObservationWith(context.Background(), 7, tgQueuePayload{}, inst)
	observation.job.OperationNo = "bot_unpublish:historical"
	observation.finish(nil)
	data := collectPublishCleanupTestMetrics(t, reader)
	sum := data["xiaohuiji.publish.cleanup.attempts"].Data.(metricdata.Sum[int64])
	cleanupTestAttribute(t, sum.DataPoints[0].Attributes, "reason", "unspecified")
}

func TestPublishCleanupAttemptOutcomesAndSeconds(t *testing.T) {
	failure := errors.New("storage failed")
	for _, tc := range []struct {
		name, outcome, want, errorType string
		err                            error
	}{
		{"settled", "settled_attempt", "settled_attempt", "none", nil},
		{"nil_slot_requeue", "deferred", "deferred", "none", nil},
		{"nil_without_settlement", "incomplete", "incomplete", "none", nil},
		{"finish_state_failed", "incomplete", "failed", "other", failure},
		{"settle_progress_failed", "incomplete", "failed", "other", failure},
		{"complete_intent_failed", "incomplete", "failed", "other", failure},
		{"late_lease_error", "settled_attempt", "failed", "other", failure},
		{"canceled", "incomplete", "canceled", "canceled", fmt.Errorf("wrapped: %w", context.Canceled)},
		{"timeout", "incomplete", "timeout", "timeout", fmt.Errorf("wrapped: %w", context.DeadlineExceeded)},
		{"retry_after", "incomplete", "deferred", "retry_after", &tgRetryAfterError{after: time.Second, err: failure}},
		{"requeue_failed", "deferred", "failed", "other", failure},
		{"stage_success_is_not_settlement", "completed", "incomplete", "none", nil},
		{"unknown_outcome", "operation-with-unbounded-id", "incomplete", "none", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inst, reader, _ := newPublishCleanupTestInstruments(t)
			_, observation := beginPublishCleanupObservationWith(context.Background(), 7,
				tgQueuePayload{CleanupOperationNo: "delete:12:current", CleanupOutboxId: 8}, inst)
			observation.started = time.Now().Add(-2 * time.Second)
			observation.recordStage("delete_messages", "completed", time.Now().Add(-time.Second), nil)
			observation.outcome = tc.outcome
			observation.finish(tc.err)
			observation.finish(tc.err) // Duplicate finish must not add a second execution attempt.
			data := collectPublishCleanupTestMetrics(t, reader)
			if len(data) != 2 {
				t.Fatalf("unexpected metrics (cleanup must not emit profile success): %v", data)
			}
			counter := data["xiaohuiji.publish.cleanup.attempts"].Data.(metricdata.Sum[int64])
			if len(counter.DataPoints) != 1 || counter.DataPoints[0].Value != 1 {
				t.Fatalf("attempts = %+v", counter)
			}
			attrs := counter.DataPoints[0].Attributes
			if attrs.Len() != 3 {
				t.Fatalf("counter labels must exclude IDs and stages: %v", attrs)
			}
			cleanupTestAttribute(t, attrs, "reason", "delete")
			cleanupTestAttribute(t, attrs, "outcome", tc.want)
			cleanupTestAttribute(t, attrs, "error_type", tc.errorType)
			duration := data["xiaohuiji.publish.cleanup.duration"]
			if duration.Unit != "s" {
				t.Fatalf("duration unit = %q", duration.Unit)
			}
			histogram := duration.Data.(metricdata.Histogram[float64])
			if len(histogram.DataPoints) != 2 {
				t.Fatalf("want stage plus attempt histogram, got %+v", histogram)
			}
			for _, point := range histogram.DataPoints {
				if point.Count != 1 || point.Sum < 0.9 || point.Sum > 30 {
					t.Fatalf("duration must be seconds, got %+v", point)
				}
				if point.Attributes.Len() != 4 {
					t.Fatalf("histogram labels = %v", point.Attributes)
				}
			}
		})
	}
}

func TestPublishCleanupStagesBoundedAndTraceErrorsSafe(t *testing.T) {
	inst, reader, recorder := newPublishCleanupTestInstruments(t)
	ctx, observation := beginPublishCleanupObservationWith(context.Background(), 7,
		tgQueuePayload{CleanupOperationNo: "cancel:current", CleanupOutboxId: 8}, inst)
	secret := "credential-not-recognized-by-any-redactor"
	raw := errors.New(secret)
	observation.recordStage("user-supplied-stage-id", "user-supplied-outcome", time.Now(), nil)
	observation.stage("settle_profile_cleanup_progress", time.Now(), raw)
	observation.recordStage("notify_unpublish_completed", "skipped", time.Now(), nil)
	fields := observation.fields("attempt", "failed", time.Now(), raw)
	if fields["cleanupOutboxId"] != int64(8) || fields["cleanupOperationNo"] != "cancel:current" ||
		fields["traceID"] != observation.span.SpanContext().TraceID().String() {
		t.Fatalf("missing SLS correlation: %v", fields)
	}
	if ctx == nil || !observation.span.SpanContext().IsValid() {
		t.Fatal("attempt must provide valid trace context")
	}
	observation.finish(raw)
	data := collectPublishCleanupTestMetrics(t, reader)
	histogram := data["xiaohuiji.publish.cleanup.duration"].Data.(metricdata.Histogram[float64])
	foundOther := false
	for _, point := range histogram.DataPoints {
		stage, _ := point.Attributes.Value(attribute.Key("stage"))
		if stage.AsString() == "other" {
			foundOther = true
			cleanupTestAttribute(t, point.Attributes, "outcome", "incomplete")
		}
	}
	if !foundOther {
		t.Fatal("unknown stage must normalize to other")
	}
	for _, span := range recorder.Ended() {
		if strings.Contains(span.Status().Description, secret) {
			t.Fatal("raw error leaked into span status")
		}
		for _, event := range span.Events() {
			for _, attr := range event.Attributes {
				if strings.Contains(attr.Value.AsString(), secret) {
					t.Fatal("raw error leaked into trace event")
				}
			}
		}
	}
}

type publishCleanupFailingMeter struct{ metric.Meter }

func (publishCleanupFailingMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, errors.New("instrument unavailable")
}

func (publishCleanupFailingMeter) Float64Histogram(string, ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	return nil, errors.New("instrument unavailable")
}

func TestPublishCleanupInstrumentFailureAndConcurrentFinish(t *testing.T) {
	inst := newPublishCleanupInstruments(publishCleanupFailingMeter{}, nil)
	_, observation := beginPublishCleanupObservationWith(context.Background(), 7, tgQueuePayload{}, inst)
	observation.stage("delete_messages", time.Now(), nil)
	observation.finish(nil) // A metric creation failure must not panic or alter cleanup.

	inst, reader, _ := newPublishCleanupTestInstruments(t)
	_, observation = beginPublishCleanupObservationWith(context.Background(), 7, tgQueuePayload{}, inst)
	observation.outcome = "settled_attempt"
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			observation.finish(nil)
		}()
	}
	workers.Wait()
	data := collectPublishCleanupTestMetrics(t, reader)
	sum := data["xiaohuiji.publish.cleanup.attempts"].Data.(metricdata.Sum[int64])
	if len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != 1 {
		t.Fatalf("concurrent finish counted more than once: %+v", sum)
	}
}

func TestPublishCleanupObservationKeepsRestoredParent(t *testing.T) {
	inst, _, recorder := newPublishCleanupTestInstruments(t)
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1, 2, 3}, SpanID: trace.SpanID{4, 5, 6},
		TraceFlags: trace.FlagsSampled, Remote: true,
	})
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), parent)
	_, observation := beginPublishCleanupObservationWith(ctx, 7, tgQueuePayload{}, inst)
	observation.finish(nil)
	spans := recorder.Ended()
	if len(spans) != 1 || spans[0].Parent().SpanID() != parent.SpanID() ||
		spans[0].SpanContext().TraceID() != parent.TraceID() {
		t.Fatal("cleanup observation lost the restored execution parent")
	}
}

func TestPublishCleanupLogRedaction(t *testing.T) {
	for _, text := range []string{
		"https://example.invalid/file?signature=signed-secret",
		"postgres://user:db-secret@example.invalid/database",
		"password=plain-secret",
		`token="quoted-secret"`,
		"Authorization: Bearer bearer-secret",
		"api_key='key-secret'",
	} {
		if got := publishCleanupSafeText(text); strings.Contains(got, "-secret") {
			t.Fatalf("credential leaked: %q", got)
		}
	}
}
