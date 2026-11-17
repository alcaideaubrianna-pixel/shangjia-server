package sys

import (
	"context"
	"testing"
	"time"

	"hotgo/internal/library/collectobserve"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestCollectStageMetricIdentityDoesNotBecomeLabels(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	meter := provider.Meter("collection-stage-cardinality")
	for _, chat := range []string{"-100123", "-100456"} {
		ctx := collectobserve.With(context.Background(), collectobserve.Metadata{
			SourceType: "bot", Mode: "history", ChatID: chat, SourceID: 351,
			EventID: 123, RuleID: 338, TenantID: 78, AccountID: 615,
		})
		recordCollectPipelineMetrics(ctx, meter, collectPipelineRulePreflight, collectPipelineCompleted, 10*time.Millisecond)
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, scope := range data.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			if sum, ok := measurement.Data.(metricdata.Sum[int64]); ok {
				if len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != 2 {
					t.Fatalf("channel identity split metric series: %+v", sum)
				}
				attrs := sum.DataPoints[0].Attributes
				source, _ := attrs.Value(attribute.Key("source_type"))
				mode, _ := attrs.Value(attribute.Key("collection_mode"))
				if attrs.Len() != 4 || source.AsString() != "bot" || mode.AsString() != "history" {
					t.Fatalf("unexpected metric dimensions: %+v", attrs)
				}
				found = true
			}
		}
	}
	if !found {
		t.Fatal("collection counter missing")
	}
}

func TestCollectStageKnownStagesDoNotCollapseToUnknown(t *testing.T) {
	for _, stage := range []collectPipelineStage{
		collectPipelineEventPersist, collectPipelineSourceAttempt, collectPipelineAuthorization,
		collectPipelineRulePreflight, collectPipelineRuleMatch, collectPipelineMaterialPair,
		collectPipelineMediaReady, collectPipelineMediaCache, collectPipelineMediaQuota,
		collectPipelineContentExtract, collectPipelineProfilePersist,
		collectPipelineWindowPrimary, collectPipelineWindowRetry,
	} {
		attrs := collectPipelineMetricAttributes(stage, collectPipelinePending)
		if attrs[0].Value.AsString() != string(stage) || attrs[1].Value.AsString() != "pending" {
			t.Fatalf("stage not exported as intended: %s %+v", stage, attrs)
		}
	}
}
