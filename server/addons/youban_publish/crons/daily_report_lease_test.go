package crons

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
)

func TestDailyReportLeaseLocalRedis(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_REPORT_REDIS") != "1" {
		t.Skip("requires disposable local Redis 16382 DB15")
	}
	previous := g.Cfg().GetAdapter()
	defer g.Cfg().SetAdapter(previous)
	adapter, err := gcfg.NewAdapterContent(`{"redis":{"default":{"address":"127.0.0.1:16382","db":15}}}`)
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	var calls atomic.Int64
	want := errors.New("generation failed")
	go func() {
		done <- withDailyReportLease(context.Background(), func() error { calls.Add(1); close(entered); <-release; return want })
	}()
	<-entered
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := withDailyReportLease(context.Background(), func() error { calls.Add(1); return nil }); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	close(release)
	if err := <-done; !errors.Is(err, want) {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("duplicate coordinator calls: %d", calls.Load())
	}
	if err := withDailyReportLease(context.Background(), func() error { calls.Add(1); return nil }); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatal("failed run did not release lease")
	}
}
