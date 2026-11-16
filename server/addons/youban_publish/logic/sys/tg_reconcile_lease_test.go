package sys

import (
	"context"
	"errors"
	"testing"
	"time"

	"hotgo/utility/ownedlease"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gredis"
)

func TestTelegramReconcileLeaseSerializesOnlySameJob(t *testing.T) {
	redis := miniredis.RunT(t)
	client, err := gredis.New(&gredis.Config{Address: redis.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close(context.Background()) })
	s := &sSysPublish{}
	newLease := func(key string) *ownedlease.Lease {
		return ownedlease.New(key, "test-reconcile", time.Minute, client)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.runTelegramReconcileLease(context.Background(), newLease("job:1"), func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first reconciliation did not start")
	}
	called := false
	if err := s.runTelegramReconcileLease(context.Background(), newLease("job:1"), func(context.Context) error {
		called = true
		return nil
	}); err != nil || called {
		t.Fatalf("same job entered concurrently: called=%v err=%v", called, err)
	}
	if err := s.runTelegramReconcileLease(context.Background(), newLease("job:2"), func(context.Context) error {
		called = true
		return nil
	}); err != nil || !called {
		t.Fatalf("different job blocked: called=%v err=%v", called, err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	called = false
	if err := s.runTelegramReconcileLease(context.Background(), newLease("job:1"), func(context.Context) error {
		called = true
		return nil
	}); err != nil || !called {
		t.Fatalf("completed lease was not released: called=%v err=%v", called, err)
	}
}

func TestTelegramReconcileLeaseReleasesAfterCancellation(t *testing.T) {
	redis := miniredis.RunT(t)
	client, err := gredis.New(&gredis.Config{Address: redis.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close(context.Background()) })
	ctx, cancel := context.WithCancel(context.Background())
	s := &sSysPublish{}
	cause := errors.New("reconcile failed")
	err = s.runTelegramReconcileLease(ctx, ownedlease.New("job:cancel", "first", time.Minute, client), func(context.Context) error {
		cancel()
		return cause
	})
	if !errors.Is(err, cause) {
		t.Fatalf("lost callback error: %v", err)
	}
	if ok, err := ownedlease.New("job:cancel", "second", time.Minute, client).TryAcquire(context.Background()); err != nil || !ok {
		t.Fatalf("canceled owner retained lease: claimed=%v err=%v", ok, err)
	}
}
