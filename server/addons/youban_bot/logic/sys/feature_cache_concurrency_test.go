package sys

import (
	"context"
	"testing"
	"time"
)

func TestFeatureConfigStalledReadDoesNotHoldLock(t *testing.T) {
	s := &sSysBot{featureDefaultsAt: time.Now()}
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := s.loadFeatureConfigs(context.Background(), func(ctx context.Context) ([]*botFeatureRow, error) {
			close(entered)
			<-release
			return []*botFeatureRow{{FeatureKey: "image_scan", Status: 1}}, nil
		})
		done <- err
	}()
	<-entered
	defer close(release)
	invalidated := make(chan struct{})
	go func() {
		s.clearFeatureCache()
		_ = s.featureDefaultsSynced()
		close(invalidated)
	}()
	select {
	case <-invalidated:
	case <-time.After(time.Second):
		t.Fatal("stalled DB read blocks feature cache lock")
	}
	// A second request can load configuration while the first is stalled.
	configs, err := s.loadFeatureConfigs(context.Background(), func(ctx context.Context) ([]*botFeatureRow, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > featureQueryTimeout {
			t.Error("configuration read has no bounded deadline")
		}
		return []*botFeatureRow{{FeatureKey: "image_scan", Status: 2}}, nil
	})
	if err != nil || configs["image_scan"].Status != 2 {
		t.Fatalf("independent read failed: configs=%v err=%v", configs, err)
	}
	// Release explicitly to inspect the stale result before the deferred close.
	release <- struct{}{}
	if err := <-done; err == nil {
		t.Fatal("in-flight result survived cache invalidation")
	}
	if s.features["image_scan"].Status != 2 {
		t.Fatal("stale read overwrote updated configuration")
	}
}

func TestFeatureConfigCancelledReadDoesNotPopulateCache(t *testing.T) {
	s := &sSysBot{featureDefaultsAt: time.Now()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := s.loadFeatureConfigs(ctx, func(ctx context.Context) ([]*botFeatureRow, error) {
		cancel()
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err == nil || s.features != nil {
		t.Fatalf("cancelled read cached: err=%v cache=%v", err, s.features)
	}
}
