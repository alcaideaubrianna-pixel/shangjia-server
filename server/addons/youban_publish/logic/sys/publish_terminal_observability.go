package sys

import (
	"context"
	"errors"
	"hotgo/utility/dbtime"
	"sync"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// publishTerminalMetrics owns instruments only. The application owns the global
// provider, readers and exporters; tests inject an isolated provider.
type publishTerminalMetrics struct {
	jobs                      metric.Int64Counter
	jobDuration               metric.Float64Histogram
	projectionCommits         metric.Int64Counter
	projectionAttempts        metric.Int64Counter
	projectionAttemptDuration metric.Float64Histogram
	projectionDuration        metric.Float64Histogram
}

func newPublishTerminalMetrics(provider metric.MeterProvider) (*publishTerminalMetrics, error) {
	if provider == nil {
		provider = otel.GetMeterProvider()
	}
	meter := provider.Meter("hotgo/youban_publish/terminal")
	m := new(publishTerminalMetrics)
	var err error
	m.jobs, err = meter.Int64Counter("xiaohuiji.publish.job.transitions",
		metric.WithDescription("Committed changed terminal job transitions"), metric.WithUnit("{job}"))
	if err != nil {
		return nil, err
	}
	m.jobDuration, err = meter.Float64Histogram("xiaohuiji.publish.job.duration",
		metric.WithDescription("Time from job creation to committed terminal transition"), metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200))
	if err != nil {
		return nil, err
	}
	m.projectionCommits, err = meter.Int64Counter("xiaohuiji.publish.projection.commits",
		metric.WithDescription("Top-level actual returned terminal confirmation commits with a changed transition; success only"), metric.WithUnit("{commit}"))
	if err != nil {
		return nil, err
	}
	m.projectionDuration, err = meter.Float64Histogram("xiaohuiji.publish.projection.duration",
		metric.WithDescription("Duration of committed changed local publish projections"), metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200))
	if err != nil {
		return nil, err
	}
	m.projectionAttempts, err = meter.Int64Counter("xiaohuiji.publish.projection.attempts",
		metric.WithDescription("Top-level terminal confirmation transaction return outcomes; success means an actual returned changed commit"), metric.WithUnit("{attempt}"))
	if err != nil {
		return nil, err
	}
	m.projectionAttemptDuration, err = meter.Float64Histogram("xiaohuiji.publish.projection.attempt.duration",
		metric.WithDescription("Terminal confirmation transaction attempt duration"), metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200))
	if err != nil {
		return nil, err
	}
	return m, nil
}

var (
	publishTerminalMetricsOnce   sync.Once
	publishTerminalMetricsGlobal *publishTerminalMetrics
)

func defaultPublishTerminalMetrics() *publishTerminalMetrics {
	publishTerminalMetricsOnce.Do(func() {
		var err error
		publishTerminalMetricsGlobal, err = newPublishTerminalMetrics(nil)
		if err != nil {
			otel.Handle(err)
		}
	})
	return publishTerminalMetricsGlobal
}

// terminalCommitted must be called after the owning transaction returns. A
// changed flag from inside a transaction is insufficient if its commit fails.
// No IDs, queue names, error messages or other unbounded values become labels.
// Reterminalization after an explicit retry is another transition, not a unique
// lifetime job count. A process crash after commit can still lose the metric.
func (m *publishTerminalMetrics) terminalCommitted(ctx context.Context, status JobStatus, changed bool, commitErr error, createdAt time.Time, committedAt time.Time) {
	if m == nil || !changed || commitErr != nil {
		return
	}
	var result string
	switch status {
	case telegramJobStatusSent:
		result = "sent"
	case telegramJobStatusFailed:
		result = "failed"
	case telegramJobStatusSuperseded:
		result = "superseded"
	default:
		// Deferred, unknown and retry states are not terminal success.
		return
	}
	attrs := metric.WithAttributes(attribute.String("status", result))
	m.jobs.Add(ctx, 1, attrs)
	if !createdAt.IsZero() && !committedAt.Before(createdAt) {
		m.jobDuration.Record(ctx, committedAt.Sub(createdAt).Seconds(), attrs)
	}
}

// projectionCommitted records only a real changed projection after commit.
// Enqueue/defer/fallback acceptance must never be passed as a changed commit.
func (m *publishTerminalMetrics) projectionCommitted(ctx context.Context, changed bool, commitErr error, duration time.Duration) {
	if m == nil || !changed || commitErr != nil {
		return
	}
	attrs := metric.WithAttributes(attribute.String("kind", "terminal_confirmation"))
	m.projectionCommits.Add(ctx, 1, attrs)
	if duration >= 0 {
		m.projectionDuration.Record(ctx, duration.Seconds(), attrs)
	}
}

func observePublishTerminalCommit(ctx context.Context, job telegramJobRecord, status JobStatus, changed bool, commitErr error) {
	if !publishTerminalTopLevelContext(ctx) {
		return
	}
	var createdAt time.Time
	if job.CreatedAt != nil {
		createdAt = dbtime.FromTimestamp(job.CreatedAt).Time
	}
	defaultPublishTerminalMetrics().terminalCommitted(ctx, status, changed, commitErr, createdAt, time.Now())
}

// Both the caller and the DB handle can carry an outer transaction. Skipping
// nested calls is conservative: GoFrame does not provide an outer-commit hook
// here, and an inner savepoint release is not a durable commit. No SQL is read.
func publishTerminalTopLevelContext(ctx context.Context) bool {
	database := g.DB()
	return gdb.TXFromCtx(ctx, database.GetGroup()) == nil &&
		gdb.TXFromCtx(database.GetCtx(), database.GetGroup()) == nil
}

func (m *publishTerminalMetrics) projectionAttempt(ctx context.Context, changed bool, commitErr error, duration time.Duration) {
	if m == nil {
		return
	}
	outcome, errorClass := "success", "none"
	if errors.Is(commitErr, context.Canceled) {
		outcome, errorClass = "canceled", "canceled"
	} else if errors.Is(commitErr, context.DeadlineExceeded) {
		outcome, errorClass = "timeout", "timeout"
	} else if commitErr != nil {
		outcome, errorClass = "failed", "commit_error"
	} else if !changed {
		outcome = "noop"
	}
	attrs := metric.WithAttributes(
		attribute.String("kind", "terminal_confirmation"),
		attribute.String("outcome", outcome),
		attribute.String("error_class", errorClass),
	)
	m.projectionAttempts.Add(ctx, 1, attrs)
	if duration >= 0 {
		m.projectionAttemptDuration.Record(ctx, duration.Seconds(), attrs)
	}
	m.projectionCommitted(ctx, changed, commitErr, duration)
}

// observePublishTerminalProjection is for confirmTelegramJobAndCyclePlan and
// confirmTelegramJobAndRecovery only, after they return their committed changed
// flag and error. It measures the terminal confirmation transaction, not later
// asynchronous profile/index projection. Do not pass a logical completion flag
// from completeProfileTelegramOperation, or an enqueue/fallback result.
func observePublishTerminalProjection(ctx context.Context, changed bool, commitErr error, duration time.Duration) {
	if !publishTerminalTopLevelContext(ctx) {
		return
	}
	defaultPublishTerminalMetrics().projectionAttempt(ctx, changed, commitErr, duration)
}
