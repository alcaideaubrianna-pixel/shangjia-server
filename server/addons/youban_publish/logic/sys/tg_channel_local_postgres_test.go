package sys

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
)

func TestTelegramChannelHeadsLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_HEAD_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:35432")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	data, err := os.ReadFile(os.Getenv("YOUBAN_CHANNEL_HEAD_SNAPSHOT"))
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]string
	if err = json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// Temporary tables shadow production names only in this disposable
		// local transaction. No real worker or external service is started.
		ddl := []string{
			"CREATE TEMP TABLE " + publishTgJobTable + " (id bigint,tenant_id bigint,channel_id bigint,target_chat_id text,status text,dispatch_status text,next_retry_at timestamp,created_at timestamp,send_phase text)",
			"CREATE TEMP TABLE " + publishTgOutboxTable + " (id bigint,job_id bigint,attempt_count int,available_at timestamp,event_type text,status text,lease_expires_at timestamp)",
			"CREATE TEMP TABLE " + publishChannelTable + " (id bigint,tenant_id bigint,tg_account_id bigint,status int,deleted_at timestamp)",
			"CREATE TEMP TABLE " + publishTgAccountTable + " (id bigint,tenant_id bigint,status text,session_key text,deleted_at timestamp)",
			"CREATE TEMP TABLE " + publishTgChannelTable + " (id bigint,tenant_id bigint,tg_account_id bigint)",
		}
		for _, sql := range ddl {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("INSERT INTO " + publishTgAccountTable + " VALUES(1,1,'authorized','local-fixture',NULL)"); err != nil {
			return err
		}
		want := map[string]bool{}
		for _, row := range rows {
			id, channel := row["job_id"], row["channel_id"]
			var retry interface{}
			if row["next_retry_at"] != "NULL" {
				retry = row["next_retry_at"]
			}
			if _, err := tx.Exec("INSERT INTO "+publishChannelTable+" VALUES(?,1,1,1,NULL)", channel); err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT INTO "+publishTgJobTable+" VALUES(?,1,?,'',?,?,?,'2026-10-15',?)", id, channel, row["status"], row["dispatch_status"], retry, row["send_phase"]); err != nil {
				return err
			}
			// Older terminal display must not hide this channel's active head.
			if _, err := tx.Exec("INSERT INTO "+publishTgJobTable+" VALUES(-?::bigint,1,?,'','failed','done',NULL,'2026-10-14','display_confirmed')", id, channel); err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT INTO "+publishTgOutboxTable+" VALUES(?,?,0,?, ?,?,NULL)", id, id, row["available_at"], telegramOutboxEventPublish, row["outbox_status"]); err != nil {
				return err
			}
			want[id] = (row["status"] == "pending" || row["status"] == "failed_retry") && row["dispatch_status"] == "idle" && (retry == nil || row["next_retry_at"] <= row["now"]) && row["outbox_status"] == "pending" && row["available_at"] <= row["now"]
		}
		query := func() (gdb.Result, error) {
			return tx.GetAll(telegramDueOutboxPostgresSQL(), rows[0]["now"], telegramOutboxEventPublish, rows[0]["now"], telegramOutboxPending, telegramOutboxProcessing, rows[0]["now"], 100)
		}
		assertHeads := func() error {
			got, err := query()
			if err != nil {
				return err
			}
			seen := map[string]bool{}
			for _, r := range got {
				seen[r["job_id"].String()] = true
			}
			for id, expected := range want {
				if seen[id] != expected {
					t.Errorf("job %s selected=%t want=%t", id, seen[id], expected)
				}
			}
			t.Logf("real PostgreSQL channel heads: fixtures=%d selected=%d", len(rows), len(got))
			return nil
		}
		if err := assertHeads(); err != nil {
			return err
		}
		// 100k later jobs must neither hide heads nor change eligibility.
		if _, err := tx.Exec("INSERT INTO " + publishTgJobTable + " SELECT 3000000+n,1,42,'','pending','idle',NULL,'2026-10-16 16:00:00','' FROM generate_series(1,100000) n"); err != nil {
			return err
		}
		started := time.Now()
		if err := assertHeads(); err != nil {
			return err
		}
		t.Logf("100000 backlog query elapsed=%s", time.Since(started))
		// Execute the exact GoFrame chained WHERE + LIMIT shape that previously
		// produced the production syntax error, not just a string balance check.
		_, err = tx.Model(publishTgJobTable+" j").Ctx(ctx).Where("j.id <> ?", 2078782).
			Where(telegramChannelBlockingJobSQL("j")).Where("j.channel_id", 228).
			Where("(j.created_at < ? OR (j.created_at = ? AND j.id < ?))", "2026-10-16", "2026-10-16", 2078782).
			Fields("j.id").Limit(1).One()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
