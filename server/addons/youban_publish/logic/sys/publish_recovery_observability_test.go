package sys

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestPublishRecoveryMetricsSeparateScanAndItemOutcomes(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })
	m := newPublishRecoveryInstruments(provider.Meter("publish-recovery-test"))
	m.step(ctx, "confirmed_job", 2*time.Second, nil)
	m.item(ctx, "complete_job", fmt.Errorf("private database failure"))
	m.item(ctx, "complete_job", fmt.Errorf("wrapped: %w", context.DeadlineExceeded))
	m.step(ctx, "private-step-id", time.Second, context.Canceled)
	m.confirmedPage(ctx, nil, time.Unix(1800000000, 0))
	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, scope := range data.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			seen[measurement.Name] = true
			switch value := measurement.Data.(type) {
			case metricdata.Sum[int64]:
				for _, point := range value.DataPoints {
					for _, attr := range point.Attributes.ToSlice() {
						switch attr.Key {
						case "step", "phase", "outcome":
						default:
							t.Fatalf("unexpected label %s", attr.Key)
						}
						if attr.Value.AsString() == "private-step-id" {
							t.Fatal("unbounded step label")
						}
					}
					if measurement.Name == "xiaohuiji.publish.recovery.items" {
						outcome, _ := point.Attributes.Value(attribute.Key("outcome"))
						if outcome.AsString() == "success" {
							t.Fatal("failed repair was reported as successful")
						}
					}
				}
			case metricdata.Histogram[float64]:
				if measurement.Unit != "s" {
					t.Fatalf("duration unit=%q", measurement.Unit)
				}
			case metricdata.Gauge[float64]:
				if measurement.Name == "xiaohuiji.publish.observation.updated_at" &&
					(len(value.DataPoints) != 1 || value.DataPoints[0].Value != 1800000000) {
					t.Fatal("freshness is not Unix seconds")
				}
			}
		}
	}
	for _, name := range []string{"xiaohuiji.publish.recovery.steps", "xiaohuiji.publish.recovery.items",
		"xiaohuiji.publish.recovery.duration", "xiaohuiji.publish.recovery.page_items",
		"xiaohuiji.publish.observation.updated_at"} {
		if !seen[name] {
			t.Fatalf("missing metric %s", name)
		}
	}
}

func TestPublishRecoveryOutcomeWrappedContextErrors(t *testing.T) {
	for _, test := range []struct {
		err  error
		want string
	}{
		{nil, "success"},
		{fmt.Errorf("wrapped: %w", context.Canceled), "canceled"},
		{fmt.Errorf("wrapped: %w", context.DeadlineExceeded), "timeout"},
		{fmt.Errorf("private-error"), "failed"},
	} {
		if got := publishRecoveryOutcome(test.err); got != test.want {
			t.Fatalf("outcome=%s want=%s", got, test.want)
		}
	}
}
