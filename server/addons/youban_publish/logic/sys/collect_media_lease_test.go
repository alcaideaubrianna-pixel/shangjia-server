package sys

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestCollectMediaLeaseFailureCancelsWork(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "ownership changed"},
		{name: "redis unavailable", err: errors.New("redis unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, stop := maintainCollectMediaLeases(context.Background(), 6106866, time.Millisecond, func(context.Context) (bool, error) {
				return false, test.err
			})
			defer stop()
			select {
			case <-ctx.Done():
				if !errors.Is(context.Cause(ctx), errCollectMediaLeaseLost) {
					t.Fatalf("wrong cancellation cause: %v", context.Cause(ctx))
				}
			case <-time.After(time.Second):
				t.Fatal("work continued after losing ownership")
			}
		})
	}
}

func TestCollectMediaLeaseCancellationStopsRenewal(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	ctx, stop := maintainCollectMediaLeases(parent, 6106866, time.Millisecond, func(ctx context.Context) (bool, error) {
		close(started)
		<-ctx.Done()
		return false, ctx.Err()
	})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("heartbeat not started")
	}
	cancel()
	stop() // Must join even when renewal is in progress.
	if !errors.Is(context.Cause(ctx), context.Canceled) {
		t.Fatalf("wrong cancellation cause: %v", context.Cause(ctx))
	}
}

func TestCollectMediaLeaseHealthyWorkRenewsUntilStopped(t *testing.T) {
	var calls atomic.Int32
	renewed := make(chan struct{})
	ctx, stop := maintainCollectMediaLeases(context.Background(), 6106866, time.Millisecond, func(context.Context) (bool, error) {
		if calls.Add(1) == 3 {
			close(renewed)
		}
		return true, nil
	})
	select {
	case <-renewed:
	case <-time.After(time.Second):
		t.Fatal("healthy work did not keep renewing")
	}
	if ctx.Err() != nil {
		t.Fatalf("healthy task canceled: %v", ctx.Err())
	}
	stop()
	before := calls.Load()
	stop() // Cleanup is idempotent.
	if calls.Load() != before {
		t.Fatal("renewal continued after cleanup")
	}
	if collectMediaLeaseTTL > 2*time.Minute || collectMediaLeaseRefreshInterval*3 >= collectMediaLeaseTTL {
		t.Fatal("lease must recover promptly with renewal headroom")
	}
}
