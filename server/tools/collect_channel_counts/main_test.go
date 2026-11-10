package main

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestHistoryReconciliationLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires fixed local PostgreSQL")
	}
	dsn := "host=127.0.0.1 port=35432 dbname=postgres sslmode=disable user=" + os.Getenv("USER")
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("count_tool_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP SCHEMA " + schema + " CASCADE")
	db, err := sql.Open("postgres", dsn+" search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec := func(q string) {
		t.Helper()
		if _, e := db.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	exec(`CREATE TABLE hg_content_profile(id bigint PRIMARY KEY,deleted_at timestamp);
 CREATE TABLE hg_youban_publish_profile_state(profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp);
 CREATE TABLE hg_youban_publish_collect_channel_profile(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,rule_id bigint,channel_id text,profile_id bigint,deleted_at timestamp,updated_at timestamp DEFAULT now(),UNIQUE(tenant_id,account_id,rule_id,channel_id,profile_id));
 CREATE TABLE hg_youban_publish_collect_event(id bigint,tenant_id bigint,account_id bigint,source_chat_id text,source_id bigint);
 CREATE TABLE hg_youban_publish_collect_dispatch(event_id bigint,tenant_id bigint,account_id bigint,source_id bigint,profile_id bigint,rule_id bigint,source_chat_id_snapshot text);
 CREATE TABLE hg_youban_publish_tg_job(tenant_id bigint,account_id bigint,profile_id bigint,collect_event_id bigint,collect_source_chat_id text);
 CREATE TABLE hg_youban_publish_profile_origin(tenant_id bigint,account_id bigint,profile_id bigint,event_id bigint,origin_kind text,chat_id text);
 INSERT INTO hg_content_profile SELECT n,NULL FROM generate_series(1,9) n;
 INSERT INTO hg_youban_publish_profile_state SELECT n,78,615,NULL FROM generate_series(1,9) n;
 UPDATE hg_youban_publish_profile_state SET account_id=999 WHERE profile_id=5;
 UPDATE hg_content_profile SET deleted_at=now() WHERE id=6;
 INSERT INTO hg_youban_publish_collect_dispatch VALUES
 (1,78,615,0,1,338,'4386299825'),(2,78,615,351,2,338,'other'),(3,78,615,351,3,338,'other'),
 (4,78,615,351,4,339,'4386299825'),(5,78,615,351,5,338,'4386299825'),(6,78,615,351,6,338,'4386299825');
 INSERT INTO hg_youban_publish_tg_job VALUES(78,615,2,2,'-1004386299825'),(78,615,8,999,'4386299825');
 INSERT INTO hg_youban_publish_collect_event VALUES(1,78,615,'4386299825',351);
 INSERT INTO hg_youban_publish_profile_origin VALUES(78,615,3,3,'collect','4386299825'),(78,615,7,999,'collect','4386299825');`)
	migration, err := os.ReadFile("../../addons/youban_publish/resource/sql/collect_channel_counts.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	exec(string(migration))
	exec(`INSERT INTO hg_youban_publish_collect_channel_profile(tenant_id,account_id,source_id,rule_id,channel_id,profile_id) VALUES(78,615,351,338,'4386299825',9)`)
	count := func(want int) {
		t.Helper()
		var got int
		if e := db.QueryRow(`SELECT profile_count FROM hg_youban_publish_collect_channel_count WHERE tenant_id=78 AND account_id=615 AND rule_id=338 AND channel_id='4386299825'`).Scan(&got); e != nil || got != want {
			t.Fatalf("count=%d want=%d err=%v", got, want, e)
		}
	}
	if err = reconcileHistory(db, 78, 615, 338, "-1004386299825", 2, 10, false); err != nil {
		t.Fatal(err)
	}
	count(1)
	if err = reconcileHistory(db, 78, 615, 338, "-1004386299825", 2, 10, true); err != nil {
		t.Fatal(err)
	}
	count(4)
	if err = reconcileHistory(db, 78, 615, 338, "4386299825", 2, 10, true); err != nil {
		t.Fatal(err)
	}
	count(4)
	if err = reconcileHistory(db, 78, 615, 338, "  -1004386299825  ", 2, 10, true); err != nil {
		t.Fatal(err)
	}
	var relations int
	if err = db.QueryRow(`SELECT count(*) FROM hg_youban_publish_collect_channel_profile`).Scan(&relations); err != nil || relations != 4 {
		t.Fatalf("normalization created duplicate relations: %d err=%v", relations, err)
	}
	if err = auditCounts(db, 78, 615, 338, false); err != nil {
		t.Fatal(err)
	}
	if err = auditCounts(db, 0, 0, 0, true); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE hg_youban_publish_collect_channel_count SET profile_count=5 WHERE rule_id=338`)
	if err = auditCounts(db, 78, 615, 338, false); err == nil {
		t.Fatal("audit missed drift")
	}
}
