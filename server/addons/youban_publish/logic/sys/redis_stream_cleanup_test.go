package sys

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
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
