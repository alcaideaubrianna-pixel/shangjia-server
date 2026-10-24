package sys

import (
	"context"
	"github.com/gogf/gf/v2/database/gdb"
	"os"
	"testing"
)

func TestTelegramChannelScheduleLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_PERMISSION_LOCAL_PG") != "1" {
		t.Skip("requires isolated local PostgreSQL")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		_, err := tx.Exec(`CREATE TEMP TABLE schedule_jobs(id bigint,status text,send_phase text,operation_no text,created_at timestamp)`)
		if err != nil {
			return err
		}
		cases := []struct {
			name, status, phase, operation string
			want                           int64
		}{
			{"untouched cycle", "pending", "", "cycle_batch:1", 2},
			{"queued cycle", "pending", "", "cycle_batch:1", 2},
			{"untouched collect", "pending", "", "collect:1", 2},
			{"AI repair is not interactive", "pending", "", "ai-republish:1", 2},
			{"display sent", "failed_retry", "display_confirmed", "collect:1", 1},
			{"display request pending", "failed_retry", "display_sending", "cycle_batch:1", 1},
			{"verification request pending", "failed_retry", "verify_sending", "collect:1", 1},
			{"active worker", "sending", "", "cycle_batch:1", 1},
			{"confirmation pending", "confirming", "", "collect:1", 1},
			{"cycle cleanup already started", "failed_retry", "cleanup_processing", "cycle_batch:1", 1},
			{"earlier manual FIFO", "pending", "", "profile:1", 1},
			{"earlier batch FIFO", "pending", "", "batchtext:1", 1},
			{"BOT publish is manual priority", "pending", "", "bot_publish:1", 1},
		}
		for _, c := range cases {
			if _, err = tx.Exec("TRUNCATE schedule_jobs"); err != nil {
				return err
			}
			if _, err = tx.Exec("INSERT INTO schedule_jobs VALUES(1,?,?,?,timestamp '2026-10-17 10:00:00'),(2,'pending','','profile:2',timestamp '2026-10-17 10:01:00')", c.status, c.phase, c.operation); err != nil {
				return err
			}
			row, e := tx.GetOne("SELECT j.id FROM schedule_jobs j ORDER BY " + telegramChannelScheduleOrderSQL("j") + " LIMIT 1")
			if e != nil {
				return e
			}
			if row["id"].Int64() != c.want {
				t.Fatalf("%s head=%d want=%d", c.name, row["id"].Int64(), c.want)
			}
			pred, e := tx.GetOne("SELECT " + telegramChannelSchedulePrecedesSQL("a", "b") + " AS precedes FROM schedule_jobs a CROSS JOIN schedule_jobs b WHERE a.id=1 AND b.id=2")
			if e != nil {
				return e
			}
			if pred["precedes"].Bool() != (c.want == 1) {
				t.Fatalf("%s comparator differs from head ordering", c.name)
			}
		}
		// Older cycles must not block newer collections or manual submissions.
		// Test the actual PostgreSQL ORDER BY and every pairwise barrier together.
		if _, err = tx.Exec("TRUNCATE schedule_jobs"); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO schedule_jobs VALUES
			(1,'pending','','cycle_batch:1',timestamp '2026-10-17 09:00:00'),
			(2,'pending','','collect:2',timestamp '2026-10-17 10:00:00'),
			(3,'pending','','profile:3',timestamp '2026-10-17 11:00:00'),
			(4,'pending','','batchtext:4',timestamp '2026-10-17 11:00:00'),
			(5,'pending','','collect:5',timestamp '2026-10-17 10:00:00'),
			(6,'pending','',' CYCLE_BATCH:6 ',timestamp '2026-10-17 09:00:00'),
			(7,'failed_retry','verify_sending','cycle_batch:7',timestamp '2026-10-17 12:00:00')`); err != nil {
			return err
		}
		rows, err := tx.GetAll("SELECT j.id FROM schedule_jobs j ORDER BY " + telegramChannelScheduleOrderSQL("j"))
		if err != nil {
			return err
		}
		want := []int64{7, 3, 4, 2, 5, 1, 6}
		if len(rows) != len(want) {
			t.Fatalf("schedule returned %d rows, want %d", len(rows), len(want))
		}
		for i, id := range want {
			if rows[i]["id"].Int64() != id {
				t.Fatalf("position %d: got %d want %d", i, rows[i]["id"].Int64(), id)
			}
			for j, otherID := range want {
				row, err := tx.GetOne("SELECT "+telegramChannelSchedulePrecedesSQL("a", "b")+" AS precedes FROM schedule_jobs a CROSS JOIN schedule_jobs b WHERE a.id=? AND b.id=?", id, otherID)
				if err != nil {
					return err
				}
				if row["precedes"].Bool() != (i < j) {
					t.Fatalf("barrier differs from head order: %d before %d", id, otherID)
				}
			}
		}
		// A stale untouched AI republish must not block a new BOT manual publish.
		if _, err = tx.Exec("TRUNCATE schedule_jobs"); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO schedule_jobs VALUES
			(1,'pending','','ai-republish:old',timestamp '2026-10-17 09:00:00'),
			(2,'pending','','bot_publish:new',timestamp '2026-10-17 10:00:00')`); err != nil {
			return err
		}
		row, err := tx.GetOne("SELECT j.id FROM schedule_jobs j ORDER BY " + telegramChannelScheduleOrderSQL("j") + " LIMIT 1")
		if err != nil {
			return err
		}
		if row["id"].Int64() != 2 {
			t.Fatalf("BOT manual publish was blocked by stale AI republish: head=%d", row["id"].Int64())
		}
		row, err = tx.GetOne("SELECT " + telegramChannelSchedulePrecedesSQL("a", "b") + " AS precedes FROM schedule_jobs a CROSS JOIN schedule_jobs b WHERE a.id=1 AND b.id=2")
		if err != nil {
			return err
		}
		if row["precedes"].Bool() {
			t.Fatal("stale AI republish still precedes BOT manual publish")
		}
		// Once the older operation has started, it remains a hard ordering barrier.
		if _, err = tx.Exec("UPDATE schedule_jobs SET status='sending' WHERE id=1"); err != nil {
			return err
		}
		row, err = tx.GetOne("SELECT " + telegramChannelSchedulePrecedesSQL("a", "b") + " AS precedes FROM schedule_jobs a CROSS JOIN schedule_jobs b WHERE a.id=1 AND b.id=2")
		if err != nil {
			return err
		}
		if !row["precedes"].Bool() {
			t.Fatal("started AI republish must remain a hard ordering barrier")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
