package sys

import (
	"bytes"
	"context"
	"testing"
	"time"

	"hotgo/internal/library/collectobserve"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestCollectProcessObservationPreservesUniquePayload(t *testing.T) {
	payload := collectProcessQueuePayload{TenantId: 1, SourceId: 351, EventId: 42, History: true}
	before, err := collectProcessTaskBody(payload)
	if err != nil {
		t.Fatal(err)
	}
	ctx := collectobserve.With(context.Background(), collectobserve.Metadata{
		EventID: 42, ReceivedAt: time.Now().Add(-time.Minute), SourceType: "bot",
	})
	unique := captureCollectProcessObservation(ctx, payload, true, time.Now())
	after, err := collectProcessTaskBody(unique)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("observability changed payload-hashed task uniqueness: %s / %s (%v)", before, after, err)
	}
	nonunique := captureCollectProcessObservation(ctx, payload, false, time.Now())
	restored := restoreCollectProcessObservation(context.Background(), nonunique, time.Now())
	meta := collectobserve.From(restored)
	timing, valid := collectobserve.QueueTimingFrom(restored)
	if meta.EventID != 42 || meta.Mode != "history" || meta.ReceivedAt.IsZero() || !valid || timing.EnqueuedAtUnixNano <= 0 {
		t.Fatalf("trusted clock/context not restored: %+v %+v", meta, timing)
	}
	nonunique.EventId = 43
	mismatched := restoreCollectProcessObservation(context.Background(), nonunique, time.Now())
	if meta := collectobserve.From(mismatched); meta.EventID != 43 || !meta.ReceivedAt.IsZero() {
		t.Fatalf("another event's observation leaked: %+v", meta)
	}
	payload.EventId = 0
	wake := captureCollectProcessObservation(ctx, payload, false, time.Now())
	if len(wake.Observation) != 0 {
		t.Fatal("source wake cannot inherit a single event's ingress clock")
	}
	wakeCtx := restoreCollectProcessObservation(ctx, wake, time.Now())
	if meta := collectobserve.From(wakeCtx); meta.EventID != 0 || !meta.ReceivedAt.IsZero() {
		t.Fatalf("source wake inherited stale worker event identity or clock: %+v", meta)
	}
	oldCtx := restoreCollectProcessObservation(ctx, unique, time.Now())
	if meta := collectobserve.From(oldCtx); meta.EventID != 42 || !meta.ReceivedAt.IsZero() {
		t.Fatalf("legacy/Unique task inherited an unproven ingress clock: %+v", meta)
	}
}

func TestCollectProcessQueueWaitUsesProducerClock(t *testing.T) {
	ctx := collectobserve.With(context.Background(), collectobserve.Metadata{
		TenantID: 123, SourceID: 351, ChatID: "-100123", SourceType: "bot", Mode: "history",
	})
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	instruments := newCollectProcessQueueInstruments(provider.Meter("isolated-process-queue"))
	now := time.Now().Truncate(time.Millisecond)
	instruments.record(ctx, now.Add(-5*time.Second).UnixNano(), now)
	instruments.record(ctx, 0, now)
	instruments.record(ctx, now.Add(time.Second).UnixNano(), now)
	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	foundWait, foundSamples := false, false
	for _, scope := range data.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			if measurement.Description == "" {
				t.Fatalf("missing description for %s", measurement.Name)
			}
			switch measurement.Name {
			case "xiaohuiji.collect.process.queue.wait_seconds":
				foundWait = true
				histogram := measurement.Data.(metricdata.Histogram[float64])
				if measurement.Unit != "s" || len(histogram.DataPoints) != 1 {
					t.Fatalf("unexpected wait histogram: %+v", measurement)
				}
				point := histogram.DataPoints[0]
				if point.Count != 1 || point.Sum != 5 || point.Attributes.Len() != 2 {
					t.Fatalf("missing/future clocks must not become zero waits: %+v", point)
				}
			case "xiaohuiji.collect.process.queue.wait_samples":
				foundSamples = true
				sum := measurement.Data.(metricdata.Sum[int64])
				if len(sum.DataPoints) != 3 {
					t.Fatalf("expected three sample categories: %+v", sum)
				}
				for _, point := range sum.DataPoints {
					if point.Value != 1 || point.Attributes.Len() != 3 {
						t.Fatalf("invalid labels or sample count: %+v", point)
					}
					for _, attr := range point.Attributes.ToSlice() {
						switch string(attr.Key) {
						case "source_type", "collection_mode", "sample":
						default:
							t.Fatalf("identity must not be a metric label: %v", attr)
						}
					}
				}
			default:
				t.Fatalf("unexpected metric: %s", measurement.Name)
			}
		}
	}
	if !foundWait || !foundSamples {
		t.Fatalf("missing queue instruments: wait=%v samples=%v", foundWait, foundSamples)
	}
}
