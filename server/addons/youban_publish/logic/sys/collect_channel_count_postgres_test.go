package sys

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// Uses a disposable schema on a fixed localhost server, never a production DSN.
func TestCollectChannelCountPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires local PostgreSQL 127.0.0.1:35432")
	}
	ctx := context.Background()
	dsn := "host=127.0.0.1 port=35432 dbname=postgres sslmode=disable user=" + os.Getenv("USER")
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("collect_count_test_%d", time.Now().UnixNano())
	if _, err = admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer admin.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE")
	db, err := sql.Open("postgres", dsn+" search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(8)
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatalf("SQL failed: %v\n%s", err, query)
		}
	}
	exec(`CREATE TABLE hg_content_profile(id bigint PRIMARY KEY,deleted_at timestamp);
 CREATE TABLE hg_youban_publish_profile_state(profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp);
 CREATE TABLE hg_youban_publish_collect_channel_profile(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,rule_id bigint,channel_id text,profile_id bigint,deleted_at timestamp,updated_at timestamp DEFAULT '2026-11-10 05:23:16',UNIQUE(tenant_id,account_id,rule_id,channel_id,profile_id));
 INSERT INTO hg_content_profile SELECT n,NULL FROM generate_series(1,1000) n;
 INSERT INTO hg_youban_publish_profile_state SELECT n,78,615,NULL FROM generate_series(1,1000) n;
 INSERT INTO hg_youban_publish_collect_channel_profile(tenant_id,account_id,source_id,rule_id,channel_id,profile_id)
 SELECT 78,615,351,338,'4386299825',n FROM generate_series(1,70) n;
 INSERT INTO hg_youban_publish_collect_channel_profile(tenant_id,account_id,source_id,rule_id,channel_id,profile_id) VALUES
 (78,615,351,338,'-1004386299825',1),(78,615,351,339,'4386299825',1),
 (78,615,351,338,'4386299825',71),(78,615,351,338,'4386299825',72);
 UPDATE hg_content_profile SET deleted_at=now() WHERE id=71;
 UPDATE hg_youban_publish_profile_state SET account_id=999 WHERE profile_id=72;
 INSERT INTO hg_youban_publish_collect_channel_profile(tenant_id,account_id,source_id,rule_id,channel_id,profile_id)
 SELECT 78,615,351,338,('9'||channel::text),n FROM generate_series(1,20) channel CROSS JOIN generate_series(1,1000) n;`)
	migration, err := os.ReadFile("../../resource/sql/collect_channel_counts.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	exec(string(migration))
	// Existing rows are explicitly uninitialized until the bounded backfill runs.
	var pending int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM hg_youban_publish_collect_channel_profile WHERE NOT count_projected`).Scan(&pending); err != nil || pending != 20074 {
		t.Fatalf("pending=%d err=%v", pending, err)
	}
	backfill := `WITH settings AS MATERIALIZED (SELECT set_config('youban.collect_count_backfill','on',true)), candidates AS (SELECT c.id FROM hg_youban_publish_collect_channel_profile c CROSS JOIN settings WHERE NOT count_projected ORDER BY tenant_id,account_id,ybp_collect_count_chat(channel_id),rule_id,id LIMIT 200 FOR UPDATE OF c SKIP LOCKED)
 UPDATE hg_youban_publish_collect_channel_profile c SET count_projected=true FROM candidates x WHERE c.id=x.id`
	var wg sync.WaitGroup
	failures := make(chan error, 16)
	started := time.Now()
	for worker := 0; worker < 2; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				result, e := db.ExecContext(ctx, backfill)
				if e != nil {
					failures <- e
					return
				}
				n, _ := result.RowsAffected()
				if n == 0 {
					return
				}
			}
		}()
	}
	// Concurrent normal writes and alias duplicates during the initial backfill.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := db.ExecContext(ctx, `INSERT INTO hg_youban_publish_collect_channel_profile(tenant_id,account_id,source_id,rule_id,channel_id,profile_id) VALUES(78,615,351,338,'4386299825',74) ON CONFLICT(tenant_id,account_id,rule_id,channel_id,profile_id) DO UPDATE SET source_id=EXCLUDED.source_id`)
			if e != nil {
				failures <- e
			}
		}()
	}
	wg.Wait()
	close(failures)
	for e := range failures {
		t.Fatal(e)
	}
	t.Logf("backfilled 20,074 rows with concurrent writes in %s", time.Since(started))
	assert := func() {
		t.Helper()
		rows, e := db.QueryContext(ctx, `SELECT s.rule_id,s.channel_id,s.profile_count,s.ready,
    (SELECT count(DISTINCT c.profile_id) FROM hg_youban_publish_collect_channel_profile c
     WHERE c.tenant_id=s.tenant_id AND c.account_id=s.account_id AND (s.rule_id=0 OR c.rule_id=s.rule_id)
      AND ybp_collect_count_chat(c.channel_id)=s.channel_id AND c.deleted_at IS NULL
      AND EXISTS(SELECT 1 FROM hg_content_profile p JOIN hg_youban_publish_profile_state ps ON ps.profile_id=p.id
       WHERE p.id=c.profile_id AND p.deleted_at IS NULL AND ps.tenant_id=c.tenant_id AND ps.account_id=c.account_id AND ps.deleted_at IS NULL)) expected
   FROM hg_youban_publish_collect_channel_count s`)
		if e != nil {
			t.Fatal(e)
		}
		defer rows.Close()
		for rows.Next() {
			var rule, n, want int64
			var chat string
			var ready bool
			if e = rows.Scan(&rule, &chat, &n, &ready, &want); e != nil {
				t.Fatal(e)
			}
			if n != want || !ready {
				t.Fatalf("rule=%d chat=%s count=%d expected=%d ready=%t", rule, chat, n, want, ready)
			}
		}
		if e = rows.Err(); e != nil {
			t.Fatal(e)
		}
	}
	exec("SELECT ybp_collect_count_finish_backfill()")
	assert()
	var count int
	if err = db.QueryRowContext(ctx, `SELECT profile_count FROM hg_youban_publish_collect_channel_count WHERE tenant_id=78 AND account_id=615 AND rule_id=338 AND channel_id='4386299825'`).Scan(&count); err != nil || count != 71 {
		t.Fatalf("online fixture count=%d err=%v", count, err)
	}
	// Migration and backfill are restartable and idempotent.
	exec(string(migration))
	exec(backfill)
	assert()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO hg_youban_publish_collect_channel_profile(tenant_id,account_id,source_id,rule_id,channel_id,profile_id) VALUES(78,615,351,338,'4386299825',75)`); err != nil {
		t.Fatal(err)
	}
	tx.Rollback()
	assert()
	exec(`UPDATE hg_content_profile SET deleted_at=now() WHERE id=1`)
	assert()
	exec(`UPDATE hg_content_profile SET deleted_at=NULL WHERE id=1`)
	assert()
	exec(`UPDATE hg_youban_publish_collect_channel_profile SET deleted_at=now() WHERE profile_id=2`)
	assert()
	exec(`UPDATE hg_youban_publish_collect_channel_profile SET deleted_at=NULL WHERE profile_id=2`)
	assert()
	exec(`DELETE FROM hg_content_profile WHERE id=3`)
	assert()
	exec(`DELETE FROM hg_youban_publish_collect_channel_profile WHERE profile_id=3`)
	assert()
	exec(`UPDATE hg_youban_publish_profile_state SET deleted_at=now() WHERE profile_id=4`)
	assert()
	exec(`UPDATE hg_youban_publish_profile_state SET deleted_at=NULL WHERE profile_id=4`)
	assert()
	exec(`UPDATE hg_youban_publish_profile_state SET account_id=999 WHERE profile_id=5`)
	assert()
	exec(`UPDATE hg_youban_publish_profile_state SET account_id=615 WHERE profile_id=5`)
	assert()
	exec(`INSERT INTO hg_youban_publish_profile_state VALUES(6,79,616,NULL);
 INSERT INTO hg_youban_publish_collect_channel_profile(tenant_id,account_id,source_id,rule_id,channel_id,profile_id) VALUES(79,616,352,338,'4386299825',6)`)
	assert()
	// The hot read touches summary rows only, regardless of the 20k relations.
	started = time.Now()
	rows, err := db.QueryContext(ctx, `SELECT channel_id,profile_count FROM hg_youban_publish_collect_channel_count WHERE tenant_id=78 AND account_id=615 AND rule_id=338 ORDER BY channel_id LIMIT 20`)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for rows.Next() {
		n++
	}
	rows.Close()
	t.Logf("20-channel summary read rows=%d elapsed=%s", n, time.Since(started))
	if n != 20 {
		t.Fatal("summary page missing rows")
	}
	var plan string
	if err = db.QueryRowContext(ctx, `EXPLAIN (FORMAT JSON) SELECT profile_count FROM hg_youban_publish_collect_channel_count WHERE tenant_id=78 AND account_id=615 AND rule_id=338 AND channel_id='4386299825'`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plan, "hg_youban_publish_tg_job") || strings.Contains(plan, "hg_content_profile") {
		t.Fatal("summary query reads history")
	}
}
