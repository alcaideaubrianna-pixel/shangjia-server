package sys

import (
	"context"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

// Only a disposable loopback PostgreSQL is accepted. No production credentials
// or Telegram clients are used by this test.
func TestIngestSpoolPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_SPOOL_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	ctx := context.Background()
	ddl, err := os.ReadFile("../../resource/sql/ingest_spool.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := g.DB().Exec(ctx, string(ddl)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := g.DB().Exec(ctx, "TRUNCATE "+collectorIngestSpoolTable+" RESTART IDENTITY"); err != nil {
		t.Fatal(err)
	}
	event := &sysin.RawUpdateEvent{TenantID: 1, SourceID: 10, SourceType: sysin.SourceTypeAccount, EventID: "test:spool:1", MessageID: 100}
	id, err := persistIngestSpool(ctx, event)
	if err != nil || id <= 0 {
		t.Fatalf("persist: id=%d err=%v", id, err)
	}
	duplicateID, err := persistIngestSpool(ctx, event)
	if err != nil || duplicateID != id {
		t.Fatalf("duplicate: id=%d err=%v", duplicateID, err)
	}
	if _, err := g.DB().Exec(ctx, "UPDATE "+collectorIngestSpoolTable+" SET status='failed_retry',attempt_count=3,next_run_at=now()+interval '1 hour' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if _, err := persistIngestSpool(ctx, event); err != nil {
		t.Fatal(err)
	}
	row, err := g.DB().GetOne(ctx, "SELECT status,attempt_count,next_run_at>now() AS deferred FROM "+collectorIngestSpoolTable+" WHERE id=?", id)
	if err != nil || row["status"].String() != "failed_retry" || row["attempt_count"].Int() != 3 || !row["deferred"].Bool() {
		t.Fatalf("duplicate changed retry scheduling: %v %v", row, err)
	}
	if _, err := g.DB().Exec(ctx, "UPDATE "+collectorIngestSpoolTable+" SET status='received' WHERE id=?", id); err != nil {
		t.Fatal(err)
	}
	if err := completeIngestSpool(ctx, id); err != nil {
		t.Fatal(err)
	}
	event.MessageID = 200
	if _, err := persistIngestSpool(ctx, event); err != nil {
		t.Fatal(err)
	}
	row, err = g.DB().GetOne(ctx, "SELECT status,(payload->>'messageId')::bigint AS message_id FROM "+collectorIngestSpoolTable+" WHERE id=?", id)
	if err != nil || row["status"].String() != "completed" || row["message_id"].Int64() != 100 {
		t.Fatalf("completed row reopened: %v %v", row, err)
	}
	other := *event
	other.TenantID = 2
	if otherID, err := persistIngestSpool(ctx, &other); err != nil || otherID == id {
		t.Fatalf("tenant collision: id=%d err=%v", otherID, err)
	}
	if _, err := g.DB().Exec(ctx, "UPDATE "+collectorIngestSpoolTable+" SET status='completed'"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.DB().Exec(ctx, "INSERT INTO "+collectorIngestSpoolTable+" (tenant_id,event_key,source_type,source_id,payload) VALUES (1,'invalid','account',10,'{}')"); err != nil {
		t.Fatal(err)
	}
	if err := (&sCollector{}).recoverIngestSpool(ctx, 10); err != nil {
		t.Fatal(err)
	}
	row, err = g.DB().GetOne(ctx, "SELECT status,last_error FROM "+collectorIngestSpoolTable+" WHERE event_key='invalid'")
	if err != nil || row["status"].String() != "dead" || row["last_error"].String() == "" {
		t.Fatalf("invalid event was not quarantined: %v %v", row, err)
	}
	t.Run("expired lease fences late acknowledgement", func(t *testing.T) {
		event.EventID = "lease-test"
		id, err := persistIngestSpool(ctx, event)
		if err != nil {
			t.Fatal(err)
		}
		first, err := claimIngestSpool(ctx)
		if err != nil || first["id"].Int64() != id {
			t.Fatal("claim failed", err)
		}
		if another, err := claimIngestSpool(ctx); err != nil || !another.IsEmpty() {
			t.Fatal("active lease claimed twice", err)
		}
		if _, err := g.DB().Exec(ctx, "UPDATE "+collectorIngestSpoolTable+" SET next_run_at=now()-interval '1 second' WHERE id=?", id); err != nil {
			t.Fatal(err)
		}
		second, err := claimIngestSpool(ctx)
		if err != nil || second["attempt_count"].Int() != first["attempt_count"].Int()+1 {
			t.Fatal("expired lease not reclaimed", err)
		}
		if err := finishIngestSpool(ctx, first, "completed", ""); err != nil {
			t.Fatal(err)
		}
		state, err := g.DB().GetOne(ctx, "SELECT status FROM "+collectorIngestSpoolTable+" WHERE id=?", id)
		if err != nil || state["status"].String() != "processing" {
			t.Fatal("late worker overwrote current owner", err)
		}
		if err := finishIngestSpool(ctx, second, "completed", ""); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("concurrent workers claim disjoint rows", func(t *testing.T) {
		for i := 0; i < 12; i++ {
			event.EventID = "parallel:" + strconv.Itoa(i)
			if _, err := persistIngestSpool(ctx, event); err != nil {
				t.Fatal(err)
			}
		}
		var wg sync.WaitGroup
		claims := make(chan gdb.Record, 12)
		errors := make(chan error, 12)
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				row, err := claimIngestSpool(ctx)
				claims <- row
				errors <- err
			}()
		}
		wg.Wait()
		close(claims)
		close(errors)
		for err := range errors {
			if err != nil {
				t.Fatal(err)
			}
		}
		seen := map[int64]bool{}
		for row := range claims {
			id := row["id"].Int64()
			if id <= 0 || seen[id] {
				t.Fatalf("duplicate or missing claim: %d", id)
			}
			seen[id] = true
		}
		if len(seen) != 12 {
			t.Fatal("lost claim")
		}
	})
	t.Run("recover after queue outage", func(t *testing.T) {
		if _, err := g.DB().Exec(ctx, "UPDATE "+collectorIngestSpoolTable+" SET status='completed'"); err != nil {
			t.Fatal(err)
		}
		ddl, err := os.ReadFile("../../resource/sql/install.pgsql.sql")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := g.DB().Exec(ctx, string(ddl)); err != nil {
			t.Fatal(err)
		}
		redis := miniredis.RunT(t)
		client := asynq.NewClient(asynq.RedisClientOpt{Addr: redis.Addr()})
		defer client.Close()
		collector := &sCollector{queueClient: client}
		event.EventID = "queue-outage-test"
		event.Priority = sysin.EventPriorityRealtime
		event.ReceivedAt = time.Now()
		event.RawUpdate = []byte(`{"message_id":100}`)
		id, err := persistIngestSpool(ctx, event)
		if err != nil {
			t.Fatal(err)
		}
		redis.SetError("ERR injected queue outage")
		if err := collector.recoverIngestSpool(ctx, 1); err != nil {
			t.Fatal(err)
		}
		row, err := g.DB().GetOne(ctx, "SELECT status,attempt_count,last_error FROM "+collectorIngestSpoolTable+" WHERE id=?", id)
		if err != nil || row["status"].String() != "failed_retry" || row["attempt_count"].Int() != 1 || row["last_error"].String() == "" {
			t.Fatal("queue failure not durable", row, err)
		}
		redis.SetError("")
		if _, err := g.DB().Exec(ctx, "UPDATE "+collectorIngestSpoolTable+" SET next_run_at=now()-interval '1 second' WHERE id=?", id); err != nil {
			t.Fatal(err)
		}
		if err := collector.recoverIngestSpool(ctx, 1); err != nil {
			t.Fatal(err)
		}
		row, err = g.DB().GetOne(ctx, "SELECT status,attempt_count FROM "+collectorIngestSpoolTable+" WHERE id=?", id)
		if err != nil || row["status"].String() != "completed" || row["attempt_count"].Int() != 2 {
			t.Fatal("queue recovery not completed", row, err)
		}
		row, err = g.DB().GetOne(ctx, "SELECT count(*) AS n FROM hg_tg_collector_event WHERE tenant_id=? AND event_key=?", event.TenantID, event.EventID)
		if err != nil || row["n"].Int() != 1 {
			t.Fatal("event replay duplicated", row, err)
		}
	})
}
