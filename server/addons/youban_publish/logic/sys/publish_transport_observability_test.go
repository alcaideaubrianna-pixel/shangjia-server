package sys

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// No global provider replacement, database, queue, exporter, or background reader.
func TestPublishTransportMetricContract(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })
	instruments := newPublishTransportInstruments(provider.Meter("isolated-publish-transport"))
	instruments.request(ctx, "sendPhoto", "verify", "bot", "url", 250*time.Millisecond, nil, true)
	instruments.request(ctx, "sendPhoto", "verify", "bot", "binary", 750*time.Millisecond, nil, true)
	instruments.fallbackEvent(ctx, "queued", "verify", "account", "not_applicable", "unknown", "none")
	instruments.fallbackEvent(ctx, "completed", "verify", "account", "not_applicable", "failed", "other")

	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, scope := range data.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			seen[measurement.Name] = true
			if strings.TrimSpace(measurement.Description) == "" {
				t.Fatalf("%s has no description", measurement.Name)
			}
			switch measurement.Name {
			case "xiaohuiji.publish.api.requests":
				sum, ok := measurement.Data.(metricdata.Sum[int64])
				if !ok || !sum.IsMonotonic || len(sum.DataPoints) != 1 {
					t.Fatalf("request uploads must share one monotonic counter series: %#v", measurement.Data)
				}
				point := sum.DataPoints[0]
				if point.Value != 2 {
					t.Fatalf("requests = %d, want 2", point.Value)
				}
				assertPublishTransportKeys(t, point.Attributes, "method", "purpose", "transport", "outcome", "error_class")
			case "xiaohuiji.publish.api.duration":
				histogram, ok := measurement.Data.(metricdata.Histogram[float64])
				if !ok || measurement.Unit != "s" || len(histogram.DataPoints) != 2 {
					t.Fatalf("duration contract: unit=%q data=%#v", measurement.Unit, measurement.Data)
				}
				var count uint64
				var total float64
				for _, point := range histogram.DataPoints {
					assertPublishTransportKeys(t, point.Attributes, "method", "purpose", "transport", "upload_mode", "outcome")
					count += point.Count
					total += point.Sum
				}
				if count != 2 || total != 1 {
					t.Fatalf("duration count=%d sum=%g, want 2 and 1 second", count, total)
				}
			case "xiaohuiji.publish.fallback.events":
				sum, ok := measurement.Data.(metricdata.Sum[int64])
				if !ok || !sum.IsMonotonic || len(sum.DataPoints) != 2 {
					t.Fatalf("fallback contract: %#v", measurement.Data)
				}
				for _, point := range sum.DataPoints {
					assertPublishTransportKeys(t, point.Attributes, "event", "purpose", "transport", "upload_mode", "outcome", "error_class")
					event, _ := point.Attributes.Value(attribute.Key("event"))
					outcome, _ := point.Attributes.Value(attribute.Key("outcome"))
					if event.AsString() == "queued" && outcome.AsString() != "unknown" {
						t.Fatal("queued handoff became final failure")
					}
					if event.AsString() == "completed" && outcome.AsString() != "failed" {
						t.Fatal("terminal failure was lost")
					}
				}
			default:
				t.Fatalf("unexpected metric %q", measurement.Name)
			}
		}
	}
	if len(seen) != 3 {
		t.Fatalf("metric names: %v", seen)
	}
}

func assertPublishTransportKeys(t *testing.T, attrs attribute.Set, keys ...string) {
	t.Helper()
	if attrs.Len() != len(keys) {
		t.Fatalf("attribute count=%d want=%d: %v", attrs.Len(), len(keys), attrs)
	}
	for _, key := range keys {
		if _, ok := attrs.Value(attribute.Key(key)); !ok {
			t.Fatalf("missing %q in %v", key, attrs)
		}
	}
}

func TestPublishTransportBoundedLabels(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })
	instruments := newPublishTransportInstruments(provider.Meter("isolated-transport-cardinality"))
	for _, raw := range []string{"tenant-123", "https://private.example/token", "chat-456"} {
		instruments.request(ctx, raw, raw, raw, raw, time.Second, nil, false)
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	for _, scope := range data.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			if measurement.Name != "xiaohuiji.publish.api.requests" {
				continue
			}
			points := measurement.Data.(metricdata.Sum[int64]).DataPoints
			if len(points) != 1 || points[0].Value != 3 {
				t.Fatalf("unbounded labels: %#v", points)
			}
			for _, key := range []string{"purpose", "transport", "outcome", "error_class"} {
				value, _ := points[0].Attributes.Value(attribute.Key(key))
				if value.AsString() != "unknown" {
					t.Fatalf("%s=%q, want unknown", key, value.AsString())
				}
			}
		}
	}
}

func TestPublishTransportResult(t *testing.T) {
	for _, test := range []struct {
		name      string
		err       error
		confirmed bool
		outcome   string
		class     string
	}{
		{"confirmed", nil, true, "success", "none"},
		{"empty_or_partial_response", nil, false, "unknown", "unknown"},
		{"canceled", context.Canceled, false, "canceled", "canceled"},
		{"timeout", context.DeadlineExceeded, false, "timeout", "timeout"},
		{"uncertain", errTelegramDeliveryUncertain, false, "unknown", "ambiguous"},
		{"reconcile", errTelegramAttemptNeedsReconcile, false, "unknown", "ambiguous"},
		{"reset", errors.New("connection reset by peer"), false, "unknown", "ambiguous"},
		{"other", errors.New("request rejected"), false, "failed", "other"},
	} {
		t.Run(test.name, func(t *testing.T) {
			outcome, class := publishTransportResult(test.err, test.confirmed)
			if outcome != test.outcome || class != test.class {
				t.Fatalf("got %s/%s want %s/%s", outcome, class, test.outcome, test.class)
			}
		})
	}
}

func TestPublishAccountPhaseIsSeparateFromAPIRequestCounts(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })
	m := newPublishTransportInstruments(provider.Meter("isolated-account-phase"))
	m.accountPhase(ctx, "verify", 2*time.Second, nil, true)
	m.accountPhase(ctx, "verify", time.Second, nil, false)
	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, scope := range data.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			seen[measurement.Name] = true
			switch measurement.Name {
			case "xiaohuiji.publish.account.phase.attempts":
				sum := measurement.Data.(metricdata.Sum[int64])
				if len(sum.DataPoints) != 2 || !sum.IsMonotonic {
					t.Fatalf("account result series=%v", sum)
				}
				for _, point := range sum.DataPoints {
					assertPublishTransportKeys(t, point.Attributes, "purpose", "outcome", "error_class")
				}
			case "xiaohuiji.publish.account.phase.duration":
				if measurement.Unit != "s" {
					t.Fatalf("phase duration unit=%q", measurement.Unit)
				}
			default:
				t.Fatalf("phase operation leaked into unrelated API metric %s", measurement.Name)
			}
		}
	}
	if len(seen) != 2 {
		t.Fatalf("phase metric count=%d", len(seen))
	}
}

func TestPublishTransportRejectsEmptyOrInvalidMessageIDs(t *testing.T) {
	if publishTransportMessagesConfirmed(nil, 0) ||
		publishTransportMessagesConfirmed([]*models.Message{nil}, 1) ||
		publishTransportMessagesConfirmed([]*models.Message{{ID: 0}}, 1) ||
		publishTransportCopiesConfirmed([]models.MessageID{{ID: 0}}, 1) {
		t.Fatal("invalid response was treated as confirmed API success")
	}
	if !publishTransportMessagesConfirmed([]*models.Message{{ID: 1}}, 1) ||
		!publishTransportCopiesConfirmed([]models.MessageID{{ID: 1}}, 1) {
		t.Fatal("complete valid response was not confirmed")
	}
}

func TestPublishTransportDiagnosticEvent(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	ctx, span := provider.Tracer("isolated-transport-diagnostics").Start(context.Background(), "publish")
	traceID := span.SpanContext().TraceID().String()
	observePublishTransportDiagnostics(ctx, "copyMessages", "mixed", "bot", "copy", time.Second, nil, true)
	span.End()
	spans := recorder.Ended()
	if len(spans) != 1 || len(spans[0].Events()) != 1 {
		t.Fatalf("expected one call event: %v", spans)
	}
	event := spans[0].Events()[0]
	if event.Name != "publish.api.request" {
		t.Fatalf("event=%s", event.Name)
	}
	attrs := attribute.NewSet(event.Attributes...)
	assertPublishTransportKeys(t, attrs, "method", "purpose", "transport", "upload_mode", "outcome", "error_class", "duration_seconds", "trace_id")
	for key, want := range map[string]string{"method": "copyMessages", "purpose": "mixed", "transport": "bot", "upload_mode": "copy", "outcome": "success", "trace_id": traceID} {
		got, _ := attrs.Value(attribute.Key(key))
		if got.AsString() != want {
			t.Fatalf("%s=%q want=%q", key, got.AsString(), want)
		}
	}
}

func TestPublishTransportPreparedUploadModes(t *testing.T) {
	for _, test := range []struct {
		source string
		mode   string
	}{
		{"telegram-file-id", "file_id"},
		{"https://example.invalid/media?token=secret", "url"},
		{"http://example.invalid/media", "url"},
		{"attach://prepared", "binary"},
		{"", "not_applicable"},
	} {
		if got := publishTransportInputMode(&models.InputFileString{Data: test.source}); got != test.mode {
			t.Fatalf("%q: got %s want %s", test.source, got, test.mode)
		}
	}
	if got := publishTransportInputMode(&models.InputFileUpload{}); got != "binary" {
		t.Fatalf("upload=%s", got)
	}
	group := []models.InputMedia{
		&models.InputMediaPhoto{Media: "telegram-file-id"},
		&models.InputMediaVideo{Media: "attach://prepared"},
	}
	if got := publishTransportGroupMode(group); got != "mixed" {
		t.Fatalf("group=%s", got)
	}
	if got := publishTransportGroupMode(nil); got != "not_applicable" {
		t.Fatalf("empty group=%s", got)
	}
}
