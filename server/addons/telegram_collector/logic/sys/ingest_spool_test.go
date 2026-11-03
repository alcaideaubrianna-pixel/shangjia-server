package sys

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
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
}
