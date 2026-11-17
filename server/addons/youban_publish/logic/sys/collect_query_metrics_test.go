package sys

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestCollectQueryResultBounded(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{nil, "success"},
		{errors.New("SELECT private FROM tenant_123"), "failed"},
		{fmt.Errorf("wrapped: %w", context.Canceled), "canceled"},
		{fmt.Errorf("wrapped: %w", context.DeadlineExceeded), "timeout"},
		{fmt.Errorf("wrapped: %w", &collectQueryTestTimeout{}), "timeout"},
	} {
		if got := collectQueryResult(tc.err); got != tc.want {
			t.Fatalf("result=%q want=%q", got, tc.want)
		}
	}
}

type collectQueryTestTimeout struct{}

func (*collectQueryTestTimeout) Error() string   { return "private transport details" }
func (*collectQueryTestTimeout) Timeout() bool   { return true }
func (*collectQueryTestTimeout) Temporary() bool { return false }

func assertCollectQueryMetricLabels(t *testing.T, attrs []attribute.KeyValue, dimension string) {
	t.Helper()
	wantLabels := 3
	if dimension == "query_name" {
		wantLabels = 4
	}
	if len(attrs) != wantLabels {
		t.Fatalf("unexpected attributes: %v", attrs)
	}
	for _, attr := range attrs {
		value := attr.Value.AsString()
		switch string(attr.Key) {
		case "source_type", "collection_mode":
			if value != "unknown" {
				t.Fatalf("unexpected context dimension: %v", attr)
			}
		case "result":
			if dimension != "query_name" || value != "success" && value != "failed" && value != "timeout" && value != "canceled" {
				t.Fatalf("unbounded result: %v", attr)
			}
		case dimension:
			if dimension == "query_name" && value != "window_primary" && value != "window_retry" && value != "unknown" {
				t.Fatalf("unbounded query name: %v", attr)
			}
			if dimension == "reuse_result" && value != "hit" && value != "miss" {
				t.Fatalf("unbounded reuse result: %v", attr)
			}
		default:
			t.Fatalf("unexpected label (IDs and SQL are forbidden): %v", attr)
		}
	}
}

func TestCollectQueryMetricsManualReader(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })
	instruments := newCollectQueryInstruments(provider.Meter("collect-query-test"))
	instruments.recordQuery(ctx, collectWindowQueryPrimary, 250*time.Millisecond, 3, nil)
	instruments.recordQuery(ctx, collectWindowQueryPrimary, 500*time.Millisecond, 0, nil)
	instruments.recordQuery(ctx, collectWindowQueryRetry, time.Second, 2, nil)
	instruments.recordQuery(ctx, collectWindowQueryKind("SELECT private FROM tenant_123"), -time.Second, -1, nil)
	instruments.recordQuery(ctx, collectWindowQueryPrimary, time.Second, 0, errors.New("private SQL failure"))
	instruments.recordQuery(ctx, collectWindowQueryPrimary, time.Second, 0, fmt.Errorf("wrapped: %w", context.DeadlineExceeded))
	instruments.recordQuery(ctx, collectWindowQueryPrimary, time.Second, 9, fmt.Errorf("wrapped: %w", context.Canceled))
	instruments.recordReuse(ctx, collectBatchReuseHit)
	instruments.recordReuse(ctx, collectBatchReuseMiss)
	instruments.recordReuse(ctx, collectBatchReuseNotEvaluated)
	instruments.recordReuse(ctx, collectBatchReuseOutcome("account-123"))

	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	found := make(map[string]bool)
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			found[m.Name] = true
			if m.Description == "" {
				t.Fatalf("missing description: %s", m.Name)
			}
			switch m.Name {
			case "xiaohuiji.collect.query.executions":
				sum := m.Data.(metricdata.Sum[int64])
				var total int64
				for _, point := range sum.DataPoints {
					total += point.Value
					assertCollectQueryMetricLabels(t, point.Attributes.ToSlice(), "query_name")
				}
				if !sum.IsMonotonic || total != 7 {
					t.Fatalf("query executions=%d monotonic=%v", total, sum.IsMonotonic)
				}
			case "xiaohuiji.collect.query.duration_seconds":
				h := m.Data.(metricdata.Histogram[float64])
				var count uint64
				var sum float64
				for _, point := range h.DataPoints {
					count += point.Count
					sum += point.Sum
					assertCollectQueryMetricLabels(t, point.Attributes.ToSlice(), "query_name")
				}
				if m.Unit != "s" || count != 6 || sum != 4.75 {
					t.Fatalf("duration unit=%s count=%d sum=%v", m.Unit, count, sum)
				}
			case "xiaohuiji.collect.query.returned_rows":
				h := m.Data.(metricdata.Histogram[int64])
				var count uint64
				var sum int64
				for _, point := range h.DataPoints {
					count += point.Count
					sum += point.Sum
					assertCollectQueryMetricLabels(t, point.Attributes.ToSlice(), "query_name")
					result, ok := point.Attributes.Value(attribute.Key("result"))
					if !ok || result.AsString() != "success" {
						t.Fatalf("failed query recorded returned rows: %v", point.Attributes)
					}
				}
				if count != 3 || sum != 5 {
					t.Fatalf("returned rows count=%d sum=%d", count, sum)
				}
			case "xiaohuiji.collect.batch_reuse":
				sum := m.Data.(metricdata.Sum[int64])
				var total int64
				for _, point := range sum.DataPoints {
					total += point.Value
					assertCollectQueryMetricLabels(t, point.Attributes.ToSlice(), "reuse_result")
				}
				if !sum.IsMonotonic || total != 2 || len(sum.DataPoints) != 2 {
					t.Fatalf("reuse count=%d points=%d", total, len(sum.DataPoints))
				}
			default:
				t.Fatalf("unexpected metric %q", m.Name)
			}
		}
	}
	if len(found) != 4 {
		t.Fatalf("metrics=%v, want four instruments", found)
	}
}
