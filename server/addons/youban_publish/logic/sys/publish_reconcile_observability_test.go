package sys

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestPublishReconcileRestorationResult(t *testing.T) {
	for _, test := range []struct {
		name     string
		restored bool
		err      error
		want     string
	}{
		{"no_evidence_nil", false, nil, "unknown"},
		{"committed", true, nil, "confirmed"},
		{"positive_boolean_with_audit_error", true, fmt.Errorf("audit: %w", errTelegramAttemptAuditIncomplete), "incomplete"},
		{"transaction_failed", false, errors.New("commit failed"), "error"},
		{"positive_boolean_with_error", true, errors.New("commit failed"), "error"},
		{"canceled", false, fmt.Errorf("restore: %w", context.Canceled), "canceled"},
		{"deadline", false, fmt.Errorf("restore: %w", context.DeadlineExceeded), "timeout"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := publishReconcileRestorationResult(test.restored, test.err); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
	if got := publishReconcileResult("", nil); got != "unknown" {
		t.Fatalf("nil error without evidence became %q", got)
	}
	if got := publishReconcileResult("waiting", nil); got != "waiting" {
		t.Fatalf("waiting became %q", got)
	}
}

func TestPublishReconcileMetricsBoundedAndIsolated(t *testing.T) {
	// Never replace the process-global provider or touch production readers.
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })
	instruments := newPublishReconcileInstruments(provider.Meter("reconcile-test"))
	for _, source := range []string{"webhook", "account", "response", "aggregate", "unknown"} {
		instruments.record(ctx, source, "resolution", "waiting")
	}
	instruments.record(ctx, "credential=secret", "job:12345", "full error text")
	instruments.record(ctx, "other unbounded source", "attempt:67890", "different error")
	instruments.record(ctx, "unknown", "evidence_restore", "confirmed")
	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	var total int64
	seen := make(map[string]int64)
	for _, scope := range data.ScopeMetrics {
		for _, instrument := range scope.Metrics {
			if instrument.Name != "xiaohuiji.publish.reconcile.events" {
				t.Fatalf("unexpected instrument %q", instrument.Name)
			}
			sum, ok := instrument.Data.(metricdata.Sum[int64])
			if !ok || !sum.IsMonotonic || sum.Temporality != metricdata.CumulativeTemporality {
				t.Fatalf("unexpected counter aggregation %#v", instrument.Data)
			}
			for _, point := range sum.DataPoints {
				if point.Attributes.Len() != 3 {
					t.Fatalf("unexpected metric dimensions %v", point.Attributes)
				}
				value := func(key string) string {
					v, exists := point.Attributes.Value(attribute.Key(key))
					if !exists {
						t.Fatalf("missing attribute %q", key)
					}
					return v.AsString()
				}
				key := value("source") + "/" + value("event") + "/" + value("outcome")
				seen[key] += point.Value
				total += point.Value
			}
		}
	}
	if total != 8 || len(seen) != 7 {
		t.Fatalf("unexpected series/count: total=%d series=%v", total, seen)
	}
	for _, source := range []string{"webhook", "account", "response", "aggregate", "unknown"} {
		if seen[source+"/resolution/waiting"] != 1 {
			t.Fatalf("missing known source %q: %v", source, seen)
		}
	}
	if seen["unknown/unknown/unknown"] != 2 || seen["unknown/evidence_restore/confirmed"] != 1 {
		t.Fatalf("labels escaped bounds or confirmation lost: %v", seen)
	}
}

func TestPublishReconcileObservationScope(t *testing.T) {
	ctx := context.Background()
	setPublishReconcileObservation(ctx, "waiting", nil) // No scope is a no-op.
	observation := &publishReconcileObservation{jobID: 12, phase: "display_sending", outcome: "unknown"}
	scoped := context.WithValue(ctx, publishReconcileObservationKey{}, observation)
	setPublishReconcileObservation(scoped, "waiting", &telegramDeliveryAttempt{Id: 34, Phase: "display"})
	if observation.jobID != 12 || observation.attemptID != 34 || observation.phase != "display" || observation.outcome != "waiting" {
		t.Fatalf("unexpected observation: %+v", observation)
	}
	setPublishReconcileObservation(scoped, "resumed", nil)
	if observation.attemptID != 34 || observation.phase != "display" || observation.outcome != "resumed" {
		t.Fatalf("lost available attempt context: %+v", observation)
	}
	// A cause swallowed by claim release remains an observed failure.
	setPublishReconcileObservation(scoped, publishReconcileResult("unknown", context.DeadlineExceeded), nil)
	if publishReconcileResult(observation.outcome, nil) != "timeout" {
		t.Fatal("claim release erased the resolution failure")
	}
}
