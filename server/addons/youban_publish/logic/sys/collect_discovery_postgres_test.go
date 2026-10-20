package sys

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestBotCollectDiscoveryLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:55484")
	}
	if sqlReviewSubprocess(t) {
		return
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "postgres"}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			"CREATE TEMP TABLE hg_youban_publish_collect_rule(id bigint,name text,tenant_id bigint,account_id bigint,status int,deleted_at timestamp)",
			"CREATE TEMP TABLE hg_youban_publish_collect_source_rule(rule_id bigint,tenant_id bigint,source_id bigint,status int)",
			"CREATE TEMP TABLE hg_youban_publish_collect_rule_item(id bigint,rule_id bigint,item_type text,value text,replacement text,sort int)",
			"INSERT INTO hg_youban_publish_collect_rule VALUES(1,'suffix',10,20,1,NULL),(2,'plain',10,20,1,NULL),(3,'other account',10,21,1,NULL),(4,'deleted',10,20,1,NOW()),(5,'disabled',10,20,0,NULL)",
			"INSERT INTO hg_youban_publish_collect_source_rule SELECT id,10,100,1 FROM hg_youban_publish_collect_rule",
			"INSERT INTO hg_youban_publish_collect_rule_item VALUES(1,1,'intro_fee_suffix','configured','',1),(2,3,'intro_fee_suffix','other','',1),(3,4,'intro_fee_suffix','deleted','',1)",
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		rules, err := botCollectChannelDiscoveryRules(ctx, gdb.Record{"tenant_id": g.NewVar(10), "account_id": g.NewVar(20), "id": g.NewVar(100)})
		if err != nil {
			return err
		}
		if len(rules) != 1 || rules[0]["id"].Int64() != 1 || rules[0]["intro_fee_suffix"].String() != "configured" {
			t.Fatalf("unexpected rules: %v", rules)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCollectMediaStatUpsertLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:55484")
	}
	if sqlReviewSubprocess(t) {
		return
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "postgres"}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			"CREATE TEMP TABLE hg_youban_publish_collect_event_media(event_id bigint,cache_status text,download_duration_ms int,download_bytes bigint,download_attempts int,cache_hit int,download_error_type text,error_message text)",
			"CREATE TEMP TABLE hg_youban_publish_collect_media_stat(id bigserial PRIMARY KEY,event_id bigint UNIQUE,tenant_id bigint,account_id bigint,tg_account_id bigint,source_id bigint,status text,media_total int,success_count int,failed_count int,pending_count int,cache_hit_count int,retry_count int,bytes bigint,duration_ms bigint,p50_ms bigint,p95_ms bigint,throughput_mbps float,success_rate float,failure_rate float,error_summary_json text,started_at timestamp,finished_at timestamp,created_at timestamp,updated_at timestamp)",
			"INSERT INTO hg_youban_publish_collect_event_media VALUES(1,'ready',100,1024,1,0,'','')",
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		s := new(sSysPublish)
		event := gdb.Record{"id": g.NewVar(1)}
		s.recordCollectMediaPerformance(ctx, event, time.Now().Add(-time.Second), nil)
		first, err := tx.GetOne("SELECT id,success_count,created_at FROM hg_youban_publish_collect_media_stat WHERE event_id=1")
		if err != nil {
			return err
		}
		if first.IsEmpty() || first["success_count"].Int() != 1 || first["created_at"].IsNil() {
			t.Fatalf("initial insert failed: %v", first)
		}
		if _, err = tx.Exec("UPDATE hg_youban_publish_collect_event_media SET cache_status='failed',error_message='timeout'"); err != nil {
			return err
		}
		s.recordCollectMediaPerformance(ctx, event, time.Now().Add(-time.Second), nil)
		rows, err := tx.GetAll("SELECT id,failed_count,created_at FROM hg_youban_publish_collect_media_stat WHERE event_id=1")
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0]["id"].Int64() != first["id"].Int64() || rows[0]["created_at"].String() != first["created_at"].String() || rows[0]["failed_count"].Int() != 1 {
			t.Fatalf("upsert changed identity or lost update: %v", rows)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// g.DB caches its first driver. Isolate tests that replace the default group
// so subsequent SQL-generation tests retain their original driver/configuration.
func sqlReviewSubprocess(t *testing.T) bool {
	t.Helper()
	if os.Getenv("YOUBAN_SQL_REVIEW_CHILD") == "1" {
		return false
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.count=1")
	cmd.Env = append(os.Environ(), "YOUBAN_SQL_REVIEW_CHILD=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("isolated PostgreSQL test: %v\n%s", err, output)
	}
	return true
}
