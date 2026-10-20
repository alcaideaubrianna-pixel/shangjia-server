package sys

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
)

func TestRepairConfirmedVerifyPurposeLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COMMIT_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:35432")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres", MaxOpenConnCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, ddl := range []string{
			"CREATE TEMP TABLE " + publishTgJobTable + " (id bigint,tenant_id bigint,profile_id bigint,status text,send_phase text) ON COMMIT DROP",
			"CREATE TEMP TABLE " + publishTgAttemptTable + " (id bigint,job_id bigint,target_chat_id text,phase text,status text,response_received int,expected_count int,confirmed_count int) ON COMMIT DROP",
			"CREATE TEMP TABLE " + publishTgAttemptMessageTable + " (attempt_id bigint,job_id bigint,target_chat_id text,message_id bigint,source text) ON COMMIT DROP",
			"CREATE TEMP TABLE " + publishTgMessageTable + " (job_id bigint,tenant_id bigint,profile_id bigint,target_chat_id text,tg_message_id bigint,media_id bigint,purpose text,status text,deleted_at timestamp,updated_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE " + publishMediaTable + " (id bigint,profile_id bigint,purpose text) ON COMMIT DROP",
		} {
			if _, err := tx.Exec(ddl); err != nil {
				return err
			}
		}
		for _, test := range []struct {
			name, change string
			want         int64
		}{
			{"confirmed response", "", 1},
			{"unknown attempt", "UPDATE " + publishTgAttemptTable + " SET status='waiting'", 0},
			{"no explicit response", "UPDATE " + publishTgAttemptTable + " SET response_received=0", 0},
			{"incomplete album", "UPDATE " + publishTgAttemptTable + " SET expected_count=2,confirmed_count=2", 0},
			{"deleted message", "UPDATE " + publishTgMessageTable + " SET deleted_at=NOW()", 0},
			{"other chat", "UPDATE " + publishTgMessageTable + " SET target_chat_id='other'", 0},
			{"other tenant", "UPDATE " + publishTgMessageTable + " SET tenant_id=99", 0},
			{"display media", "UPDATE " + publishMediaTable + " SET purpose='display'", 0},
			{"cancelled job", "UPDATE " + publishTgJobTable + " SET send_phase='timeout_cancelled'", 0},
			{"webhook only", "UPDATE " + publishTgAttemptMessageTable + " SET source='webhook'", 0},
			{"conflicting display audit", "INSERT INTO " + publishTgAttemptTable + " VALUES (2,1,'chat','display','confirmed',1,1,1); INSERT INTO " + publishTgAttemptMessageTable + " VALUES (2,1,'chat',45316,'response')", 0},
		} {
			t.Run(test.name, func(t *testing.T) {
				for _, table := range []string{publishTgJobTable, publishTgAttemptTable, publishTgAttemptMessageTable, publishTgMessageTable, publishMediaTable} {
					if _, err := tx.Exec("DELETE FROM " + table); err != nil {
						t.Fatal(err)
					}
				}
				for _, sql := range []string{
					"INSERT INTO " + publishTgJobTable + " VALUES (1,16,654547,'sending','verify_confirmed')",
					"INSERT INTO " + publishTgAttemptTable + " VALUES (1,1,'chat','verify','confirmed',1,1,1)",
					"INSERT INTO " + publishTgAttemptMessageTable + " VALUES (1,1,'chat',45316,'response')",
					"INSERT INTO " + publishTgMessageTable + " VALUES (1,16,654547,'chat',45316,4996314,'display','sent',NULL,NOW())",
					"INSERT INTO " + publishMediaTable + " VALUES (4996314,654547,'verify')",
				} {
					if _, err := tx.Exec(sql); err != nil {
						t.Fatal(err)
					}
				}
				if test.change != "" {
					if _, err := tx.Exec(test.change); err != nil {
						t.Fatal(err)
					}
				}
				got, err := repairConfirmedTelegramVerifyPurposeTx(ctx, tx, 1)
				if err != nil || got != test.want {
					t.Fatalf("repaired=%d want=%d err=%v", got, test.want, err)
				}
				if got, err = repairConfirmedTelegramVerifyPurposeTx(ctx, tx, 1); err != nil || got != 0 {
					t.Fatalf("not idempotent: repaired=%d err=%v", got, err)
				}
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
