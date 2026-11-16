package sys

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestCollectPipelineMetricAttributesBounded(t *testing.T) {
	attrs := collectPipelineMetricAttributes("tenant-123", "arbitrary-error-message")
	if len(attrs) != 2 || attrs[0].Key != "stage" || attrs[0].Value.AsString() != "unknown" ||
		attrs[1].Key != "result" || attrs[1].Value.AsString() != "unknown" {
		t.Fatalf("unbounded attributes: %v", attrs)
	}
}

func TestCollectPipelineSpanBoundedAndNoRawError(t *testing.T) {
	ctx := context.Background()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })
	tracer := provider.Tracer("collect-pipeline-test")
	const raw = "SELECT private_text FROM secret; https://example.invalid/file?signature=private-token 原文"
	for _, stage := range []collectPipelineStage{
		collectPipelineSubmit, collectPipelineProcess, collectPipelineDispatch,
		collectPipelineAdmission, collectPipelineQueue, collectPipelineStage(raw),
	} {
		startedAt := time.Now()
		recordCollectPipelineSpan(ctx, tracer, stage, collectPipelineResult(raw),
			startedAt, startedAt.Add(time.Millisecond), errors.New(raw),
			attribute.Int64("job.id", 123),
			attribute.String("stage", raw), attribute.String("result", raw),
			attribute.String("error.type", raw))
	}
	spans := recorder.Ended()
	if len(spans) != 6 {
		t.Fatalf("got %d spans, want 6", len(spans))
	}
	for _, span := range spans {
		attrs := make(map[attribute.Key]string)
		for _, attr := range span.Attributes() {
			attrs[attr.Key] = attr.Value.Emit()
			if strings.Contains(attr.Value.Emit(), raw) {
				t.Fatal("span contains raw sensitive text")
			}
		}
		if span.Name() != "collect."+attrs["stage"] || attrs["result"] != "unknown" ||
			attrs["error.type"] != "other" || len(attrs) != 4 {
			t.Fatalf("unbounded span dimensions: name=%q attrs=%v", span.Name(), attrs)
		}
		if len(span.Events()) != 0 {
			t.Fatal("raw exception events must not be recorded")
		}
		if span.Status().Code != codes.Error || span.Status().Description != "collection stage failed" {
			t.Fatalf("unexpected status: %+v", span.Status())
		}
	}
	if spans[len(spans)-1].Name() != "collect.unknown" {
		t.Fatal("typed cast generated an unbounded span name")
	}
}

func TestCollectPipelineErrorTypeBounded(t *testing.T) {
	for _, test := range []struct {
		err  error
		want string
	}{
		{nil, "none"},
		{context.Canceled, "canceled"},
		{context.DeadlineExceeded, "deadline_exceeded"},
		{newCollectProcessRetryError(time.Second, "private text"), "retryable"},
		{asynq.ErrDuplicateTask, "duplicate"},
		{asynq.ErrTaskIDConflict, "conflict"},
		{errors.New("private SQL or signed URL"), "other"},
	} {
		if got := collectPipelineErrorType(test.err); got != test.want {
			t.Fatalf("got %s, want %s", got, test.want)
		}
	}
}

func TestCollectPipelineMetricsDurationAndCardinality(t *testing.T) {
	ctx := context.Background()
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })
	meter := provider.Meter("collect-pipeline-test")
	recordCollectPipelineMetrics(ctx, meter, collectPipelineSubmit, collectPipelineQueued, 250*time.Millisecond)
	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	foundCount, foundDuration := false, false
	for _, scope := range data.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			switch value := measurement.Data.(type) {
			case metricdata.Sum[int64]:
				if measurement.Name != "xiaohuiji.collect.pipeline.operations" || len(value.DataPoints) != 1 {
					t.Fatalf("unexpected counter: %+v", measurement)
				}
				point := value.DataPoints[0]
				if point.Value != 1 || point.Attributes.Len() != 2 {
					t.Fatalf("unexpected count/labels: %+v", point)
				}
				foundCount = true
			case metricdata.Histogram[float64]:
				if measurement.Name != "xiaohuiji.collect.pipeline.duration_seconds" || measurement.Unit != "s" || len(value.DataPoints) != 1 {
					t.Fatalf("unexpected histogram: %+v", measurement)
				}
				point := value.DataPoints[0]
				if point.Count != 1 || point.Sum != 0.25 || point.Attributes.Len() != 2 {
					t.Fatalf("duration must be seconds with bounded labels: %+v", point)
				}
				foundDuration = true
			}
		}
	}
	if !foundCount || !foundDuration {
		t.Fatalf("missing instruments: count=%t duration=%t", foundCount, foundDuration)
	}
}

func TestCollectPipelineProcessResult(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    string
		processed bool
		err       error
		want      collectPipelineResult
	}{
		{"completed", "", true, nil, collectPipelineCompleted},
		{"media_pending_is_not_completed", "", false, nil, collectPipelinePending},
		{"ignored", sysin.CollectEventStatusIgnored, true, nil, collectPipelineIgnored},
		{"failed", "", true, errors.New("failed"), collectPipelineFailed},
		{"retryable", "", false, newCollectProcessRetryError(time.Second, "retry"), collectPipelineRetryable},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := collectPipelineProcessResult(test.status, test.processed, test.err); got != test.want {
				t.Fatalf("got %s, want %s", got, test.want)
			}
		})
	}
}

func TestCollectDispatchHandoffPreservesCallAndError(t *testing.T) {
	if got := collectPipelineDispatchResult(nil); got != collectPipelineUnconfirmed {
		t.Fatalf("nil may mean pending media or skipped; got %s", got)
	}
	wantErr := errors.New("queue submit failed")
	if got := collectPipelineDispatchResult(wantErr); got != collectPipelineFailed {
		t.Fatalf("queue error classified as %s", got)
	}
	calls := 0
	got := observeCollectDispatchHandoff(context.Background(), gdb.Record{}, 1, 2, func() error {
		calls++
		return wantErr
	})
	if got != wantErr || calls != 1 {
		t.Fatalf("changed business behavior: err=%v calls=%d", got, calls)
	}
}

func TestCollectPublishQueueRequiresReceipt(t *testing.T) {
	for _, test := range []struct {
		name    string
		receipt bool
		err     error
		want    collectPipelineResult
	}{
		{"acknowledged", true, nil, collectPipelineQueued},
		{"no_receipt", false, nil, collectPipelineUnconfirmed},
		{"redis_error", false, errors.New("redis unavailable"), collectPipelineFailed},
		{"error_beats_receipt", true, errors.New("timeout"), collectPipelineFailed},
		{"duplicate_is_not_new_admission", false, asynq.ErrDuplicateTask, collectPipelineDuplicate},
		{"task_id_conflict_is_not_success", false, asynq.ErrTaskIDConflict, collectPipelineConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := collectPublishQueueResult(test.receipt, test.err); got != test.want {
				t.Fatalf("got %s, want %s", got, test.want)
			}
		})
	}
}

func TestCollectPublishJobMetricStages(t *testing.T) {
	ctx := context.Background()
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))
	t.Cleanup(func() { _ = provider.Shutdown(ctx) })
	meter := provider.Meter("publish-job-test")
	// Two durable jobs, but only one channel head is physically enqueued.
	// The other job must not acquire a fabricated queue success.
	for i := 0; i < 2; i++ {
		recordCollectPipelineMetrics(ctx, meter, collectPipelineAdmission, collectPipelinePersisted, -1)
	}
	recordCollectPipelineMetrics(ctx, meter, collectPipelineQueue, collectPipelineQueued, time.Millisecond)
	var data metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &data); err != nil {
		t.Fatal(err)
	}
	var admitted, queued int64
	for _, scope := range data.ScopeMetrics {
		for _, measurement := range scope.Metrics {
			switch value := measurement.Data.(type) {
			case metricdata.Sum[int64]:
				for _, point := range value.DataPoints {
					if point.Attributes.Len() != 2 {
						t.Fatalf("high-cardinality metric attributes: %v", point.Attributes)
					}
					stage, _ := point.Attributes.Value("stage")
					switch stage.AsString() {
					case string(collectPipelineAdmission):
						admitted += point.Value
					case string(collectPipelineQueue):
						queued += point.Value
					}
				}
			case metricdata.Histogram[float64]:
				for _, point := range value.DataPoints {
					stage, _ := point.Attributes.Value("stage")
					if stage.AsString() == string(collectPipelineAdmission) {
						t.Fatal("batch admission duration must not be repeated per job")
					}
				}
			}
		}
	}
	if admitted != 2 || queued != 1 {
		t.Fatalf("job admission/queue counts: admitted=%d queued=%d", admitted, queued)
	}
}
