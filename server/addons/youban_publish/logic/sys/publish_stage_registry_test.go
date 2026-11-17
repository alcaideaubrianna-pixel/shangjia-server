package sys

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"
)

func TestPublishExecutionClosesAbandonedStageExactlyOnce(t *testing.T) {
	ctx, registry := withPublishStageRegistry(context.Background())
	calls := 0
	step := &telegramPublishStage{finishObservation: func(err error, panicked bool) {
		calls++
		if !errors.Is(err, errPublishStageAbandoned) || panicked {
			t.Errorf("unfinished stage should be abandoned, got %v/%v", err, panicked)
		}
	}}
	registerPublishStage(ctx, step)
	registry.close(nil)
	step.End(nil)
	registry.close(nil)
	if calls != 1 || !step.ended.Load() {
		t.Fatalf("abandoned observation count=%d", calls)
	}
}

func TestPublishExecutionCloseRacingStageRegistration(t *testing.T) {
	ctx, registry := withPublishStageRegistry(context.Background())
	var observed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			step := &telegramPublishStage{finishObservation: func(error, bool) { observed.Add(1) }}
			registerPublishStage(ctx, step)
			step.End(nil)
		}()
	}
	registry.close(nil)
	wg.Wait()
	registry.close(nil)
	if observed.Load() != 64 {
		t.Fatalf("registration/close lost or duplicated stage observations: %d", observed.Load())
	}
}

func TestPublishAbandonedStageUsesBoundedMetricOutcome(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	ctx, registry := withPublishStageRegistry(context.Background())
	finish := startPublishStageObservationWith(ctx, "media.query",
		newPublishStageInstruments(provider.Meter("registry-test")), trace.NewNoopTracerProvider().Tracer("registry-test"))
	step := &telegramPublishStage{finishObservation: finish}
	registerPublishStage(ctx, step)
	registry.close(context.Canceled)
	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, scope := range data.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			if sum, ok := measurement.Data.(metricdata.Sum[int64]); ok {
				for _, point := range sum.DataPoints {
					assertPublishStageLabels(t, point.Attributes)
					outcome, _ := point.Attributes.Value("result")
					if outcome.AsString() != "abandoned" || point.Value != 1 {
						t.Fatalf("unfinished stage inherited execution result: %v", point)
					}
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("missing abandoned stage observation")
	}
}
