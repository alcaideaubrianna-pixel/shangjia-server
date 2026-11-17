package global

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTelemetryShutdownHasIndependentUncancelledBudgets(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var traceCtx context.Context
	var traceDeadline time.Time
	calls := 0
	shutdownTelemetryProviders(ctx, func(ctx context.Context) error {
		calls++
		traceCtx = ctx
		var ok bool
		traceDeadline, ok = ctx.Deadline()
		if !ok || ctx.Err() != nil || time.Until(traceDeadline) < 4*time.Second {
			t.Fatalf("trace did not receive its own uncancelled budget: %v", ctx.Err())
		}
		return errors.New("simulated exporter shutdown failure")
	}, func(ctx context.Context) error {
		calls++
		deadline, ok := ctx.Deadline()
		if !ok || ctx.Err() != nil || time.Until(deadline) < 4*time.Second {
			t.Fatalf("metrics did not receive its own uncancelled budget: %v", ctx.Err())
		}
		if traceCtx.Err() != context.Canceled || deadline.Before(traceDeadline) {
			t.Fatal("metric shutdown reused the cancelled trace budget")
		}
		return nil
	})
	if calls != 2 {
		t.Fatalf("shutdown calls=%d want=2", calls)
	}
}
