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

		// Replay the production backlog shapes seen on 2026-10-24. IDs and
		// timestamps are anonymized, but operation classes, duplicate clicks,
		// channel fan-out and active verification phases match the live cases.
		if _, err = tx.Exec(`CREATE TEMP TABLE production_schedule_fixture(
			id bigint,status text,send_phase text,operation_no text,
			created_at timestamp,profile_id bigint,channel_id bigint
		)`); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO production_schedule_fixture VALUES
			(101,'pending','','ai-republish:old',timestamp '2026-10-22 19:42:58',7001,501),
			(102,'pending','','cycle_batch:old',timestamp '2026-10-23 08:00:00',7002,501),
			(103,'pending','','collect:old',timestamp '2026-10-24 15:00:00',7003,501),
			(104,'pending','','bot_publish:first',timestamp '2026-10-24 15:16:49',7004,501),
			(105,'pending','','bot_publish:duplicate',timestamp '2026-10-24 15:17:01',7004,501),
			(106,'pending','','batchtext:manual',timestamp '2026-10-24 15:18:00',7005,501),
			(201,'sending','verify_sending','ai-republish:started',timestamp '2026-10-22 19:41:47',7101,502),
			(202,'pending','','bot_publish:waiting',timestamp '2026-10-24 19:24:01',7102,502),
			(301,'pending','','bot_publish:fanout',timestamp '2026-10-24 20:58:24',7201,503),
			(302,'pending','','bot_publish:fanout',timestamp '2026-10-24 20:58:24',7201,504),
			(303,'pending','','bot_publish:fanout',timestamp '2026-10-24 20:58:24',7201,505)
		`); err != nil {
			return err
		}

		rows, err = tx.GetAll(`
			SELECT DISTINCT ON (channel_id) id,channel_id
			FROM production_schedule_fixture j
			ORDER BY channel_id, ` + telegramChannelScheduleOrderSQL("j"))
		if err != nil {
			return err
		}
		wantHeads := map[int64]int64{501: 104, 502: 201, 503: 301, 504: 302, 505: 303}
		if len(rows) != len(wantHeads) {
			t.Fatalf("production fixture returned %d channel heads, want %d", len(rows), len(wantHeads))
		}
		for _, fixtureRow := range rows {
			channelID := fixtureRow["channel_id"].Int64()
			if got, want := fixtureRow["id"].Int64(), wantHeads[channelID]; got != want {
				t.Fatalf("production fixture channel %d head=%d want=%d", channelID, got, want)
			}
		}

		// Repeated clicks for the same profile intentionally remain FIFO here.
		// Replacement happens in prepareProfileChannelPublish when the newer
		// generation reaches the sender; the scheduler must not reorder it ahead
		// of an already-started generation.
		row, err = tx.GetOne(`SELECT ` + telegramChannelSchedulePrecedesSQL("a", "b") + ` AS precedes
			FROM production_schedule_fixture a CROSS JOIN production_schedule_fixture b
			WHERE a.id=104 AND b.id=105`)
		if err != nil {
			return err
		}
		if !row["precedes"].Bool() {
			t.Fatal("same-profile BOT generations lost FIFO ordering")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
