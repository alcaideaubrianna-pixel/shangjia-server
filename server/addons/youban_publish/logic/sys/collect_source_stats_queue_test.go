package sys

import (
	"context"

	"os"
	"strings"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
)

func TestCollectSourceStatsBatchLimit(t *testing.T) {
	old := os.Getenv("YOUBAN_PUBLISH_COLLECT_SOURCE_STATS_BATCH_SIZE")
	t.Cleanup(func() { _ = os.Setenv("YOUBAN_PUBLISH_COLLECT_SOURCE_STATS_BATCH_SIZE", old) })

	for _, tc := range []struct {
		name  string
		value string
		want  int
	}{
		{name: "default", value: "", want: 32},
		{name: "invalid", value: "bad", want: 32},
		{name: "bounded low", value: "4", want: 4},
		{name: "bounded high", value: "999", want: 128},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.Setenv("YOUBAN_PUBLISH_COLLECT_SOURCE_STATS_BATCH_SIZE", tc.value); err != nil {
				t.Fatal(err)
			}
			if got := collectSourceStatsBatchLimit(); got != tc.want {
				t.Fatalf("collectSourceStatsBatchLimit() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestCollectSourceStatsAggregateLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_event(id bigint,source_id bigint,status text,material_role text,received_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch(source_id bigint,event_id bigint,status text) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_source(id bigint,event_total bigint,success_total bigint,failed_total bigint,last_event_at timestamp,updated_at timestamp,deleted_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_source VALUES(1,0,0,0,NULL,NULL,NULL),(2,0,0,0,NULL,NULL,NULL)`,
			`INSERT INTO hg_youban_publish_collect_event VALUES(1,1,'pending','display','2026-11-01'),(2,1,'failed','verify','2026-11-02'),(3,1,'failed','display','2026-11-03'),(4,1,'ignored',NULL,'2026-11-04'),(5,2,'failed','display','2026-11-05')`,
			`INSERT INTO hg_youban_publish_collect_dispatch VALUES(1,1,'sent'),(1,1,'sent'),(1,3,'failed'),(2,5,'sent')`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		if err := new(sSysPublish).syncCollectSourceStats(ctx, 1); err != nil {
			return err
		}
		row, err := tx.GetOne(`SELECT * FROM hg_youban_publish_collect_source WHERE id=1`)
		if err != nil {
			return err
		}
		if row["event_total"].Int64() != 4 || row["failed_total"].Int64() != 1 || row["success_total"].Int64() != 1 {
			t.Fatalf("wrong aggregate: %v", row)
		}
		if row["last_event_at"].GTime().Format("Y-m-d") != "2026-11-04" {
			t.Fatal("wrong last event", row)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCollectSourceStatsConcurrentTriggersCoalesce(t *testing.T) {
	redis := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(opt)
	defer client.Close()
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := enqueueCollectSourceStatsTask(context.Background(), client, 1421); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := enqueueCollectSourceStatsTask(context.Background(), client, 1422); err != nil {
		t.Fatal(err)
	}
	tasks, err := inspector.ListScheduledTasks(tgQueueNameBackground)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("40 same-source triggers plus another source produced %d tasks, want 2", len(tasks))
	}
	for _, task := range tasks {
		if task.Type != tgTaskTypeCollectSourceStats {
			t.Fatal(task.Type)
		}
	}
}

func TestCollectSourceStatsDurableRecoveryLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	redis := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(opt)
	defer client.Close()
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		ddl, err := os.ReadFile("../../resource/sql/collect_source_stats_work.pgsql.sql")
		if err != nil {
			return err
		}
		sql := strings.Replace(string(ddl), "CREATE TABLE IF NOT EXISTS", "CREATE TEMP TABLE", 1)
		sql = strings.Replace(sql, ");", ") ON COMMIT DROP;", 1)
		if _, err = tx.Exec(sql); err != nil {
			return err
		}
		broken := asynq.NewClient(opt)
		broken.Close()
		service := &sSysPublish{tgQueueClient: broken}
		if err = service.scheduleCollectSourceStats(ctx, 1421); err == nil {
			t.Fatal("closed Redis client unexpectedly succeeded")
		}
		for i := 0; i < 1000; i++ {
			if err = markCollectSourceStatsDirty(ctx, 1421); err != nil {
				return err
			}
		}
		row, err := tx.GetOne("SELECT COUNT(*) n FROM " + collectSourceStatsWorkTable)
		if err != nil {
			return err
		}
		if row["n"].Int() != 1 {
			t.Fatal("dirty work grew with triggers")
		}
		if _, err = tx.Exec("UPDATE " + collectSourceStatsWorkTable + " SET available_at=CURRENT_TIMESTAMP-INTERVAL '1 minute'"); err != nil {
			return err
		}
		service.tgQueueClient = client
		if err = service.recoverCollectSourceStats(ctx, 100); err != nil {
			return err
		}
		tasks, err := inspector.ListScheduledTasks(tgQueueNameBackground)
		if err != nil {
			return err
		}
		if len(tasks) != 1 {
			t.Fatalf("lost enqueue was not recovered: %d", len(tasks))
		}
		// A concurrent event arriving during counting must survive conditional ACK.
		if err = refreshCollectSourceStatsWork(ctx, 1421, func(ctx context.Context, id int64) error { return markCollectSourceStatsDirty(ctx, id) }); err != nil {
			return err
		}
		row, err = tx.GetOne("SELECT pending FROM " + collectSourceStatsWorkTable + " WHERE source_id=1421")
		if err != nil {
			return err
		}
		if row["pending"].Int() != 1 {
			t.Fatal("concurrent generation lost")
		}
		calls := 0
		refresh := func(context.Context, int64) error { calls++; return nil }
		if err = refreshCollectSourceStatsWork(ctx, 1421, refresh); err != nil {
			return err
		}
		if err = refreshCollectSourceStatsWork(ctx, 1421, refresh); err != nil {
			return err
		}
		if calls != 1 {
			t.Fatalf("clean source redundantly scanned %d times", calls)
		}
		// A trigger after the task completes still has durable recovery, even
		// while the previous Asynq uniqueness lock has not been released.
		if err = service.scheduleCollectSourceStats(ctx, 1421); err != nil {
			return err
		}
		row, err = tx.GetOne("SELECT pending FROM " + collectSourceStatsWorkTable + " WHERE source_id=1421")
		if err != nil {
			return err
		}
		if row["pending"].Int() != 1 {
			t.Fatal("post-refresh trigger lost")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
