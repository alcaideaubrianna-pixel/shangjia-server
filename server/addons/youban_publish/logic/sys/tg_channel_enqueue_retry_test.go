package sys

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestTelegramChannelEnqueueDurableTimeoutReturnsForOutbox(t *testing.T) {
	for _, err := range []error{context.DeadlineExceeded, fmt.Errorf("wrapped: %w", context.DeadlineExceeded), &net.DNSError{IsTimeout: true}, errors.New("redis: connection pool timeout"), errors.New("i/o timeout")} {
		n, got := runTelegramChannelEnqueueRetries(context.Background(), true, func() error { return err })
		if n != 1 || !errors.Is(got, err) {
			t.Fatalf("timeout retried: n=%d err=%v", n, got)
		}
	}
}

func TestTelegramChannelEnqueueCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	n, err := runTelegramChannelEnqueueRetries(ctx, true, func() error { t.Fatal("called with canceled ctx"); return nil })
	if n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatal(n, err)
	}
	n, err = runTelegramChannelEnqueueRetries(context.Background(), false, func() error { return fmt.Errorf("wrapped: %w", context.Canceled) })
	if n != 1 || !errors.Is(err, context.Canceled) {
		t.Fatal(n, err)
	}
}

func TestTelegramChannelEnqueueTransientBoundAndReceipt(t *testing.T) {
	n, err := runTelegramChannelEnqueueRetries(context.Background(), true, func() error { return errors.New("TRYAGAIN transient") })
	if n != 3 || err == nil {
		t.Fatal(n, err)
	}
	calls := 0
	n, err = runTelegramChannelEnqueueRetries(context.Background(), true, func() error {
		calls++
		if calls == 1 {
			return errors.New("LOADING startup")
		}
		return nil
	})
	if n != 2 || err != nil {
		t.Fatal(n, err)
	}
	for _, msg := range []string{"OOM command not allowed", "WRONGTYPE bad key", "NOAUTH required", "connection reset by peer"} {
		n, err = runTelegramChannelEnqueueRetries(context.Background(), true, func() error { return errors.New(msg) })
		if n != 1 || err == nil {
			t.Fatalf("permanent/ambiguous error retried: %d %v", n, err)
		}
	}
}

func TestTelegramChannelEnqueueDirectCallerKeepsBoundedRetries(t *testing.T) {
	n, err := runTelegramChannelEnqueueRetries(context.Background(), false, func() error { return context.DeadlineExceeded })
	if n != 3 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(n, err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		for i := 0; i < 100; i++ {
			d := telegramChannelEnqueueRetryDelay(attempt)
			base := time.Duration(attempt) * 50 * time.Millisecond
			if d < base || d > 2*base {
				t.Fatalf("unbounded jitter: %v", d)
			}
		}
	}
}

func TestTelegramChannelEnqueueCancelDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n, err := runTelegramChannelEnqueueRetries(ctx, true, func() error { cancel(); return errors.New("TRYAGAIN transient") })
	if n != 1 || !errors.Is(err, context.Canceled) {
		t.Fatal(n, err)
	}
}
