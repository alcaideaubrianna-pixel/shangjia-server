package sys

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
)

// TestStreamCleanupSingleConnectionLocalPostgres 用一个真实PG连接验证删除回调不会被持锁事务占满连接池。
func TestStreamCleanupSingleConnectionLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_STREAM_MEDIA_PG") != "1" {
		t.Skip("requires disposable local PG and Redis16382")
	}
	t.Setenv("YOUBAN_PUBLISH_PUSH_ENGINE", pushEngineUnifiedStream)
	previousConfig := g.Cfg().GetAdapter()
	t.Cleanup(func() { g.Cfg().SetAdapter(previousConfig) })
	adapter, err := gcfg.NewAdapterContent(`{"redis":{"default":{"address":"127.0.0.1:16382","db":15}}}`)
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "5432", User: "postgres", Pass: "postgres", Name: "youban_hotgo", MaxOpenConnCount: 1, MaxIdleConnCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	job := telegramJobRecord{TenantId: 999, AccountId: 999, TargetChatId: "-100999999987"}
	// 发送槽位故意保持占用；删除必须使用自己的槽位且不占PG事务。
	sender := newTelegramStreamExecutionLease(job)
	if ok, err := sender.TryAcquire(ctx); err != nil || !ok {
		t.Fatal(ok, err)
	}
	defer sender.Unlock(context.Background())
	started := time.Now()
	err = NewSysPublish().withTelegramCleanupExecution(ctx, job, func(ctx context.Context) error {
		value, err := db.GetValue(ctx, "SELECT 1")
		if err != nil {
			return err
		}
		if value.Int() != 1 {
			t.Fatal(value.String())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("cleanup waited for pool or send lease", time.Since(started))
	}
}

// Both execution engines share the same Redis keys. Filling the automatic
// capacity must leave the reserved slot usable by a manual stream task.
func TestTelegramCleanupReservedSlotAcrossEngines(t *testing.T) {
	server := miniredis.RunT(t)
	previousConfig := g.Cfg().GetAdapter()
	adapter, err := gcfg.NewAdapterContent(fmt.Sprintf(`{"redis":{"default":{"address":%q}}}`, server.Addr()))
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	t.Cleanup(func() { g.Cfg().SetAdapter(previousConfig) })
	client, err := gredis.New(&gredis.Config{Address: server.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	shared := g.Redis()
	previousRedis := shared.GetAdapter()
	shared.SetAdapter(client.GetAdapter())
	t.Cleanup(func() {
		shared.SetAdapter(previousRedis)
		_ = client.Close(context.Background())
	})
	t.Setenv("YOUBAN_PUBLISH_PUSH_ENGINE", pushEngineUnifiedStream)
	ctx := context.Background()
	chat := "-100999999950"
	service := &sSysPublish{}
	for i := 0; i < 3; i++ {
		lease, ok, err := tryTelegramCleanupChannelSlot(ctx, chat, channelPriorityCycleDelete)
		if err != nil || !ok {
			t.Fatalf("automatic slot %d: acquired=%t err=%v", i, ok, err)
		}
		defer service.releaseTelegramChannelLease(ctx, lease)
	}
	if _, ok, err := tryTelegramCleanupChannelSlot(ctx, chat, channelPriorityCycleDelete); err != nil || ok {
		t.Fatalf("automatic job used reserved slot: acquired=%t err=%v", ok, err)
	}
	job := telegramJobRecord{TargetChatId: chat, Priority: tgJobPriorityBulk}
	called := false
	err = service.withTelegramCleanupExecution(ctx, job, func(context.Context) error { called = true; return nil })
	var retry *tgRetryAfterError
	if !errors.As(err, &retry) || called {
		t.Fatalf("automatic stream task bypassed reservation: called=%t err=%v", called, err)
	}
	err = service.withTelegramCleanupExecution(ctx, job, func(context.Context) error {
		called = true
		if _, ok, err := tryTelegramCleanupChannelSlot(ctx, chat, channelPriorityManualDelete); err != nil || ok {
			t.Fatalf("total channel limit exceeded: acquired=%t err=%v", ok, err)
		}
		return nil
	}, channelPriorityManualDelete)
	if err != nil || !called {
		t.Fatalf("manual stream task did not use reserved slot: called=%t err=%v", called, err)
	}
	lease, ok, err := tryTelegramCleanupChannelSlot(ctx, chat, channelPriorityManualDelete)
	if err != nil || !ok {
		t.Fatalf("stream callback did not release slot: acquired=%t err=%v", ok, err)
	}
	service.releaseTelegramChannelLease(ctx, lease)
	other, ok, err := tryTelegramCleanupChannelSlot(ctx, "-100999999951", channelPriorityCycleDelete)
	if err != nil || !ok {
		t.Fatalf("different channel was blocked: acquired=%t err=%v", ok, err)
	}
	service.releaseTelegramChannelLease(ctx, other)
}

func TestTelegramCleanupJobLeasePreventsDuplicateExecution(t *testing.T) {
	server := miniredis.RunT(t)
	previousConfig := g.Cfg().GetAdapter()
	adapter, err := gcfg.NewAdapterContent(fmt.Sprintf(`{"redis":{"default":{"address":%q}}}`, server.Addr()))
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	t.Cleanup(func() { g.Cfg().SetAdapter(previousConfig) })
	client, err := gredis.New(&gredis.Config{Address: server.Addr()})
	if err != nil {
		t.Fatal(err)
	}
	shared := g.Redis()
	previousRedis := shared.GetAdapter()
	shared.SetAdapter(client.GetAdapter())
	t.Cleanup(func() {
		shared.SetAdapter(previousRedis)
		_ = client.Close(context.Background())
	})

	service := &sSysPublish{}
	job := telegramJobRecord{Id: 88001, TargetChatId: "-100999999960"}
	started := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- service.withTelegramCleanupJobLease(context.Background(), job.Id, func(ctx context.Context) error {
			close(started)
			<-release
			return ctx.Err()
		})
	}()
	<-started

	secondErr := service.withTelegramCleanupJobLease(context.Background(), job.Id, func(context.Context) error {
		return errors.New("duplicate execution")
	})
	var retry *tgRetryAfterError
	if !errors.As(secondErr, &retry) {
		t.Fatalf("duplicate cleanup was not rejected: %v", secondErr)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first cleanup failed: %v", err)
	}
}
