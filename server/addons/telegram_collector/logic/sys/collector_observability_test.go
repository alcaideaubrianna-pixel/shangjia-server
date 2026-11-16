package sys

import (
	"context"
	"errors"
	"testing"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestCollectorObservationResult(t *testing.T) {
	for _, tt := range []struct {
		result collectorObserveResult
		err    error
		want   collectorObserveResult
	}{
		{collectorResultOK, nil, collectorResultOK},
		{collectorResultAccepted, nil, collectorResultAccepted},
		{collectorResultSkipped, nil, collectorResultSkipped},
		{collectorResultError, nil, collectorResultError},
		{collectorResultDuplicate, nil, collectorResultDuplicate},
		{collectorResultOK, errors.New("failed"), collectorResultError},
		{collectorResultOK, context.Canceled, collectorResultCanceled},
		{collectorResultOK, context.DeadlineExceeded, collectorResultCanceled},
	} {
		if got := collectorObservationResult(tt.result, tt.err); got != tt.want {
			t.Fatalf("got %q, want %q", got, tt.want)
		}
	}
	// A nil observer is safe on early exits before stage initialization.
	endCollectorStage(nil, collectorResultSkipped, nil)
}

func TestCollectorObservationQueueClassConversion(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  collectorObserveQueueClass
	}{
		{"urgent", collectorQueueUrgent}, {"realtime", collectorQueueRealtime},
		{"history", collectorQueueHistory}, {"mixed", collectorQueueMixed},
		{"youban_account_615", "unknown"}, {"", "unknown"},
	} {
		if got := collectorObserveQueueClassFromClass(tt.input); got != tt.want {
			t.Fatalf("queue %q: got %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCollectorObservationDeliveryLaneUsesPayload(t *testing.T) {
	for _, tt := range []struct {
		name     string
		delivery sysin.CollectorDelivery
		want     collectorObserveQueueClass
	}{
		{"account_history", sysin.CollectorDelivery{SourceType: sysin.SourceTypeAccount, History: true}, collectorQueueHistory},
		{"account_realtime", sysin.CollectorDelivery{SourceType: sysin.SourceTypeAccount}, collectorQueueRealtime},
		{"bot_urgent", sysin.CollectorDelivery{SourceType: sysin.SourceTypeBot}, collectorQueueUrgent},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// No Asynq context exists on direct calls. The parsed delivery's
			// existing routing function must still choose the same lane.
			if got := collectorObserveQueueClassFromPriority(deliveryPriority(&tt.delivery)); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
	for _, tt := range []struct {
		priority int
		want     collectorObserveQueueClass
	}{
		{sysin.EventPriorityNormal, collectorQueueHistory},
		{sysin.EventPriorityRealtime, collectorQueueRealtime},
		{sysin.EventPriorityUrgent, collectorQueueUrgent},
	} {
		if got := collectorObserveQueueClassFromPriority(tt.priority); got != tt.want {
			t.Fatalf("priority %d: got %q, want %q", tt.priority, got, tt.want)
		}
	}
}

func TestCollectorObservationIngestAndDeliveryStages(t *testing.T) {
	for _, stage := range []collectorObserveStage{
		collectorStageIngestBot, collectorStageIngestAccount, collectorStageEventPersist,
		collectorStageDeliveryConsume, collectorStageDeliveryClaim, collectorStageDeliveryRead,
		collectorStageDeliveryEventRead, collectorStageDeliveryParse, collectorStageDeliveryComplete,
		collectorStageDeliveryPrepare, collectorStageEventComplete,
	} {
		if collectorBoundedStage(stage) != string(stage) {
			t.Fatalf("stage was omitted from whitelist: %q", stage)
		}
	}
	if collectorBoundedResult(collectorResultAccepted) != "accepted" {
		t.Fatal("accepted outcome was omitted from whitelist")
	}
}

func TestCollectorObservationFinishEnrichesSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	ctx, span := provider.Tracer("collector-test").Start(context.Background(), "test")
	observation := &collectorStageObservation{
		ctx: ctx, span: span, started: time.Now(), stage: collectorStageEventPersist,
	}
	// Simulate identities that become available only after the existing DB read.
	observation.meta = collectorObservationMeta{TenantID: 78, SourceID: 351, EventID: 12345}
	endCollectorStage(observation, collectorResultAccepted, nil)
	endCollectorStage(observation, collectorResultError, nil) // Deferred fallback cannot overwrite success.
	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("expected one completed span, got %d", len(ended))
	}
	values := make(map[attribute.Key]string)
	for _, item := range ended[0].Attributes() {
		values[item.Key] = item.Value.AsString()
	}
	if values["tenant_id"] != "78" || values["source_id"] != "351" ||
		values["event_id"] != "12345" || values["result"] != "accepted" {
		t.Fatalf("late metadata/result missing: %v", values)
	}
}

func TestCollectorObservationExplicitFailureMarksSpan(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	ctx, span := provider.Tracer("collector-test").Start(context.Background(), "test")
	observation := &collectorStageObservation{
		ctx: ctx, span: span, started: time.Now(), stage: collectorStageIngestBot,
	}
	// Models defer on panic: result=error even though named err is still nil.
	endCollectorStage(observation, collectorResultError, nil)
	ended := recorder.Ended()
	if len(ended) != 1 || ended[0].Status().Code != codes.Error {
		t.Fatal("explicit failure with nil err was not marked as span error")
	}
}

func TestCollectorObservationLabelsAreBounded(t *testing.T) {
	meta := collectorObservationMeta{
		QueueClass: "tenant-78-queue-123", SourceType: "source-351",
		TenantID: 78, AccountID: 615, SourceID: 351, EventID: 12345,
		DeliveryID: 67890, ChatID: "-100123456789", MessageID: 17,
	}
	attrs := meta.metricAttributes()
	if len(attrs) != 2 {
		t.Fatalf("unexpected metric labels: %v", attrs)
	}
	for _, attr := range attrs {
		if attr.Value.AsString() != "unknown" {
			t.Fatalf("unbounded metric value: %v", attr)
		}
		if attr.Key != "queue_class" && attr.Key != "source_type" {
			t.Fatalf("identity leaked into metrics: %v", attr)
		}
	}
	if collectorBoundedStage("event-12345") != "unknown" || collectorBoundedResult("error-private-message") != "unknown" {
		t.Fatal("unbounded stage/result was accepted")
	}
	// The same identities must remain available to the trace.
	if len(meta.traceAttributes()) != 9 {
		t.Fatalf("missing trace identity: %v", meta.traceAttributes())
	}
}

func TestCollectorDeliveryWaitSample(t *testing.T) {
	now := time.UnixMilli(1794888000000)
	for _, tt := range []struct {
		name   string
		epoch  int64
		now    time.Time
		result string
		wait   time.Duration
	}{
		{"legacy", 0, now, "missing", 0},
		{"missing_receipt", now.UnixMilli(), time.Time{}, "missing", 0},
		{"clock_skew", now.Add(time.Second).UnixMilli(), now, "future", 0},
		{"immediate", now.UnixMilli(), now, "valid", 0},
		{"scheduled", now.Add(-3 * time.Hour).UnixMilli(), now, "valid", 3 * time.Hour},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wait, result := collectorDeliveryWaitSample(tt.epoch, tt.now)
			if wait != tt.wait || result != tt.result {
				t.Fatalf("got (%s,%s), want (%s,%s)", wait, result, tt.wait, tt.result)
			}
		})
	}
}

func TestCollectorPipelineHistogramConfiguration(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	instruments, err := newCollectorPipelineInstruments(provider.Meter("collector-test"))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	instruments.duration.Record(ctx, .15)
	instruments.queueWait.Record(ctx, 35)
	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, scope := range data.ScopeMetrics {
		for _, series := range scope.Metrics {
			histogram, ok := series.Data.(metricdata.Histogram[float64])
			if !ok {
				continue
			}
			if series.Unit != "s" || len(histogram.DataPoints) != 1 {
				t.Fatalf("unexpected histogram: %+v", series)
			}
			point := histogram.DataPoints[0]
			if point.Count != 1 || len(point.Bounds) < 10 {
				t.Fatalf("missing sample or explicit buckets: %+v", point)
			}
			found++
		}
	}
	if found != 2 {
		t.Fatalf("expected duration and wait histograms, got %d", found)
	}
}
