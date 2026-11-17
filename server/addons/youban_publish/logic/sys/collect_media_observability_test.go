package sys

import (
	"context"
	"errors"
	"testing"
	"time"

	"hotgo/internal/library/collectobserve"

	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestCollectMediaObservabilityContract(t *testing.T) {
	ctx := collectobserve.With(context.Background(), collectobserve.Metadata{Mode: "history"})
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })
	instruments := newCollectMediaInstruments(provider.Meter("collect-media-contract"))
	// Two media cache hits are independent of profile reuse or network traffic.
	instruments.reuse(ctx, "bot", "media_row")
	instruments.reuse(ctx, "account", "message_sibling")
	// A failed first download followed by a successful retry is one success.
	instruments.download(ctx, false, errors.New("network failure"), 250*time.Millisecond)
	instruments.download(ctx, true, nil, 1500*time.Millisecond)
	// Upload deduplication happens after downloading and has its own layer.
	instruments.reuse(ctx, "bot", "fingerprint_storage")
	instruments.delegate(ctx, nil)
	instruments.delegate(ctx, newCollectMediaFairnessRetryError("waiting", time.Second))
	instruments.reuse(collectobserve.With(context.Background(), collectobserve.Metadata{Mode: "event:123"}), "tenant:123", "profile:456")

	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	metrics := make(map[string]metricdata.Metrics)
	for _, scope := range data.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			if measurement.Description == "" {
				t.Errorf("metric %s has no description", measurement.Name)
			}
			metrics[measurement.Name] = measurement
		}
	}
	downloads := metrics["xiaohuiji.collect.media_download_attempts"].Data.(metricdata.Sum[int64])
	if len(downloads.DataPoints) != 2 {
		t.Fatalf("download series = %d, want 2", len(downloads.DataPoints))
	}
	for _, point := range downloads.DataPoints {
		if point.Value != 1 || point.Attributes.Len() != 4 {
			t.Fatalf("unexpected download point: %+v", point)
		}
		result, _ := point.Attributes.Value("result")
		attempt, _ := point.Attributes.Value("attempt")
		source, _ := point.Attributes.Value("source_type")
		mode, _ := point.Attributes.Value("collection_mode")
		if mode.AsString() != "history" {
			t.Fatalf("collection_mode = %q, want history", mode.AsString())
		}
		if source.AsString() != "bot" ||
			(result.AsString() == "error" && attempt.AsString() != "initial") ||
			(result.AsString() == "success" && attempt.AsString() != "retry") {
			t.Fatalf("unexpected download attributes: %v", point.Attributes)
		}
	}
	durations := metrics["xiaohuiji.collect.media_download_duration_seconds"]
	if durations.Unit != "s" {
		t.Fatalf("duration unit = %q, want s", durations.Unit)
	}
	var durationSum float64
	for _, point := range durations.Data.(metricdata.Histogram[float64]).DataPoints {
		durationSum += point.Sum
		wantBounds := []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600}
		if len(point.Bounds) != len(wantBounds) {
			t.Fatalf("histogram bounds = %v", point.Bounds)
		}
		for index, bound := range wantBounds {
			if point.Bounds[index] != bound {
				t.Fatalf("histogram bounds = %v", point.Bounds)
			}
		}
	}
	if durationSum != 1.75 {
		t.Fatalf("duration sum = %v, want 1.75 seconds", durationSum)
	}
	reuses := metrics["xiaohuiji.collect.media_reuses"].Data.(metricdata.Sum[int64])
	if len(reuses.DataPoints) != 4 {
		t.Fatalf("reuse series = %d, want 4", len(reuses.DataPoints))
	}
	for _, point := range reuses.DataPoints {
		source, _ := point.Attributes.Value("source_type")
		layer, _ := point.Attributes.Value("reuse_layer")
		mode, _ := point.Attributes.Value("collection_mode")
		if point.Attributes.Len() != 3 || point.Value != 1 {
			t.Fatalf("unexpected reuse point: %+v", point)
		}
		if source.AsString() == "unknown" && (layer.AsString() != "unknown" || mode.AsString() != "unknown") {
			t.Fatalf("unbounded reuse label: %v", point.Attributes)
		}
	}
	delegated := metrics["xiaohuiji.collect.media_delegated_results"].Data.(metricdata.Sum[int64])
	if len(delegated.DataPoints) != 2 {
		t.Fatalf("delegated series = %d, want 2", len(delegated.DataPoints))
	}
	for _, point := range delegated.DataPoints {
		result, _ := point.Attributes.Value("result")
		if point.Value != 1 || (result.AsString() != "success" && result.AsString() != "deferred") {
			t.Fatalf("unexpected delegated point: %+v", point)
		}
	}
}
