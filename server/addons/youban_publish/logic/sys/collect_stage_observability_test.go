package sys

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"hotgo/internal/library/collectobserve"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type collectPipelineTestTimeoutError struct{}

func (collectPipelineTestTimeoutError) Error() string { return "private network address" }
func (collectPipelineTestTimeoutError) Timeout() bool { return true }

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
				if attrs.Len() != 6 || source.AsString() != "bot" || mode.AsString() != "history" {
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

func TestCollectPipelineObservationClassification(t *testing.T) {
	for _, test := range []struct {
		name   string
		result collectPipelineResult
		err    error
		reason string
		want   collectPipelineObservation
	}{
		{"nil_is_unconfirmed", collectPipelineUnconfirmed, nil, "", collectPipelineObservation{collectPipelineUnconfirmed, collectErrorNone, collectWaitNone}},
		{"explicit_receipt", collectPipelineQueued, nil, "", collectPipelineObservation{collectPipelineQueued, collectErrorNone, collectWaitNone}},
		{"waiting_media", collectPipelinePending, nil, "media_cache_pending", collectPipelineObservation{collectPipelinePending, collectErrorNone, collectWaitMediaCache}},
		{"waiting_without_reason", collectPipelinePending, nil, "", collectPipelineObservation{collectPipelinePending, collectErrorNone, collectWaitUnspecified}},
		{"unbounded_reason", collectPipelinePending, nil, "tenant78 signed-url private-text", collectPipelineObservation{collectPipelinePending, collectErrorNone, collectWaitUnknown}},
		{"success_discards_wait", collectPipelineCompleted, nil, "lease_busy", collectPipelineObservation{collectPipelineCompleted, collectErrorNone, collectWaitNone}},
		{"wrapped_timeout", collectPipelineCompleted, fmt.Errorf("wrapped: %w", context.DeadlineExceeded), "", collectPipelineObservation{collectPipelineTimeout, collectErrorDeadline, collectWaitNone}},
		{"network_timeout", collectPipelineCompleted, fmt.Errorf("wrapped: %w", collectPipelineTestTimeoutError{}), "", collectPipelineObservation{collectPipelineTimeout, collectErrorTimeout, collectWaitNone}},
		{"wrapped_cancel", collectPipelineCompleted, fmt.Errorf("wrapped: %w", context.Canceled), "", collectPipelineObservation{collectPipelineCanceled, collectErrorCanceled, collectWaitNone}},
		{"retry_is_not_failure", collectPipelineFailed, newCollectProcessRetryError(time.Second, "private reason"), "", collectPipelineObservation{collectPipelineRetryable, collectErrorRetryable, collectWaitBackoff}},
		{"retry_lease", collectPipelineFailed, newCollectProcessRetryError(time.Second, "private reason"), "lease_busy", collectPipelineObservation{collectPipelineRetryable, collectErrorRetryable, collectWaitLease}},
		{"real_error_beats_wait", collectPipelinePending, errors.New("private SQL"), "media_incomplete", collectPipelineObservation{collectPipelineFailed, collectErrorOther, collectWaitNone}},
		{"failed_without_error", collectPipelineFailed, nil, "", collectPipelineObservation{collectPipelineFailed, collectErrorUnknown, collectWaitNone}},
		{"invalid_result", "private-result", nil, "", collectPipelineObservation{collectPipelineUnknown, collectErrorNone, collectWaitNone}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := classifyCollectPipelineObservation(test.result, test.err, attribute.String("collect.wait_reason", test.reason))
			if got != test.want {
				t.Fatalf("classification=%+v want=%+v", got, test.want)
			}
		})
	}
}

func TestCollectPipelineFinishOnceAndUnsampledMetrics(t *testing.T) {
	for _, sampled := range []bool{false, true} {
		t.Run(fmt.Sprintf("sampled_%t", sampled), func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			recorder := tracetest.NewSpanRecorder()
			sampler := sdktrace.NeverSample()
			if sampled {
				sampler = sdktrace.AlwaysSample()
			}
			tp := sdktrace.NewTracerProvider(sdktrace.WithSampler(sampler), sdktrace.WithSpanProcessor(recorder))
			t.Cleanup(func() {
				_ = tp.Shutdown(context.Background())
				_ = mp.Shutdown(context.Background())
			})
			ctx := collectobserve.With(context.Background(), collectobserve.Metadata{
				TenantID: 78, AccountID: 615, SourceID: 351, EventID: 19344635,
				ChatID: "-1003763589169", SourceType: "bot", Mode: "history",
			})
			instruments := newCollectPipelineInstruments(mp.Meter("finish-once"))
			_, finish := startCollectPipelineStageWith(ctx, collectPipelineMediaReady, instruments, tp.Tracer("finish-once"))
			finish(collectPipelinePending, nil,
				attribute.String("collect.wait_reason", "media_incomplete"),
				attribute.String("stage", "private-stage"),
				attribute.String("error_kind", "private-error"),
				attribute.String("source_type", "private-source"))
			var wg sync.WaitGroup
			for i := 0; i < 16; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					finish(collectPipelineFailed, errors.New("must not be recorded"))
				}()
			}
			wg.Wait()
			var data metricdata.ResourceMetrics
			if err := reader.Collect(context.Background(), &data); err != nil {
				t.Fatal(err)
			}
			var count int64
			var durations uint64
			for _, scope := range data.ScopeMetrics {
				for _, measurement := range scope.Metrics {
					switch value := measurement.Data.(type) {
					case metricdata.Sum[int64]:
						for _, point := range value.DataPoints {
							count += point.Value
							result, _ := point.Attributes.Value("result")
							reason, _ := point.Attributes.Value("wait_reason")
							kind, _ := point.Attributes.Value("error_kind")
							if point.Attributes.Len() != 6 || result.AsString() != "pending" || reason.AsString() != "media_incomplete" || kind.AsString() != "none" {
								t.Fatalf("unbounded or overwritten dimensions: %v", point.Attributes)
							}
						}
					case metricdata.Histogram[float64]:
						for _, point := range value.DataPoints {
							durations += point.Count
						}
					}
				}
			}
			if count != 1 || durations != 1 {
				t.Fatalf("finish recorded more than once or lost unsampled metrics: count=%d durations=%d", count, durations)
			}
			spans := recorder.Ended()
			if !sampled {
				if len(spans) != 0 {
					t.Fatal("NeverSample unexpectedly exported a span")
				}
				return
			}
			if len(spans) != 1 || spans[0].Name() != "collect.media_ready" || spans[0].Status().Code != codes.Unset {
				t.Fatalf("pending stage must end once without error: %+v", spans)
			}
			for _, attr := range spans[0].Attributes() {
				if attr.Value.Emit() == "private-stage" || attr.Value.Emit() == "private-error" || attr.Value.Emit() == "private-source" {
					t.Fatalf("caller overwrote bounded telemetry: %v", attr)
				}
			}
		})
	}
}

func TestCollectPipelineObservationErrorMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	instruments := newCollectPipelineInstruments(provider.Meter("outcome-classification"))
	for _, err := range []error{context.Canceled, context.DeadlineExceeded, newCollectProcessRetryError(time.Second, "private retry"), errors.New("private SQL")} {
		instruments.record(context.Background(), collectPipelineProcess,
			classifyCollectPipelineObservation(collectPipelineCompleted, err), time.Millisecond)
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, scope := range data.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			if value, ok := measurement.Data.(metricdata.Sum[int64]); ok {
				for _, point := range value.DataPoints {
					result, _ := point.Attributes.Value("result")
					kind, _ := point.Attributes.Value("error_kind")
					got[result.AsString()] = kind.AsString()
					if point.Value != 1 || point.Attributes.Len() != 6 {
						t.Fatalf("unexpected error counter: %+v", point)
					}
				}
			}
		}
	}
	for result, kind := range map[string]string{"canceled": "canceled", "timeout": "deadline_exceeded", "retryable": "retryable", "failed": "other"} {
		if got[result] != kind {
			t.Fatalf("result=%s kind=%q want=%q", result, got[result], kind)
		}
	}
}

func TestCollectStageKnownStagesDoNotCollapseToUnknown(t *testing.T) {
	for _, stage := range []collectPipelineStage{
		collectPipelineEventPersist, collectPipelineSourceAttempt, collectPipelineAuthorization,
		collectPipelineRulePreflight, collectPipelineRuleMatch, collectPipelineMaterialPair,
		collectPipelineMediaReady, collectPipelineMediaCache, collectPipelineMediaQuota,
		collectPipelineContentExtract, collectPipelineProfilePersist,
		collectPipelineWindowPrimary, collectPipelineWindowRetry,
		collectPipelineProcessConsume,
	} {
		attrs := collectPipelineMetricAttributes(stage, collectPipelinePending)
		if attrs[0].Value.AsString() != string(stage) || attrs[1].Value.AsString() != "pending" {
			t.Fatalf("stage not exported as intended: %s %+v", stage, attrs)
		}
	}
}

func TestCollectProfilePersistenceRequiresDurableCommitAndTrustedIngress(t *testing.T) {
	finishedAt := time.Unix(1794960000, 0)
	for _, test := range []struct {
		name             string
		created, durable bool
		profileID        int64
		commitErr        error
		receivedAt       time.Time
		wantResult       collectPipelineResult
		wantAction       string
		wantMissing      string
		wantSeconds      float64
	}{
		{"new_profile", true, true, 831873, nil, finishedAt.Add(-2 * time.Second), collectPipelinePersisted, "created", "", 2},
		{"existing_profile", false, true, 831873, nil, finishedAt.Add(-time.Hour), collectPipelinePersisted, "reused", "", 0},
		{"legacy_sent_early_return_is_reused", false, true, 831873, nil, finishedAt.Add(-time.Hour), collectPipelinePersisted, "reused", "", 0},
		{"outer_transaction_not_committed", true, false, 831873, nil, finishedAt.Add(-time.Second), collectPipelineUnconfirmed, "", "transaction_unconfirmed", 0},
		{"nested_reuse_not_durable", false, false, 831873, nil, finishedAt.Add(-time.Second), collectPipelineUnconfirmed, "", "transaction_unconfirmed", 0},
		{"rollback", true, true, 831873, errors.New("transaction rolled back"), finishedAt.Add(-time.Second), collectPipelineFailed, "", "", 0},
		{"invalid_profile_id", true, true, 0, nil, finishedAt.Add(-time.Second), collectPipelineUnconfirmed, "", "", 0},
		{"missing_trusted_ingress", true, true, 831873, nil, time.Time{}, collectPipelinePersisted, "created", "missing_received_at", 0},
		{"future_ingress", true, true, 831873, nil, finishedAt.Add(time.Second), collectPipelinePersisted, "created", "future_received_at", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
			instruments := newCollectPipelineInstruments(provider.Meter("profile-persistence"))
			ctx := collectobserve.With(context.Background(), collectobserve.Metadata{
				TenantID: 78, AccountID: 615, SourceID: 351, ProfileID: test.profileID,
				SourceType: "bot", Mode: "history", ReceivedAt: test.receivedAt,
			})
			got := instruments.recordProfilePersistence(ctx, test.profileID, test.created, test.durable, test.commitErr, finishedAt)
			if got != test.wantResult {
				t.Fatalf("persistence result=%s want=%s", got, test.wantResult)
			}
			var data metricdata.ResourceMetrics
			if err := reader.Collect(context.Background(), &data); err != nil {
				t.Fatal(err)
			}
			var persisted, missing int64
			var lifecycleCount uint64
			var lifecycleSeconds float64
			for _, scope := range data.ScopeMetrics {
				for _, measurement := range scope.Metrics {
					switch measurement.Name {
					case "xiaohuiji.collect.profiles.persisted":
						sum := measurement.Data.(metricdata.Sum[int64])
						for _, point := range sum.DataPoints {
							persisted += point.Value
							action, _ := point.Attributes.Value("action")
							if action.AsString() != test.wantAction || point.Attributes.Len() != 3 {
								t.Fatalf("wrong action or unbounded persistence dimensions: %v", point.Attributes)
							}
						}
					case "xiaohuiji.collect.lifecycle.missing":
						sum := measurement.Data.(metricdata.Sum[int64])
						for _, point := range sum.DataPoints {
							missing += point.Value
							reason, _ := point.Attributes.Value("reason")
							if reason.AsString() != test.wantMissing || point.Attributes.Len() != 4 {
								t.Fatalf("wrong missing reason or unbounded dimensions: %v", point.Attributes)
							}
						}
					case "xiaohuiji.collect.lifecycle.duration_seconds":
						histogram := measurement.Data.(metricdata.Histogram[float64])
						for _, point := range histogram.DataPoints {
							lifecycleCount += point.Count
							lifecycleSeconds += point.Sum
							if point.Attributes.Len() != 3 {
								t.Fatalf("unbounded lifecycle dimensions: %v", point.Attributes)
							}
						}
					}
				}
			}
			wantPersisted, wantMissing, wantLifecycle := int64(0), int64(0), uint64(0)
			if test.wantAction != "" {
				wantPersisted = 1
			}
			if test.wantMissing != "" {
				wantMissing = 1
			}
			if test.wantSeconds > 0 {
				wantLifecycle = 1
			}
			if persisted != wantPersisted || missing != wantMissing || lifecycleCount != wantLifecycle || lifecycleSeconds != test.wantSeconds {
				t.Fatalf("counts persisted=%d missing=%d lifecycle=%d seconds=%f", persisted, missing, lifecycleCount, lifecycleSeconds)
			}
		})
	}
}
