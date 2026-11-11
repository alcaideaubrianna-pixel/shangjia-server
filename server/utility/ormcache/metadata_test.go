package ormcache

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
)

// TestMetadataPoolCycleLocalPostgres 复现外部冷查询持有缓存锁、事务占满连接池时的循环等待。
func TestMetadataPoolCycleLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_STREAM_MEDIA_PG") != "1" {
		t.Skip("requires local youban_hotgo PostgreSQL")
	}
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("nonblocking=%v", enabled), func(t *testing.T) {
			group := fmt.Sprintf("ormcache_%d", time.Now().UnixNano())
			gdb.SetConfigGroup(group, gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "5432", User: "postgres", Pass: "postgres", Name: "youban_hotgo", MaxOpenConnCount: 1, MaxIdleConnCount: 1}})
			db, err := gdb.Instance(group)
			if err != nil {
				t.Fatal(err)
			}
			metadata := db.GetCore().GetInnerMemCache()
			original := metadata.GetAdapter()
			t.Cleanup(func() { metadata.SetAdapter(original) })
			if enabled {
				EnableNonBlockingMetadata(db)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if exists, err := db.GetCore().HasTable("hg_youban_publish_tg_job"); err != nil || !exists {
				t.Fatal("missing fixture tables")
			}
			if err := db.GetCore().ClearTableFields(ctx, "hg_youban_publish_tg_attempt"); err != nil {
				t.Fatal(err)
			}
			pool, err := db.Master()
			if err != nil {
				t.Fatal(err)
			}
			var coldResult = make(chan error, 1)
			var wait time.Duration
			err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
				previousWaits := pool.Stats().WaitCount
				go func() {
					coldCtx, coldCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
					defer coldCancel()
					_, err := db.TableFields(coldCtx, "hg_youban_publish_tg_attempt")
					coldResult <- err
				}()
				for pool.Stats().WaitCount == previousWaits {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(time.Millisecond):
					}
				}
				started := time.Now()
				if exists, err := db.GetCore().HasTable("hg_youban_publish_tg_job"); err != nil || !exists {
					t.Fatal("lost cached table list")
				}
				wait = time.Since(started)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			coldErr := <-coldResult
			t.Logf("cached metadata read blocked %s; cold query error=%v", wait, coldErr)
			if enabled && (wait >= 200*time.Millisecond || coldErr != nil) {
				t.Fatalf("connection/cache cycle persists: wait=%s err=%v", wait, coldErr)
			}
			if !enabled && wait < 300*time.Millisecond {
				t.Fatalf("baseline failed to reproduce cache lock contention: %s", wait)
			}
		})
	}
}
