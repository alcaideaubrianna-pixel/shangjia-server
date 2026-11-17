package sys

import (
	"context"
	"testing"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestCollectDeliverySkipIsBoundedAndSeparateFromSuccess(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })
	counter := newCollectDeliverySkipCounter(provider.Meter("isolated-skip"))
	delivery := &collectorin.CollectorDelivery{SourceID: 351, SourceType: "bot", History: true}
	recordCollectDeliverySkipped(ctx, counter, delivery, "approval_blocked")
	recordCollectDeliverySkipped(ctx, counter, delivery, "sensitive unbounded reason")
	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, scope := range data.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			if measurement.Name != "xiaohuiji.collect.delivery.skipped" || measurement.Unit != "{delivery}" || measurement.Description == "" {
				t.Fatalf("unexpected skip instrument: %+v", measurement)
			}
			for _, point := range measurement.Data.(metricdata.Sum[int64]).DataPoints {
				count += int(point.Value)
				if point.Attributes.Len() != 3 {
					t.Fatalf("unexpected dimensions: %+v", point)
				}
				for _, attr := range point.Attributes.ToSlice() {
					switch string(attr.Key) {
					case "reason":
						if attr.Value.AsString() != "approval_blocked" && attr.Value.AsString() != "unknown" {
							t.Fatalf("unbounded reason: %+v", attr)
						}
					case "source_type", "collection_mode":
					default:
						t.Fatalf("identity label: %+v", attr)
					}
				}
			}
		}
	}
	if count != 2 {
		t.Fatalf("skip attempts=%d want=2", count)
	}
}
