package sys

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
)

type watchdogBackfillTestTX struct {
	gdb.TX
	ctx      context.Context
	queryCtx context.Context
	err      error
	calls    int
}

func (tx *watchdogBackfillTestTX) Ctx(ctx context.Context) gdb.TX { tx.ctx = ctx; return tx }
func (tx *watchdogBackfillTestTX) GetCtx() context.Context        { return tx.ctx }
func (tx *watchdogBackfillTestTX) GetValue(_ string, _ ...any) (gdb.Value, error) {
	tx.calls++
	tx.queryCtx = tx.ctx
	return gvar.New(42), tx.err
}

func TestTelegramWatchdogBackfillRequiresExplicitScope(t *testing.T) {
	for _, scope := range [][5]int64{{0, 2, 0, 100, 10}, {1, 0, 0, 100, 10}, {1, 2, -1, 100, 10}, {1, 2, 100, 100, 10}, {1, 2, 0, 100, 0}, {1, 2, 0, 100, 501}} {
		tx := new(watchdogBackfillTestTX)
		cursor, err := new(sSysPublish).backfillTelegramSendDeadlines(context.Background(), tx, scope[0], scope[1], scope[2], scope[3], int(scope[4]))
		if err == nil || cursor != scope[2] || tx.calls != 0 {
			t.Fatalf("unbounded backfill accepted: scope=%v cursor=%d err=%v calls=%d", scope, cursor, err, tx.calls)
		}
	}
	if _, err := new(sSysPublish).backfillTelegramSendDeadlines(context.Background(), nil, 1, 2, 0, 100, 10); err == nil {
		t.Fatal("backfill accepted missing transaction")
	}
}

func TestTelegramWatchdogBackfillFailureKeepsCursorAndBudget(t *testing.T) {
	want := errors.New("historical lock failure")
	tx := &watchdogBackfillTestTX{err: want}
	parent := context.Background()
	cursor, err := new(sSysPublish).backfillTelegramSendDeadlines(parent, tx, 1, 2, 7, 100, 10)
	if !errors.Is(err, want) || cursor != 7 || tx.calls != 1 {
		t.Fatalf("failed backfill advanced cursor: cursor=%d err=%v calls=%d", cursor, err, tx.calls)
	}
	deadline, ok := tx.queryCtx.Deadline()
	if !ok || time.Until(deadline) > 8*time.Second || parent.Err() != nil {
		t.Fatal("historical work changed the caller budget")
	}
	if tx.ctx != nil {
		t.Fatal("backfill did not restore the transaction context")
	}
}

func TestTelegramWatchdogBackfillScopedLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_WATCHDOG_TEST_PG_PORT")
	n, err := strconv.Atoi(port)
	if port == "" {
		t.Skip("requires explicitly selected disposable local PostgreSQL port")
	}
	if err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable local PostgreSQL port")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			"CREATE TEMP TABLE " + publishTgJobTable + " (id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,status text,send_phase text,send_deadline_at timestamp,created_at timestamp,updated_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE " + publishTgAttemptTable + " (job_id bigint,created_at timestamp) ON COMMIT DROP",
			"INSERT INTO " + publishTgJobTable + " VALUES (1,1,2,'sending','',NULL,'2026-01-01','2026-01-02'),(2,1,2,'pending','',NULL,'2026-01-01','2026-01-02'),(3,1,3,'sending','',NULL,'2026-01-01','2026-01-02'),(4,1,2,'failed_retry','display_confirmed',NULL,'2026-01-01','2026-01-02'),(5,1,2,'sent','',NULL,'2026-01-01','2026-01-02'),(6,1,2,'unknown','',NULL,'2026-01-01',NULL),(7,3,2,'sending','',NULL,'2026-01-01','2026-01-02'),(8,1,2,'sending','',NULL,'2026-01-01','2026-01-02')",
			"INSERT INTO " + publishTgAttemptTable + " VALUES (1,'2026-01-01 01:00'),(1,'2026-01-01 02:00')",
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		cursor := int64(0)
		for _, want := range []int64{2, 5, 6} {
			cursor, err = new(sSysPublish).backfillTelegramSendDeadlines(ctx, tx, 1, 2, cursor, 6, 2)
			if err != nil {
				return err
			}
			if cursor != want {
				t.Fatalf("cursor=%d want=%d", cursor, want)
			}
		}
		cursor, err = new(sSysPublish).backfillTelegramSendDeadlines(ctx, tx, 1, 2, 0, 6, 500)
		if err != nil {
			return err
		}
		if cursor != 6 {
			t.Fatalf("idempotent rerun cursor=%d want=6", cursor)
		}
		cursor, err = new(sSysPublish).backfillTelegramSendDeadlines(ctx, tx, 1, 2, 6, 7, 2)
		if err != nil {
			return err
		}
		if cursor != 6 {
			t.Fatalf("empty page advanced cursor=%d", cursor)
		}
		for id, want := range map[int64]string{1: "2026-01-01 01:10:00", 4: "2026-01-02 00:10:00", 6: "2026-01-01 00:10:00"} {
			v, err := tx.GetValue("SELECT send_deadline_at FROM "+publishTgJobTable+" WHERE id=?", id)
			if err != nil {
				return err
			}
			if gtime.New(v.Val()).Format("Y-m-d H:i:s") != want {
				t.Fatalf("job=%d deadline=%v want=%s", id, v, want)
			}
		}
		v, err := tx.GetValue("SELECT count(*) FROM " + publishTgJobTable + " WHERE id IN (2,3,5,7,8) AND send_deadline_at IS NOT NULL")
		if err != nil {
			return err
		}
		if v.Int() != 0 {
			t.Fatal("backfill changed an ineligible or out-of-scope job")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
