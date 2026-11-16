package sys

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/database/gredis"
	"hotgo/utility/ownedlease"
)

func TestCollectSourceWorkerIdleRetiresAndNewWorkKeepsWorkerAlive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var probes atomic.Int32
	waitCollectSourceQueueWorker(ctx, time.Millisecond, 3*time.Millisecond,
		func(context.Context) (bool, error) { return true, nil },
		func(context.Context) (bool, error) { probes.Add(1); return false, nil })
	if ctx.Err() != nil || probes.Load() < 2 {
		t.Fatal("empty queue did not retire")
	}
	// A new consumer for the same source must stay alive when new work arrives.
	next, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		waitCollectSourceQueueWorker(next, time.Millisecond, time.Millisecond,
			func(context.Context) (bool, error) { return true, nil },
			func(context.Context) (bool, error) { probes.Add(1); return true, nil })
	}()
	time.Sleep(10 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("consumer retired with pending work")
	default:
	}
	stop()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("cancellation did not stop consumer")
	}
}

func TestCollectSourceWorkerLostLeaseCannotRenewSuccessor(t *testing.T) {
	r := miniredis.RunT(t)
	client, err := gredis.New(&gredis.Config{Address: r.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close(context.Background()) })
	ctx := context.Background()
	first := ownedlease.New("source:1", "first", time.Second, client)
	if ok, err := first.TryAcquire(ctx); err != nil || !ok {
		t.Fatalf("first acquire: %v %v", ok, err)
	}
	r.FastForward(2 * time.Second)
	second := ownedlease.New("source:1", "second", time.Second, client)
	if ok, err := second.TryAcquire(ctx); err != nil || !ok {
		t.Fatalf("second acquire: %v %v", ok, err)
	}
	var probes int
	waitCollectSourceQueueWorker(ctx, time.Millisecond, time.Second, first.Renew, func(context.Context) (bool, error) { probes++; return true, nil })
	if probes != 0 {
		t.Fatal("lost owner continued consuming")
	}
	_ = first.Unlock(ctx)
	if ok, err := second.Renew(ctx); err != nil || !ok {
		t.Fatalf("stale cleanup removed successor: %v %v", ok, err)
	}
}

func TestCollectSourceWorkerProbeErrorsDoNotRetireHealthyLease(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	waitCollectSourceQueueWorker(ctx, time.Millisecond, time.Millisecond, func(context.Context) (bool, error) { return true, nil }, func(context.Context) (bool, error) { return false, errors.New("unavailable") })
	if ctx.Err() == nil {
		t.Fatal("probe error was mistaken for idle queue")
	}
}
