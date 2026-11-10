package sys

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestDailyReportStageTimeoutLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL")
	}
	ctx := context.Background()
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	node := gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}
	root, err := gdb.New(node)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("daily_report_timeout_%d", time.Now().UnixNano())
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") })
	node.Namespace = schema
	gdb.SetConfigGroup("default", gdb.ConfigGroup{node})
	db := g.DB()
	t.Cleanup(func() { _ = db.GetCore().ClearTableFieldsAll(ctx) })
	for _, sql := range []string{
		`CREATE TABLE hg_youban_publish_daily_report(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,report_date date,notify_status text,notify_retry_count int,notify_next_retry_at timestamp,notify_lease_until timestamp)`,
		`INSERT INTO hg_youban_publish_daily_report(report_date,notify_status,notify_retry_count) VALUES('2020-01-01','sent',0)`,
		`CREATE TABLE hg_youban_publish_account(id bigint,tenant_id bigint,account_type text,status int,created_at timestamp,deleted_at timestamp)`,
		`CREATE TABLE hg_youban_publish_collect_source(tenant_id bigint,account_id bigint,status int,created_at timestamp,deleted_at timestamp)`,
		`CREATE TABLE hg_youban_publish_channel(tenant_id bigint,tg_account_id bigint,cycle_publish_enabled int,created_at timestamp,deleted_at timestamp)`,
		`CREATE TABLE hg_youban_publish_collect_dispatch(tenant_id bigint,account_id bigint,created_at timestamp)`,
		`CREATE TABLE hg_youban_publish_success_record(tenant_id bigint,account_id bigint,job_id bigint,status text,action text,created_at timestamp)`,
		`CREATE TABLE hg_youban_publish_tg_job(id bigint,tenant_id bigint,account_id bigint,sent_at timestamp,updated_at timestamp)`,
	} {
		if _, err = db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	locked := make(chan error, 1)
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- root.Transaction(ctx, func(_ context.Context, tx gdb.TX) error {
			_, err := tx.Exec("LOCK TABLE " + schema + ".hg_youban_publish_account IN ACCESS EXCLUSIVE MODE")
			locked <- err
			if err != nil {
				return err
			}
			<-release
			return nil
		})
	}()
	defer func() {
		close(release)
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	if err = <-locked; err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	err = (&sSysPublish{}).ProcessDailyReports(ctx, time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "读取日报账号失败") || strings.Contains(err.Error(), "清理过期日报失败") {
		t.Fatalf("original generation error lost: %v", err)
	}
	if time.Since(started) > 40*time.Second {
		t.Fatal("generation timeout did not bound execution")
	}
	count, err := db.GetValue(ctx, "SELECT COUNT(*) FROM hg_youban_publish_daily_report")
	if err != nil {
		t.Fatal(err)
	}
	if count.Int() != 0 {
		t.Fatal("generation timeout prevented independent retention cleanup")
	}
}
