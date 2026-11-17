package sys

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// These sources deliberately use private providers and ManualReader, never
// SetMeterProvider, exporters, background readers, databases or consumers.
func newPublishTerminalMetricsTest(t *testing.T) (*publishTerminalMetrics, *sdkmetric.ManualReader) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown metric provider: %v", err)
		}
	})
	m, err := newPublishTerminalMetrics(provider)
	if err != nil {
		t.Fatalf("create terminal metrics: %v", err)
	}
	return m, reader
}

func collectPublishTerminalMetricsTest(t *testing.T, reader *sdkmetric.ManualReader) map[string]metricdata.Metrics {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatalf("collect terminal metrics: %v", err)
	}
	metrics := make(map[string]metricdata.Metrics)
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			metrics[m.Name] = m
		}
	}
	return metrics
}

func TestPublishTerminalMetricsCommittedTransitions(t *testing.T) {
	m, reader := newPublishTerminalMetricsTest(t)
	ctx := context.Background()
	started := time.Unix(100, 0)
	for _, status := range []JobStatus{telegramJobStatusSent, telegramJobStatusFailed, telegramJobStatusSuperseded} {
		m.terminalCommitted(ctx, status, true, nil, started, started.Add(2500*time.Millisecond))
	}
	metrics := collectPublishTerminalMetricsTest(t, reader)
	jobs := metrics["xiaohuiji.publish.job.transitions"]
	sum, ok := jobs.Data.(metricdata.Sum[int64])
	if !ok || !sum.IsMonotonic || len(sum.DataPoints) != 3 {
		t.Fatalf("expected monotonic counter with three terminal series: %#v", jobs)
	}
	for _, point := range sum.DataPoints {
		if point.Value != 1 || point.Attributes.Len() != 1 {
			t.Fatalf("unexpected terminal value or attribute cardinality: %#v", point)
		}
		status, exists := point.Attributes.Value("status")
		if !exists || (status.AsString() != "sent" && status.AsString() != "failed" && status.AsString() != "superseded") {
			t.Fatalf("unexpected terminal status: %#v", point)
		}
	}
	duration := metrics["xiaohuiji.publish.job.duration"]
	histogram, ok := duration.Data.(metricdata.Histogram[float64])
	if !ok || duration.Unit != "s" || len(histogram.DataPoints) != 3 {
		t.Fatalf("expected three duration series in seconds: %#v", duration)
	}
	for _, point := range histogram.DataPoints {
		if point.Count != 1 || point.Sum != 2.5 {
			t.Fatalf("unexpected terminal duration: %#v", point)
		}
	}
}

func TestPublishTerminalMetricsSkipUncommittedAndDeferred(t *testing.T) {
	m, reader := newPublishTerminalMetricsTest(t)
	ctx := context.Background()
	now := time.Unix(100, 0)
	commitErr := errors.New("commit failed with arbitrary sensitive detail")
	m.terminalCommitted(ctx, telegramJobStatusSent, false, nil, now, now)
	m.terminalCommitted(ctx, telegramJobStatusFailed, true, commitErr, now, now)
	for _, status := range []JobStatus{telegramJobStatusPending, telegramJobStatusSending, telegramJobStatusRetryWait, telegramJobStatusConfirming, JobStatusUnknown} {
		m.terminalCommitted(ctx, status, true, nil, now, now)
	}
	m.projectionCommitted(ctx, false, nil, time.Second)
	m.projectionCommitted(ctx, true, commitErr, time.Second)
	if metrics := collectPublishTerminalMetricsTest(t, reader); len(metrics) != 0 {
		t.Fatalf("uncommitted or deferred work emitted metrics: %#v", metrics)
	}
}

func TestPublishProjectionMetricsCommittedDuration(t *testing.T) {
	m, reader := newPublishTerminalMetricsTest(t)
	m.projectionCommitted(context.Background(), true, nil, 1250*time.Millisecond)
	metrics := collectPublishTerminalMetricsTest(t, reader)
	commits, ok := metrics["xiaohuiji.publish.projection.commits"].Data.(metricdata.Sum[int64])
	if !ok || !commits.IsMonotonic || len(commits.DataPoints) != 1 || commits.DataPoints[0].Value != 1 || commits.DataPoints[0].Attributes.Len() != 1 {
		t.Fatalf("unexpected projection counter: %#v", commits)
	}
	duration := metrics["xiaohuiji.publish.projection.duration"]
	histogram, ok := duration.Data.(metricdata.Histogram[float64])
	if !ok || duration.Unit != "s" || len(histogram.DataPoints) != 1 || histogram.DataPoints[0].Count != 1 || histogram.DataPoints[0].Sum != 1.25 {
		t.Fatalf("unexpected projection duration: %#v", duration)
	}
}

func TestPublishTerminalMetricsMissingOrFutureCreationTime(t *testing.T) {
	m, reader := newPublishTerminalMetricsTest(t)
	now := time.Unix(100, 0)
	m.terminalCommitted(context.Background(), telegramJobStatusSent, true, nil, time.Time{}, now)
	m.terminalCommitted(context.Background(), telegramJobStatusSent, true, nil, now.Add(time.Second), now)
	metrics := collectPublishTerminalMetricsTest(t, reader)
	jobs, ok := metrics["xiaohuiji.publish.job.transitions"].Data.(metricdata.Sum[int64])
	if !ok || len(jobs.DataPoints) != 1 || jobs.DataPoints[0].Value != 2 {
		t.Fatalf("missing timestamps should retain terminal counts: %#v", jobs)
	}
	if _, exists := metrics["xiaohuiji.publish.job.duration"]; exists {
		t.Fatal("invalid creation timestamps emitted duration")
	}
}

func TestPublishTerminalMetricsProviderIsolation(t *testing.T) {
	global := otel.GetMeterProvider()
	m, reader := newPublishTerminalMetricsTest(t)
	_, otherReader := newPublishTerminalMetricsTest(t)
	m.projectionCommitted(context.Background(), true, nil, 0)
	if metrics := collectPublishTerminalMetricsTest(t, reader); len(metrics) != 2 {
		t.Fatalf("expected private provider metrics: %#v", metrics)
	}
	if metrics := collectPublishTerminalMetricsTest(t, otherReader); len(metrics) != 0 {
		t.Fatalf("metrics leaked into other provider: %#v", metrics)
	}
	if otel.GetMeterProvider() != global {
		t.Fatal("factory replaced global meter provider")
	}
}

func TestPublishTerminalProjectionAttempts(t *testing.T) {
	m, reader := newPublishTerminalMetricsTest(t)
	ctx := context.Background()
	m.projectionAttempt(ctx, true, nil, time.Second)
	m.projectionAttempt(ctx, false, nil, time.Second)
	m.projectionAttempt(ctx, true, errors.New("arbitrary database detail must not become a label"), time.Second)
	m.projectionAttempt(ctx, true, fmt.Errorf("wrapped commit: %w", context.DeadlineExceeded), time.Second)
	m.projectionAttempt(ctx, true, fmt.Errorf("wrapped commit: %w", context.Canceled), time.Second)
	metrics := collectPublishTerminalMetricsTest(t, reader)
	attempts, ok := metrics["xiaohuiji.publish.projection.attempts"].Data.(metricdata.Sum[int64])
	if !ok || !attempts.IsMonotonic || len(attempts.DataPoints) != 5 {
		t.Fatalf("unexpected projection attempts: %#v", attempts)
	}
	seen := make(map[string]bool)
	for _, point := range attempts.DataPoints {
		kind, _ := point.Attributes.Value("kind")
		outcome, _ := point.Attributes.Value("outcome")
		errorClass, _ := point.Attributes.Value("error_class")
		if point.Value != 1 || point.Attributes.Len() != 3 || kind.AsString() != "terminal_confirmation" {
			t.Fatalf("unexpected attempt dimensions: %#v", point)
		}
		seen[outcome.AsString()] = true
		wantClass := "none"
		switch outcome.AsString() {
		case "failed":
			wantClass = "commit_error"
		case "timeout", "canceled":
			wantClass = outcome.AsString()
		}
		if errorClass.AsString() != wantClass {
			t.Fatalf("unbounded or incorrect error class: %#v", point)
		}
	}
	if !seen["success"] || !seen["noop"] || !seen["failed"] || !seen["timeout"] || !seen["canceled"] {
		t.Fatalf("unexpected attempt outcomes: %v", seen)
	}
	commits, ok := metrics["xiaohuiji.publish.projection.commits"].Data.(metricdata.Sum[int64])
	if !ok || len(commits.DataPoints) != 1 || commits.DataPoints[0].Value != 1 {
		t.Fatalf("noop or failed attempts counted as committed: %#v", commits)
	}
	duration := metrics["xiaohuiji.publish.projection.attempt.duration"]
	histogram, ok := duration.Data.(metricdata.Histogram[float64])
	if !ok || duration.Unit != "s" || len(histogram.DataPoints) != 5 {
		t.Fatalf("unexpected attempt durations: %#v", duration)
	}
}

func TestPublishTerminalMetricsLongDurationBuckets(t *testing.T) {
	m, reader := newPublishTerminalMetricsTest(t)
	ctx := context.Background()
	started := time.Unix(100, 0)
	m.terminalCommitted(ctx, telegramJobStatusSent, true, nil, started, started.Add(30*time.Minute))
	m.projectionAttempt(ctx, true, nil, 30*time.Minute)
	metrics := collectPublishTerminalMetricsTest(t, reader)
	for _, name := range []string{"xiaohuiji.publish.job.duration", "xiaohuiji.publish.projection.duration", "xiaohuiji.publish.projection.attempt.duration"} {
		histogram, ok := metrics[name].Data.(metricdata.Histogram[float64])
		if !ok || len(histogram.DataPoints) != 1 {
			t.Fatalf("missing duration histogram %s", name)
		}
		point := histogram.DataPoints[0]
		if point.Count != 1 || point.Sum != 1800 || len(point.Bounds) == 0 || point.Bounds[len(point.Bounds)-1] != 7200 {
			t.Fatalf("incorrect long duration buckets for %s: %#v", name, point)
		}
		found := false
		for i, bound := range point.Bounds {
			if bound == 1800 {
				found = point.BucketCounts[i] == 1
			}
		}
		if !found || point.BucketCounts[len(point.BucketCounts)-1] != 0 {
			t.Fatalf("30 minute sample overflowed %s: %#v", name, point)
		}
	}
}
