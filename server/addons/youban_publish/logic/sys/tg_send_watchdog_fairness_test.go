package sys

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
)

func TestTelegramTimeoutCleanupFairnessPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_HEAD_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL on port 35432")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("CREATE TEMP TABLE " + publishTgJobTable + " (id bigint PRIMARY KEY, tenant_id bigint, account_id bigint, send_phase text, timeout_cleanup_at timestamp)"); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO " + publishTgJobTable + " SELECT i,1,1,'timeout_cancelled',NULL FROM generate_series(1,200) i"); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO " + publishTgJobTable + " SELECT i,1,1,'timeout_cancelled',LOCALTIMESTAMP - interval '1 minute' FROM generate_series(201,330) i"); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO " + publishTgJobTable + " VALUES (331,2,2,'timeout_cancelled',LOCALTIMESTAMP - interval '1 minute')"); err != nil {
			return err
		}
		now := time.Now()
		found := false
		for i := 0; i < 14; i++ {
			rows, err := tx.GetAll(telegramTimeoutCleanupClaimSQL, now, now.Add(2*time.Minute))
			if err != nil {
				return err
			}
			if len(rows) > 10 {
				t.Fatalf("claim exceeded batch limit: %d", len(rows))
			}
			for _, row := range rows {
				if row["id"].Int64() <= 200 {
					t.Fatal("completed NULL history was claimed")
				}
				if row["account_id"].Int64() == 2 {
					found = true
				}
			}
		}
		if !found {
			t.Fatal("later account starved behind earlier account")
		}
		rows, err := tx.GetAll(telegramTimeoutCleanupClaimSQL, now, now.Add(2*time.Minute))
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			t.Fatal("leased rows reclaimed before expiry")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
