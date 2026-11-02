package sys

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
)

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
	now := time.Unix(1800000000, 0)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := enqueueCollectSourceStatsTaskAt(context.Background(), client, 1421, now); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := enqueueCollectSourceStatsTaskAt(context.Background(), client, 1422, now); err != nil {
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

func TestCollectSourceStatsNextWindowSurvivesActiveTask(t *testing.T) {
	redis := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	client := asynq.NewClient(opt)
	defer client.Close()
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	now := time.Unix(1800000000, 0)
	// Keeping the earlier task's unique lock models the completion/ACK window.
	for _, at := range []time.Time{now, now.Add(30 * time.Second), now.Add(31 * time.Second)} {
		if err := enqueueCollectSourceStatsTaskAt(context.Background(), client, 1421, at); err != nil {
			t.Fatal(err)
		}
	}
	tasks, err := inspector.ListScheduledTasks(tgQueueNameBackground)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("next-window refresh was swallowed: tasks=%d", len(tasks))
	}
	for _, task := range tasks {
		var payload collectSourceStatsPayload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		earliest := time.Unix((payload.Window+1)*30+5, 0)
		if task.NextProcessAt.Before(earliest) {
			t.Fatal("refresh executes before its window closes")
		}
	}
}
