package sys

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	hglock "hotgo/internal/library/hgrds/lock"
)

// TestLocalChannelCompletionWaitsForDispatch 验证真实 Redis 上短暂调度竞争不会丢失完成唤醒，普通事件仍快速返回。
func TestLocalChannelCompletionWaitsForDispatch(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_RUNTIME_CLEANUP") != "1" {
		t.Skip("requires disposable local Redis 16382")
	}
	previous := g.Cfg().GetAdapter()
	defer g.Cfg().SetAdapter(previous)
	adapter, err := gcfg.NewAdapterContent(`{"redis":{"default":{"address":"127.0.0.1:16382","db":15}}}`)
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	job := telegramJobRecord{TenantId: 999, ChannelId: 999999, TargetChatId: "-100999999999"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	held := hglock.NewConfig(time.Second, 10*time.Millisecond).WithoutWatchDog().Mutex(telegramChannelDispatchKey(job))
	if err := held.TryLock(ctx); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { time.Sleep(150 * time.Millisecond); done <- held.Unlock(ctx) }()
	started := time.Now()
	wakeCtx := context.WithValue(ctx, telegramChannelWakeDispatchKey{}, true)
	reserved, err := NewSysPublish().withTelegramChannelDispatchLease(wakeCtx, job, func(context.Context) (bool, error) { return true, nil })
	if releaseErr := <-done; releaseErr != nil {
		t.Fatal(releaseErr)
	}
	if err != nil || !reserved || time.Since(started) < 100*time.Millisecond {
		t.Fatalf("completion wake lost: reserved=%v duration=%s err=%v", reserved, time.Since(started), err)
	}
	held = hglock.NewConfig(time.Second, 10*time.Millisecond).WithoutWatchDog().Mutex(telegramChannelDispatchKey(job))
	if err := held.TryLock(ctx); err != nil {
		t.Fatal(err)
	}
	defer held.Unlock(context.Background())
	started = time.Now()
	reserved, err = NewSysPublish().withTelegramChannelDispatchLease(ctx, job, func(context.Context) (bool, error) { t.Fatal("ordinary event acquired busy lease"); return true, nil })
	if err == nil || reserved || time.Since(started) > 500*time.Millisecond {
		t.Fatalf("ordinary event must fail fast: reserved=%v elapsed=%s err=%v", reserved, time.Since(started), err)
	}
	bounded, boundedCancel := context.WithTimeout(context.WithValue(ctx, telegramChannelWakeDispatchKey{}, true), 80*time.Millisecond)
	defer boundedCancel()
	reserved, err = NewSysPublish().withTelegramChannelDispatchLease(bounded, job, func(context.Context) (bool, error) {
		t.Fatal("expired completion acquired busy lease")
		return true, nil
	})
	if err == nil || reserved {
		t.Fatalf("completion wait ignored deadline: reserved=%v err=%v", reserved, err)
	}
}
