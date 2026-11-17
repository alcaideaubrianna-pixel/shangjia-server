package sys

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func collectorTelemetryFixture(t *testing.T, sampler sdktrace.Sampler) (collectorPipelineInstruments, *sdkmetric.ManualReader, trace.Tracer, *tracetest.SpanRecorder) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	metrics := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	recorder := tracetest.NewSpanRecorder()
	traces := sdktrace.NewTracerProvider(sdktrace.WithSampler(sampler), sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		_ = traces.Shutdown(context.Background())
		_ = metrics.Shutdown(context.Background())
	})
	instruments, err := newCollectorPipelineInstruments(metrics.Meter("collector-transport-isolated"))
	if err != nil {
		t.Fatal(err)
	}
	// Never replace global providers, collectorMeter, instruments or sync.Once.
	return instruments, reader, traces.Tracer("collector-transport-isolated"), recorder
}

func collectorTelemetryMeasurements(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
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

func collectorTelemetrySum(t *testing.T, measurements map[string]metricdata.Metrics, name string) metricdata.Sum[int64] {
	t.Helper()
	data, ok := measurements[name].Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("missing integer sum %s: %+v", name, measurements[name])
	}
	return data
}

func collectorTelemetryHistogram(t *testing.T, measurements map[string]metricdata.Metrics, name string) metricdata.Histogram[float64] {
	t.Helper()
	measurement := measurements[name]
	data, ok := measurement.Data.(metricdata.Histogram[float64])
	if !ok || measurement.Unit != "s" {
		t.Fatalf("missing seconds histogram %s: %+v", name, measurement)
	}
	return data
}

func collectorTelemetryAttribute(set attribute.Set, key string) string {
	value, _ := set.Value(attribute.Key(key))
	return value.AsString()
}

func TestCollectorTelemetryFinishOnceIndependentOfSpanSampling(t *testing.T) {
	for _, sampled := range []bool{false, true} {
		name, sampler := "unsampled", sdktrace.NeverSample()
		if sampled {
			name, sampler = "sampled", sdktrace.AlwaysSample()
		}
		t.Run(name, func(t *testing.T) {
			instruments, reader, tracer, recorder := collectorTelemetryFixture(t, sampler)
			_, observation := startCollectorStageWithInstruments(context.Background(), collectorStageDeliveryHandler,
				collectorObservationMeta{SourceType: collectorSourceAccount, Mode: collectorModeHistory, QueueClass: collectorQueueHistory},
				instruments, tracer)
			observation.Finish(collectorResultAccepted, nil)
			observation.Finish(collectorResultError, errors.New("late fallback"))
			measurements := collectorTelemetryMeasurements(t, reader)
			for _, name := range []string{"telegram_collector_pipeline_operations", "xiaohuiji.collect.transport.stage.started"} {
				sum := collectorTelemetrySum(t, measurements, name)
				if len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != 1 || !sum.IsMonotonic {
					t.Fatalf("%s counted twice or disappeared without sampling: %+v", name, sum)
				}
			}
			completed := collectorTelemetrySum(t, measurements, "telegram_collector_pipeline_operations").DataPoints[0]
			if collectorTelemetryAttribute(completed.Attributes, "result") != "accepted" {
				t.Fatal("deferred fallback overwrote explicit accepted outcome")
			}
			active := collectorTelemetrySum(t, measurements, "xiaohuiji.collect.transport.stage.active")
			if len(active.DataPoints) != 1 || active.DataPoints[0].Value != 0 || active.IsMonotonic {
				t.Fatalf("active stage did not balance: %+v", active)
			}
			duration := collectorTelemetryHistogram(t, measurements, "telegram_collector_pipeline_duration_seconds")
			if len(duration.DataPoints) != 1 || duration.DataPoints[0].Count != 1 || duration.DataPoints[0].Sum < 0 {
				t.Fatalf("duration counted twice: %+v", duration)
			}
			wantSpans := 0
			if sampled {
				wantSpans = 1
			}
			if len(recorder.Ended()) != wantSpans {
				t.Fatalf("ended spans=%d want=%d", len(recorder.Ended()), wantSpans)
			}
		})
	}
}

func TestCollectorTelemetryConcurrentFinishCountsOneAttempt(t *testing.T) {
	instruments, reader, tracer, recorder := collectorTelemetryFixture(t, sdktrace.AlwaysSample())
	_, observation := startCollectorStageWithInstruments(context.Background(), collectorStageEventConsume,
		collectorObservationMeta{}, instruments, tracer)
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			observation.Finish(collectorResultOK, nil)
		}()
	}
	workers.Wait()
	measurements := collectorTelemetryMeasurements(t, reader)
	sum := collectorTelemetrySum(t, measurements, "telegram_collector_pipeline_operations")
	if len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != 1 || len(recorder.Ended()) != 1 {
		t.Fatal("concurrent Finish counted or ended the same stage more than once")
	}
}

func TestCollectorTelemetryActiveUsesStartLabelsAfterEnrichment(t *testing.T) {
	instruments, reader, tracer, _ := collectorTelemetryFixture(t, sdktrace.NeverSample())
	_, observation := startCollectorStageWithInstruments(context.Background(), collectorStageEventConsume,
		collectorObservationMeta{}, instruments, tracer)
	before := collectorTelemetrySum(t, collectorTelemetryMeasurements(t, reader), "xiaohuiji.collect.transport.stage.active")
	if len(before.DataPoints) != 1 || before.DataPoints[0].Value != 1 {
		t.Fatal("running stage was not observed")
	}
	observation.meta = collectorObservationMeta{
		QueueClass: collectorQueueHistory, SourceType: collectorSourceBot, Mode: collectorModeHistory,
		TenantID: 78, SourceID: 351, EventID: 123,
	}
	observation.Finish(collectorResultOK, nil)
	measurements := collectorTelemetryMeasurements(t, reader)
	active := collectorTelemetrySum(t, measurements, "xiaohuiji.collect.transport.stage.active")
	if len(active.DataPoints) != 1 || active.DataPoints[0].Value != 0 ||
		collectorTelemetryAttribute(active.DataPoints[0].Attributes, "source_type") != "unknown" {
		t.Fatalf("late metadata left an unmatched active series: %+v", active)
	}
	completed := collectorTelemetrySum(t, measurements, "telegram_collector_pipeline_operations")
	if collectorTelemetryAttribute(completed.DataPoints[0].Attributes, "source_type") != "bot" ||
		collectorTelemetryAttribute(completed.DataPoints[0].Attributes, "collection_mode") != "history" {
		t.Fatal("completed observation lost enriched metadata")
	}
}

type collectorTelemetryTimeoutError struct{}

func (collectorTelemetryTimeoutError) Error() string {
	return "private token https://private.invalid/media"
}
func (collectorTelemetryTimeoutError) Timeout() bool { return true }

func TestCollectorTelemetryErrorsAndLabelsAreBounded(t *testing.T) {
	for _, tc := range []struct {
		name      string
		result    collectorObserveResult
		err       error
		want      string
		errorType string
	}{
		{"success", collectorResultAccepted, nil, "accepted", "none"},
		{"skip", collectorResultSkipped, nil, "skipped", "none"},
		{"panic_fallback", collectorResultError, nil, "error", "error"},
		{"canceled", collectorResultOK, errors.Join(errors.New("private"), context.Canceled), "canceled", "canceled"},
		{"deadline", collectorResultOK, errors.Join(errors.New("private"), context.DeadlineExceeded), "timeout", "deadline_exceeded"},
		{"network_timeout", collectorResultOK, collectorTelemetryTimeoutError{}, "timeout", "timeout"},
		{"ordinary_error", collectorResultOK, errors.New("private URL"), "error", "error"},
		{"invalid_result", "tenant-78", nil, "unknown", "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			instruments, reader, _, _ := collectorTelemetryFixture(t, sdktrace.NeverSample())
			instruments.recordStage(context.Background(), "source-351", collectorObservationMeta{
				QueueClass: "queue-615", SourceType: "tenant-78", Mode: "chat-123",
				TenantID: 78, AccountID: 615, BotID: 109, SourceID: 351, EventID: 123,
				ChatID: "-1003736052878", MessageID: 2117,
			}, tc.result, 250*time.Millisecond, tc.err)
			measurements := collectorTelemetryMeasurements(t, reader)
			point := collectorTelemetrySum(t, measurements, "telegram_collector_pipeline_operations").DataPoints[0]
			want := map[string]string{
				"stage": "unknown", "queue_class": "unknown", "source_type": "unknown",
				"collection_mode": "unknown", "result": tc.want, "error_type": tc.errorType,
			}
			if point.Attributes.Len() != len(want) {
				t.Fatalf("unexpected metric labels: %v", point.Attributes)
			}
			for _, attr := range point.Attributes.ToSlice() {
				if attr.Value.AsString() != want[string(attr.Key)] {
					t.Fatalf("identity/error detail leaked to labels: %v", attr)
				}
			}
			histogram := collectorTelemetryHistogram(t, measurements, "telegram_collector_pipeline_duration_seconds")
			if len(histogram.DataPoints) != 1 || histogram.DataPoints[0].Sum != .25 {
				t.Fatal("execution duration was not recorded in seconds")
			}
		})
	}
}

func TestCollectorTelemetryQueueWaitOnceAndMissingIsNotZero(t *testing.T) {
	for _, stage := range []collectorObserveStage{collectorStageEventConsume, collectorStageDeliveryConsume} {
		t.Run(string(stage), func(t *testing.T) {
			instruments, reader, _, _ := collectorTelemetryFixture(t, sdktrace.NeverSample())
			now := time.UnixMilli(1794888000000)
			meta := collectorObservationMeta{QueueClass: collectorQueueHistory, SourceType: collectorSourceAccount, Mode: collectorModeHistory}
			for _, epoch := range []int64{now.Add(-5500 * time.Millisecond).UnixMilli(), 0, now.Add(time.Second).UnixMilli()} {
				wait := newCollectorQueueWaitObservation(context.Background(), stage, meta, epoch, now, instruments)
				wait.Finish(collectorObservationMeta{})
				wait.Finish(meta)
			}
			measurements := collectorTelemetryMeasurements(t, reader)
			histogramName, samplesName := "telegram_collector_pipeline_event_queue_wait_seconds", "telegram_collector_event_wait_samples"
			if stage == collectorStageDeliveryConsume {
				histogramName, samplesName = "telegram_collector_pipeline_delivery_queue_wait_seconds", "telegram_collector_delivery_wait_samples"
			}
			samples := collectorTelemetrySum(t, measurements, samplesName)
			if len(samples.DataPoints) != 3 {
				t.Fatalf("availability samples=%+v, want valid/missing/future", samples)
			}
			for _, point := range samples.DataPoints {
				sample := collectorTelemetryAttribute(point.Attributes, "sample")
				if point.Value != 1 || (sample != "valid" && sample != "missing" && sample != "future") ||
					collectorTelemetryAttribute(point.Attributes, "queue_class") != "history" {
					t.Fatalf("invalid sample count or lost start metadata: %+v", point)
				}
			}
			histogram := collectorTelemetryHistogram(t, measurements, histogramName)
			if len(histogram.DataPoints) != 1 || histogram.DataPoints[0].Count != 1 || histogram.DataPoints[0].Sum != 5.5 {
				t.Fatalf("missing/future waits became fake histogram samples: %+v", histogram)
			}
			// An unsupported stage must not accidentally emit a delivery wait.
			newCollectorQueueWaitObservation(context.Background(), collectorStageDeliveryHandler, meta, 0, now, instruments).Finish(meta)
			after := collectorTelemetrySum(t, collectorTelemetryMeasurements(t, reader), samplesName)
			for _, point := range after.DataPoints {
				if point.Value != 1 {
					t.Fatal("unsupported stage emitted a queue sample")
				}
			}
		})
	}
}

func TestCollectorTelemetryUnavailableDurationStillCountsOutcome(t *testing.T) {
	instruments, reader, _, _ := collectorTelemetryFixture(t, sdktrace.NeverSample())
	instruments.recordStage(context.Background(), collectorStageDeliveryRead, collectorObservationMeta{}, collectorResultSkipped, -1, nil)
	measurements := collectorTelemetryMeasurements(t, reader)
	sum := collectorTelemetrySum(t, measurements, "telegram_collector_pipeline_operations")
	if len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != 1 {
		t.Fatal("unavailable timing erased stage outcome")
	}
	if _, exists := measurements["telegram_collector_pipeline_duration_seconds"]; exists {
		t.Fatal("unavailable duration manufactured a latency sample")
	}
}
