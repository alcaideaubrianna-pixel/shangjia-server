package sys

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
)

func TestTelegramObserveAggregateBeforeJoinPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires disposable localhost:55484 PostgreSQL")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, q := range []string{
			"CREATE TEMP TABLE " + publishTgJobTable + "(tenant_id bigint,account_id bigint,channel_id bigint,target_chat_id text,bot_id bigint,status text,dispatch_status text,error_message text,sent_at timestamp,updated_at timestamp,queue_name text,priority int,created_at timestamp)",
			"CREATE TEMP TABLE " + publishChannelTable + "(id bigint PRIMARY KEY,channel_title text)",
			"CREATE TEMP TABLE " + publishBotTable + "(id bigint PRIMARY KEY,bot_name text,bot_username text)",
			"INSERT INTO " + publishChannelTable + " VALUES (1,'频道'),(2,NULL)",
			"INSERT INTO " + publishBotTable + " VALUES (1,'bot','username'),(2,NULL,NULL)",
			"INSERT INTO " + publishTgJobTable + " SELECT n%3,n%5,n%4,CASE WHEN n%7=0 THEN NULL ELSE (n%3)::text END,n%4,(ARRAY['pending','failed_retry','sending','sent','failed','superseded',NULL])[1+n%7],(ARRAY['idle','queued','',NULL])[1+n%4],(ARRAY['限流','Too Many Requests','error','',NULL])[1+n%5],CASE WHEN n%2=0 THEN timestamp '2026-01-01' END,timestamp '2026-01-02'+n*interval '1 second',(ARRAY['','youban_publish_tg','other',NULL])[1+n%4],n%3,timestamp '2026-01-01' FROM generate_series(1,1000) n",
		} {
			if _, err := tx.Exec(q); err != nil {
				return err
			}
		}
		{
			oldQuery := `SELECT 
j.tenant_id,j.account_id,j.channel_id,j.target_chat_id,MAX(c.channel_title) AS channel_title,
SUM(CASE WHEN j.status IN ('pending','failed_retry') AND (j.dispatch_status='' OR j.dispatch_status='idle') THEN 1 ELSE 0 END) AS pending_count,
SUM(CASE WHEN j.dispatch_status='queued' THEN 1 ELSE 0 END) AS queued_count,
SUM(CASE WHEN j.status='sending' THEN 1 ELSE 0 END) AS sending_count,
SUM(CASE WHEN j.status='sent' THEN 1 ELSE 0 END) AS sent_count,
SUM(CASE WHEN j.status='failed' THEN 1 ELSE 0 END) AS failed_count,
SUM(CASE WHEN j.status='failed_retry' THEN 1 ELSE 0 END) AS retry_count,
SUM(CASE WHEN j.error_message LIKE '%限流%' OR j.error_message LIKE '%Too Many Requests%' THEN 1 ELSE 0 END) AS rate_limit_count,
MAX(j.sent_at) AS last_sent_at,
MAX(CASE WHEN j.status IN ('failed','failed_retry') THEN j.updated_at ELSE NULL END) AS last_error_at,
MAX(j.error_message) AS last_error_message FROM ` + publishTgJobTable + ` j LEFT JOIN ` + publishChannelTable + ` c ON c.id=j.channel_id GROUP BY j.tenant_id,j.account_id,j.channel_id,j.target_chat_id`
			newQuery := telegramObserveChannelStatsSQL()
			if err := assertSharedTelegramStatsEqual(t, tx, oldQuery, "channel"); err != nil {
				return err
			}
			diff, err := tx.GetValue(`SELECT count(*) FROM ((SELECT to_jsonb(x) v FROM (` + oldQuery + `) x EXCEPT ALL SELECT to_jsonb(x) FROM (` + newQuery + `) x) UNION ALL (SELECT to_jsonb(x) FROM (` + newQuery + `) x EXCEPT ALL SELECT to_jsonb(x) FROM (` + oldQuery + `) x)) d`)
			if err != nil {
				return err
			}
			if diff.Int() != 0 {
				t.Fatalf("Channel statistics changed: %d rows", diff.Int())
			}
		}
		{
			oldQuery := `SELECT 
j.tenant_id,j.bot_id,MAX(b.bot_name) AS bot_name,MAX(b.bot_username) AS bot_username,
SUM(CASE WHEN j.status IN ('pending','failed_retry') AND (j.dispatch_status='' OR j.dispatch_status='idle') THEN 1 ELSE 0 END) AS pending_count,
SUM(CASE WHEN j.dispatch_status='queued' THEN 1 ELSE 0 END) AS queued_count,
SUM(CASE WHEN j.status='sending' THEN 1 ELSE 0 END) AS sending_count,
SUM(CASE WHEN j.status='sent' THEN 1 ELSE 0 END) AS sent_count,
SUM(CASE WHEN j.status='failed' THEN 1 ELSE 0 END) AS failed_count,
SUM(CASE WHEN j.status='failed_retry' THEN 1 ELSE 0 END) AS retry_count,
SUM(CASE WHEN j.error_message LIKE '%限流%' OR j.error_message LIKE '%Too Many Requests%' THEN 1 ELSE 0 END) AS rate_limit_count,
MAX(j.sent_at) AS last_sent_at,
MAX(CASE WHEN j.status IN ('failed','failed_retry') THEN j.updated_at ELSE NULL END) AS last_error_at,
MAX(j.error_message) AS last_error_message FROM ` + publishTgJobTable + ` j LEFT JOIN ` + publishBotTable + ` b ON b.id=j.bot_id GROUP BY j.tenant_id,j.bot_id`
			newQuery := telegramObserveBotStatsSQL()
			if err := assertSharedTelegramStatsEqual(t, tx, oldQuery, "bot"); err != nil {
				return err
			}
			diff, err := tx.GetValue(`SELECT count(*) FROM ((SELECT to_jsonb(x) v FROM (` + oldQuery + `) x EXCEPT ALL SELECT to_jsonb(x) FROM (` + newQuery + `) x) UNION ALL (SELECT to_jsonb(x) FROM (` + newQuery + `) x EXCEPT ALL SELECT to_jsonb(x) FROM (` + oldQuery + `) x)) d`)
			if err != nil {
				return err
			}
			if diff.Int() != 0 {
				t.Fatalf("Bot statistics changed: %d rows", diff.Int())
			}
		}
		if err := assertSharedTelegramStatsEqual(t, tx, `SELECT COALESCE(NULLIF(queue_name,''),'youban_publish_tg') AS queue_name,priority AS priority_level,status,COUNT(1) AS job_count,MIN(created_at) AS oldest_job_at,MAX(created_at) AS latest_job_at FROM `+publishTgJobTable+` GROUP BY COALESCE(NULLIF(queue_name,''),'youban_publish_tg'),priority,status`, "queue"); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertSharedTelegramStatsEqual(t *testing.T, tx gdb.TX, oldQuery, kind string) error {
	t.Helper()
	oldRows, err := tx.GetAll(oldQuery)
	if err != nil {
		return err
	}
	newRows, err := tx.GetAll(telegramObserveStatsPostgresSQL())
	if err != nil {
		return err
	}
	var matched int
	for _, row := range newRows {
		if row["stat_kind"].String() != kind {
			continue
		}
		matched++
		found := false
		for _, old := range oldRows {
			equal := true
			for key, value := range old {
				if value.IsNil() != row[key].IsNil() || value.String() != row[key].String() {
					equal = false
					break
				}
			}
			if equal {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s combined aggregation changed a row", kind)
		}
	}
	if matched != len(oldRows) {
		t.Fatalf("%s groups changed: old=%d new=%d", kind, len(oldRows), matched)
	}
	return nil
}

func TestTelegramObserveRefreshLeasePostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires disposable localhost:55484 PostgreSQL")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(_ context.Context, first gdb.TX) error {
		if _, err := first.GetValue("SELECT pg_advisory_xact_lock(hashtextextended(?,0))", "youban_publish:telegram-observe-stats"); err != nil {
			return err
		}
		// Use the parent context to acquire a different connection, not a nested savepoint.
		return db.Transaction(ctx, func(inner context.Context, second gdb.TX) error {
			return new(sSysPublish).refreshTelegramObserveStatsPostgres(inner, second)
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(ctx, func(_ context.Context, tx gdb.TX) error {
		locked, err := tx.GetValue("SELECT pg_try_advisory_xact_lock(hashtextextended(?,0))", "youban_publish:telegram-observe-stats")
		if err == nil && !locked.Bool() {
			t.Fatal("refresh lease was not released at transaction end")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
