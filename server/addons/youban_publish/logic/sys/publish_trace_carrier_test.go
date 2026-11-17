package sys

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// These tests use private providers/readers only: no global SDK replacement,
// exporters, production configuration, Redis, or database access.
func TestPublishTraceCarrierRoundTrip(t *testing.T) {
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2},
		TraceFlags: trace.FlagsSampled,
	})
	state, err := trace.ParseTraceState("vendor=value")
	if err != nil {
		t.Fatal(err)
	}
	parent = parent.WithTraceState(state)
	ctx := trace.ContextWithSpanContext(context.Background(), parent)
	carrier := capturePublishTrace(ctx)
	body, err := json.Marshal(tgQueuePayload{JobId: 7, Trace: carrier})
	if err != nil {
		t.Fatal(err)
	}
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	restored, payload, err := decodeTelegramQueuePayloadContext(base, asynq.NewTask(tgTaskTypePublish, body))
	if err != nil || payload.JobId != 7 {
		t.Fatalf("decode failed: job=%d err=%v", payload.JobId, err)
	}
	got := trace.SpanContextFromContext(restored)
	if got.TraceID() != parent.TraceID() || got.SpanID() != parent.SpanID() || !got.IsRemote() ||
		got.TraceState().String() != "vendor=value" || !got.IsSampled() {
		t.Fatal("W3C parent was not restored")
	}
	cancel()
	if !errors.Is(restored.Err(), context.Canceled) {
		t.Fatal("restoring metadata lost consumer cancellation")
	}
}

func TestPublishTraceCarrierBoundedAndOptional(t *testing.T) {
	carrier := &publishTraceCarrier{}
	carrier.Set("baggage", "credential=private")
	carrier.Set("Authorization", "private")
	carrier.Set("traceparent", strings.Repeat("x", 513))
	if carrier.Traceparent != "" || carrier.Get("baggage") != "" || len(carrier.Keys()) != 2 {
		t.Fatal("carrier accepted unbounded or unrelated headers")
	}
	for _, input := range []string{
		`{"jobId":7}`, `{"jobId":7,"trace":null}`,
		`{"jobId":7,"trace":{"traceparent":"invalid"}}`,
		`{"jobId":7,"trace":"invalid"}`,
		`{"jobId":7,"trace":{"traceparent":"` + strings.Repeat("x", 4096) + `"}}`,
	} {
		ctx := context.Background()
		got, payload, err := decodeTelegramQueuePayloadContext(ctx, asynq.NewTask(tgTaskTypePublish, []byte(input)))
		if err != nil || payload.JobId != 7 || got != ctx {
			t.Fatal("optional invalid metadata changed legacy task behavior")
		}
	}
	if capturePublishTrace(context.Background()) != nil {
		t.Fatal("empty context should omit telemetry")
	}
	unsampled := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}})
	restored := restorePublishTrace(context.Background(), capturePublishTrace(trace.ContextWithSpanContext(context.Background(), unsampled)))
	if got := trace.SpanContextFromContext(restored); !got.IsValid() || got.IsSampled() {
		t.Fatal("unsampled parent was lost or promoted")
	}
}

func TestPublishTraceDoesNotChangeUniqueDigest(t *testing.T) {
	const legacy = `{"jobId":7,"cleanupOperationNo":"operation","cleanupJobIds":[7,8],"forceCompletion":true}`
	payload := tgQueuePayload{JobId: 7, CleanupOperationNo: "operation", CleanupJobIds: []int64{7, 8}, ForceCompletion: true}
	ctx := trace.ContextWithSpanContext(context.Background(),
		trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}}))
	payload.Trace = capturePublishTrace(ctx)
	unique := telegramQueueTracePayload(ctx, payload, true)
	body, err := json.Marshal(unique)
	if err != nil {
		t.Fatal(err)
	}
	// Asynq v0.25.1 UniqueKey hashes the complete payload with MD5.
	if string(body) != legacy || md5.Sum(body) != md5.Sum([]byte(legacy)) {
		t.Fatal("unique task body/digest changed")
	}
	cleanup, err := telegramCleanupTaskBody(payload)
	if err != nil || string(cleanup) != legacy {
		t.Fatal("cleanup fallback changed unique body")
	}
	if telegramQueueTracePayload(ctx, payload, false).Trace == nil {
		t.Fatal("non-unique enqueue did not capture parent")
	}
	if payload.Trace == nil {
		t.Fatal("serialization mutated caller's payload")
	}
}

func TestPublishCleanupPersistedCarrierSurvivesRedelivery(t *testing.T) {
	parent := trace.NewSpanContext(trace.SpanContextConfig{TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2}})
	ctx := trace.ContextWithSpanContext(context.Background(), parent)
	body, err := json.Marshal(tgQueuePayload{JobId: 7, CleanupOperationNo: "operation", Trace: capturePublishTrace(ctx)})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := restoreTelegramCleanupIntentPayload(11, 7, "operation", string(body))
	if err != nil {
		t.Fatal(err)
	}
	restored := trace.SpanContextFromContext(restorePublishTrace(context.Background(), payload.Trace))
	if restored.TraceID() != parent.TraceID() || restored.SpanID() != parent.SpanID() {
		t.Fatal("existing outbox JSON did not preserve the enqueue parent")
	}
	taskBody, err := telegramCleanupTaskBody(payload)
	if err != nil || strings.Contains(string(taskBody), "trace") {
		t.Fatal("persisted trace entered the Asynq Unique digest")
	}
	if payload.Trace == nil {
		t.Fatal("task serialization removed the persisted parent from the hydrated payload")
	}
}

func TestPublishStageMetricsAndSpansExactlyOnce(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = mp.Shutdown(ctx); _ = tp.Shutdown(ctx) })
	instruments := newPublishStageInstruments(mp.Meter("publish-stage-test"))
	tracer := tp.Tracer("publish-stage-test")
	const sensitive = "https://private.invalid/path?token=private SELECT credential"
	for _, test := range []struct {
		stage    string
		err      error
		panicked bool
	}{
		{"queue.enqueue", nil, false},
		{"job.deferred", nil, false},
		{"display.api_send", errors.New(sensitive), false},
		{sensitive, errors.New(sensitive), false},
		{"queue.consume", context.Canceled, false},
		{"queue.consume", context.DeadlineExceeded, false},
		{"queue.enqueue", asynq.ErrDuplicateTask, false},
		{"queue.enqueue", asynq.ErrTaskIDConflict, false},
		{"queue.consume", &tgRetryAfterError{}, false},
		{"job.execute", nil, true},
	} {
		finish := startPublishStageObservationWith(ctx, test.stage, instruments, tracer)
		var group sync.WaitGroup
		for i := 0; i < 8; i++ {
			group.Add(1)
			go func() {
				defer group.Done()
				finish(test.err, test.panicked)
			}()
		}
		group.Wait()
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	var attempts, durationCount int64
	results := map[string]int64{}
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			switch m.Name {
			case "xiaohuiji.publish.stage.attempts":
				for _, point := range m.Data.(metricdata.Sum[int64]).DataPoints {
					assertPublishStageLabels(t, point.Attributes)
					attempts += point.Value
					result, _ := point.Attributes.Value("result")
					results[result.AsString()] += point.Value
				}
			case "xiaohuiji.publish.stage.duration_seconds":
				if m.Unit != "s" {
					t.Fatal("duration unit must be seconds")
				}
				for _, point := range m.Data.(metricdata.Histogram[float64]).DataPoints {
					assertPublishStageLabels(t, point.Attributes)
					durationCount += int64(point.Count)
					if point.Sum < 0 {
						t.Fatal("negative stage duration")
					}
				}
			}
		}
	}
	if attempts != 10 || durationCount != 10 || len(recorder.Ended()) != 10 {
		t.Fatalf("observations duplicated or missing: attempts=%d durations=%d spans=%d", attempts, durationCount, len(recorder.Ended()))
	}
	for result, want := range map[string]int64{
		"returned": 2, "error": 2, "canceled": 1, "deadline_exceeded": 1,
		"duplicate": 1, "conflict": 1, "retry": 1, "panic": 1,
	} {
		if results[result] != want {
			t.Fatalf("result %s: got %d want %d", result, results[result], want)
		}
	}
	for _, span := range recorder.Ended() {
		if strings.Contains(span.Name(), sensitive) || len(span.Events()) != 0 || strings.Contains(span.Status().Description, sensitive) {
			t.Fatal("span leaked raw errors or stage input")
		}
		attrs := attribute.NewSet(span.Attributes()...)
		stage, _ := attrs.Value("stage")
		result, _ := attrs.Value("result")
		assertPublishStageLabels(t, attribute.NewSet(
			attribute.String("stage", stage.AsString()), attribute.String("result", result.AsString())))
		if result.AsString() == "returned" && span.Status().Code != codes.Unset {
			t.Fatal("nil was promoted to business success")
		}
	}
}

func assertPublishStageLabels(t *testing.T, attrs attribute.Set) {
	t.Helper()
	if attrs.Len() != 2 {
		t.Fatal("unexpected stage metric dimensions")
	}
	stage, ok := attrs.Value("stage")
	if !ok || normalizePublishStage(stage.AsString()) != stage.AsString() {
		t.Fatal("unbounded stage")
	}
	result, ok := attrs.Value("result")
	if !ok {
		t.Fatal("missing result")
	}
	switch result.AsString() {
	case "returned", "error", "canceled", "deadline_exceeded", "duplicate", "conflict", "retry", "panic", "abandoned":
	default:
		t.Fatal("unbounded or business-success result")
	}
}

func TestPublishStageTimerEndAndPanicFinishOnce(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = mp.Shutdown(ctx); _ = tp.Shutdown(ctx) })
	instruments := newPublishStageInstruments(mp.Meter("timer-test"))
	newStage := func() *telegramPublishStage {
		return &telegramPublishStage{finishObservation: startPublishStageObservationWith(ctx, "job.execute", instruments, tp.Tracer("timer-test"))}
	}
	stage := newStage()
	stage.End(nil)
	stage.End(errors.New("ignored"))
	stage = newStage()
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("Finish swallowed panic")
			}
		}()
		var err error
		defer stage.Finish(&err)
		panic("private panic")
	}()
	stage.End(nil)
	if len(recorder.Ended()) != 2 || recorder.Ended()[1].Status().Code != codes.Error {
		t.Fatal("stage timer finish duplicated or panic span missing")
	}
}

func TestPublishExecutionRetriesAreSeparateSpans(t *testing.T) {
	ctx := context.Background()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })
	tracer := provider.Tracer("publish-execution-test")
	parentCtx, parent := tracer.Start(ctx, "enqueue")
	carrier := capturePublishTrace(parentCtx)
	parent.End()
	for i := 0; i < 2; i++ {
		consumerCtx := restorePublishTrace(ctx, carrier)
		_, span := beginTelegramPublishExecutionWith(consumerCtx, 7, tracer)
		finishTelegramPublishSpan(span, nil)
	}
	spans := recorder.Ended()
	if len(spans) != 3 {
		t.Fatal("missing execution spans")
	}
	first, retry := spans[1], spans[2]
	if first.Parent().SpanID() != parent.SpanContext().SpanID() ||
		retry.Parent().SpanID() != parent.SpanContext().SpanID() ||
		first.SpanContext().TraceID() != parent.SpanContext().TraceID() ||
		first.SpanContext().SpanID() == retry.SpanContext().SpanID() ||
		first.SpanKind() != trace.SpanKindConsumer {
		t.Fatal("deliveries must be separate consumer spans under the enqueue parent")
	}
}
