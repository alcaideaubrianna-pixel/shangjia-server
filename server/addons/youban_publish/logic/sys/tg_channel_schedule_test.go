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
			{"earlier manual FIFO", "pending", "", "profile:1", 1},
			{"earlier batch FIFO", "pending", "", "batchtext:1", 1},
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
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
