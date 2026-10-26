package sys

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestContentRegionDirectoryCacheServesStaleWhileRefreshing(t *testing.T) {
	stale := &contentRegionDirectory{byCode: map[string]string{"110000": "旧值"}}
	fresh := &contentRegionDirectory{byCode: map[string]string{"110000": "北京市"}}
	cache := &contentRegionDirectoryCacheState{
		value:     stale,
		expiresAt: time.Now().Add(-time.Minute),
	}
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	loader := func(ctx context.Context) (*contentRegionDirectory, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return fresh, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	begin := time.Now()
	if got := cache.load(context.Background(), loader); got != stale {
		t.Fatalf("expired cache returned %p, want stale %p", got, stale)
	}
	if elapsed := time.Since(begin); elapsed > 100*time.Millisecond {
		t.Fatalf("expired cache blocked request for %s", elapsed)
	}
	<-started
	if got := cache.load(context.Background(), loader); got != stale {
		t.Fatalf("concurrent load returned %p, want stale %p", got, stale)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("loader calls = %d, want 1", got)
	}

	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		cache.RLock()
		got := cache.value
		refreshing := cache.refreshing
		cache.RUnlock()
		if got == fresh && !refreshing {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("background refresh did not publish fresh directory")
}

func TestContentRegionDirectoryCacheKeepsStaleOnRefreshFailure(t *testing.T) {
	stale := &contentRegionDirectory{byCode: map[string]string{"110000": "北京市"}}
	cache := &contentRegionDirectoryCacheState{
		value:     stale,
		expiresAt: time.Now().Add(-time.Minute),
	}
	done := make(chan struct{})
	loader := func(context.Context) (*contentRegionDirectory, error) {
		defer close(done)
		return nil, errors.New("database unavailable")
	}

	if got := cache.load(context.Background(), loader); got != stale {
		t.Fatalf("expired cache returned %p, want stale %p", got, stale)
	}
	<-done
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		cache.RLock()
		got := cache.value
		refreshing := cache.refreshing
		cache.RUnlock()
		if got == stale && !refreshing {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("failed refresh did not retain stale directory")
}

func TestContentRegionDirectoryCacheInitialLoadIsSingleFlight(t *testing.T) {
	cache := &contentRegionDirectoryCacheState{}
	started := make(chan struct{})
	release := make(chan struct{})
	result := make(chan *contentRegionDirectory, 1)
	var calls atomic.Int32
	loader := func(ctx context.Context) (*contentRegionDirectory, error) {
		calls.Add(1)
		close(started)
		select {
		case <-release:
			return &contentRegionDirectory{byCode: map[string]string{"110000": "北京市"}}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	go func() {
		result <- cache.load(context.Background(), loader)
	}()
	<-started
	if got := cache.load(context.Background(), loader); len(got.byCode) != 0 {
		t.Fatalf("concurrent initial load returned unexpected data: %+v", got.byCode)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("loader calls = %d, want 1", got)
	}
	close(release)
	if got := <-result; got.byCode["110000"] != "北京市" {
		t.Fatalf("initial load result = %+v", got.byCode)
	}
}
