package sys

import (
	"database/sql"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
	_ "modernc.org/sqlite"

	"hotgo/utility/dbtime"
)

func TestTelegramStaleDispatchRecoveryPreservesFutureDue(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	cutoff := telegramRecoveryTimeText(gtime.NewFromTime(now.In(dbtime.Location())))
	if cutoff != "2026-11-16 20:00:00" {
		t.Fatalf("business cutoff=%s", cutoff)
	}
	if _, err = db.Exec("CREATE TABLE jobs (id INTEGER PRIMARY KEY, next_retry_at TEXT, dispatch_status TEXT, status TEXT DEFAULT 'pending')"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO jobs (id,next_retry_at,dispatch_status) VALUES (1,NULL,'queued'),(2,'2026-11-16 19:59:59','queued'),(3,'2026-11-16 20:00:00','queued'),(4,'2026-11-16 20:01:00','queued')"); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query("SELECT id FROM jobs WHERE ("+telegramStaleDispatchDueCondition+") ORDER BY id", cutoff)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if len(ids) != 3 || ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
		t.Fatalf("selected ids=%v", ids)
	}
	// Simulate the worker installing a future deadline after selection.
	if _, err = db.Exec("UPDATE jobs SET next_retry_at='2026-11-16 20:02:00' WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("UPDATE jobs SET status='sent' WHERE id=3"); err != nil {
		t.Fatal(err)
	}
	result, err := db.Exec("UPDATE jobs SET dispatch_status='idle',next_retry_at=? WHERE id IN (1,2,3) AND status IN ('pending','failed_retry','unknown','confirming') AND dispatch_status='queued' AND ("+telegramStaleDispatchDueCondition+")", cutoff, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		t.Fatalf("affected=%d err=%v", n, err)
	}
	var due, status string
	if err = db.QueryRow("SELECT next_retry_at,dispatch_status FROM jobs WHERE id=2").Scan(&due, &status); err != nil {
		t.Fatal(err)
	}
	if due != "2026-11-16 20:02:00" || status != "queued" {
		t.Fatalf("future deadline overwritten: %s %s", due, status)
	}
}
