package sys

import (
	"context"
	"testing"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func newInventoryManualReader(t *testing.T, now func() time.Time) (*collectorInventoryObserver, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown: %v", err)
		}
	})
	observer, err := newCollectorInventoryObserver(provider.Meter("inventory-test"), now)
	if err != nil {
		t.Fatal(err)
	}
	return observer, reader
}

func collectInventoryValues(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	values := make(map[string]int64)
	for _, scope := range data.ScopeMetrics {
		for _, instrument := range scope.Metrics {
			gauge, ok := instrument.Data.(metricdata.Gauge[int64])
			if !ok {
				t.Fatalf("%s is %T, want int64 gauge", instrument.Name, instrument.Data)
			}
			for _, point := range gauge.DataPoints {
				if _, exists := values[instrument.Name]; exists {
					t.Fatalf("duplicate series for %s", instrument.Name)
				}
				attrs := point.Attributes.ToSlice()
				if len(attrs) != 3 {
					t.Fatalf("unbounded or missing attributes: %v", attrs)
				}
				want := map[string]string{
					"coverage": "instance_loaded", "loader": "account_runtime_provider", "entry_type": "protocol_account",
				}
				for _, attr := range attrs {
					expected, exists := want[string(attr.Key)]
					if !exists || attr.Value.AsString() != expected {
						t.Fatalf("unexpected inventory attribute: %v", attr)
					}
				}
				values[instrument.Name] = point.Value
			}
		}
	}
	return values
}

func assertInventoryValues(t *testing.T, reader *sdkmetric.ManualReader, valid, sources, entries int64) {
	t.Helper()
	values := collectInventoryValues(t, reader)
	if got, exists := values[collectorInventoryValidMetric]; !exists || got != valid {
		t.Fatalf("valid: got %v, want %d", values, valid)
	}
	if valid == 0 {
		if len(values) != 1 {
			t.Fatalf("invalid snapshot retained quantity samples: %v", values)
		}
		return
	}
	if got, exists := values[collectorInventorySourcesMetric]; !exists || got != sources {
		t.Fatalf("sources: got %v, want %d", values, sources)
	}
	if got, exists := values[collectorInventoryEntriesMetric]; !exists || got != entries {
		t.Fatalf("entries: got %v, want %d", values, entries)
	}
	if len(values) != 3 {
		t.Fatalf("unexpected inventory series: %v", values)
	}
}

func TestInventoryLoadedReplacementAndIdentity(t *testing.T) {
	now := time.Unix(1800000000, 0)
	observer, reader := newInventoryManualReader(t, func() time.Time { return now })
	assertInventoryValues(t, reader, 0, 0, 0)
	bindings := []*sysin.AccountRuntimeBinding{
		nil,
		{AccountID: 0, Sources: []sysin.AccountRuntimeSource{{TenantID: 1, SourceID: 99}}},
		{AccountID: 10, Sources: []sysin.AccountRuntimeSource{
			{TenantID: 1, AccountID: 500, SourceID: 1, ChatID: "private-chat"},
			{TenantID: 1, AccountID: 500, SourceID: 1, ChatID: "duplicate"},
			{TenantID: 2, AccountID: 500, SourceID: 1},
			{TenantID: 0, SourceID: 2},
			{TenantID: 1, SourceID: 0},
		}},
		{AccountID: 10, Sources: []sysin.AccountRuntimeSource{{TenantID: 1, SourceID: 2}}},
		{AccountID: 20, Sources: []sysin.AccountRuntimeSource{{TenantID: 1, AccountID: 500, SourceID: 2}}},
		{AccountID: 30}, // Listener/task-only runtime is not a collection entry.
	}
	observer.replace(context.Background(), bindings, true)
	assertInventoryValues(t, reader, 1, 3, 2)
	// The snapshot owns counts, never retains mutable provider slices.
	bindings[3].Sources = nil
	assertInventoryValues(t, reader, 1, 3, 2)
	// Shrinking and empty successful lists replace, rather than accumulate.
	observer.replace(context.Background(), bindings[4:5], true)
	assertInventoryValues(t, reader, 1, 1, 1)
	observer.replace(context.Background(), nil, true)
	assertInventoryValues(t, reader, 1, 0, 0)
}

func TestInventoryWithdrawsUnknownAndExpiredSnapshots(t *testing.T) {
	for _, reason := range []string{"load_error", "provider_absent", "canceled", "expired", "clock_reversed"} {
		t.Run(reason, func(t *testing.T) {
			now := time.Unix(1800000000, 0)
			observer, reader := newInventoryManualReader(t, func() time.Time { return now })
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			bindings := []*sysin.AccountRuntimeBinding{{
				AccountID: 10, Sources: []sysin.AccountRuntimeSource{{TenantID: 1, SourceID: 1}},
			}}
			observer.replace(ctx, bindings, true)
			assertInventoryValues(t, reader, 1, 1, 1)
			switch reason {
			case "load_error", "provider_absent":
				observer.replace(ctx, bindings, false)
			case "canceled":
				cancel()
			case "expired":
				now = now.Add(collectorInventoryMaxAge - time.Nanosecond)
				assertInventoryValues(t, reader, 1, 1, 1)
				now = now.Add(time.Nanosecond)
			case "clock_reversed":
				now = now.Add(-time.Second)
			}
			assertInventoryValues(t, reader, 0, 0, 0)
			// Repeated SDK collections cannot revive an obsolete observation.
			assertInventoryValues(t, reader, 0, 0, 0)
			observer.replace(context.Background(), nil, true)
			assertInventoryValues(t, reader, 1, 0, 0)
		})
	}
}

func TestInventoryReplicaSnapshotsAreIndependent(t *testing.T) {
	now := time.Unix(1800000000, 0)
	first, firstReader := newInventoryManualReader(t, func() time.Time { return now })
	second, secondReader := newInventoryManualReader(t, func() time.Time { return now })
	bindings := []*sysin.AccountRuntimeBinding{{
		AccountID: 10, Sources: []sysin.AccountRuntimeSource{{TenantID: 1, SourceID: 1}},
	}}
	first.replace(context.Background(), bindings, true)
	second.replace(context.Background(), bindings, true)
	// Both are local views of the same identity, not two global entries.
	assertInventoryValues(t, firstReader, 1, 1, 1)
	assertInventoryValues(t, secondReader, 1, 1, 1)
	first.replace(context.Background(), nil, true)
	assertInventoryValues(t, firstReader, 1, 0, 0)
	assertInventoryValues(t, secondReader, 1, 1, 1)
}
