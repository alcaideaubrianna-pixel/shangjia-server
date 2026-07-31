package sys

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"hotgo/internal/library/hgrds/lock"
)

func TestTelegramAccountClientLeaseKey(t *testing.T) {
	if got, want := telegramAccountClientLeaseKey(12), "youban_publish:tg:account-client:12"; got != want {
		t.Fatalf("lease key = %q, want %q", got, want)
	}
	if telegramAccountClientLeaseKey(12) == telegramAccountClientLeaseKey(13) {
		t.Fatal("different Telegram accounts must not share a client lease")
	}
}

func TestWaitTelegramAccountClientLeaseRetriesUntilAvailable(t *testing.T) {
	var attempts atomic.Int32
	var waits atomic.Int32
	want := &lock.Lock{}
	lease, err := waitTelegramAccountClientLease(context.Background(), time.Millisecond, func() (*lock.Lock, error) {
		if attempts.Add(1) < 3 {
			return nil, lock.ErrLockFailed
		}
		return want, nil
	}, func(error) {
		waits.Add(1)
	})
	if err != nil {
		t.Fatalf("wait lease failed: %v", err)
	}
	if lease != want {
		t.Fatal("wait lease returned unexpected lock")
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}
	if got := waits.Load(); got != 1 {
		t.Fatalf("wait callbacks = %d, want 1", got)
	}
}

func TestWaitTelegramAccountClientLeaseStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var attempts atomic.Int32
	_, err := waitTelegramAccountClientLease(ctx, time.Millisecond, func() (*lock.Lock, error) {
		attempts.Add(1)
		return nil, lock.ErrLockFailed
	}, nil)
	if err != context.Canceled {
		t.Fatalf("error = %v, want context canceled", err)
	}
	if got := attempts.Load(); got != 0 {
		t.Fatalf("attempts = %d, want 0 after context cancellation", got)
	}
}

func TestCollectDisplayEventPassedEarlyCheck(t *testing.T) {
	for _, status := range []string{"prechecked", "media_pending", "media_ready", "processed", "dispatched"} {
		if !collectDisplayEventPassedEarlyCheck(status) {
			t.Fatalf("status %q must allow verify media", status)
		}
	}
	for _, status := range []string{"pending", "group_collecting", "ignored", "failed", ""} {
		if collectDisplayEventPassedEarlyCheck(status) {
			t.Fatalf("status %q must block verify media", status)
		}
	}
}
