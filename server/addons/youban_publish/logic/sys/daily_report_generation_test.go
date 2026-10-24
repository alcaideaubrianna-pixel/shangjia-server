package sys

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	botService "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestDailyReportGenerationLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	var previous botService.ISysBot
	func() { defer func() { _ = recover() }(); previous = botService.SysBot() }()
	fake := &dailyReportTestBot{ISysBot: previous}
	botService.RegisterSysBot(fake)
	t.Cleanup(func() { botService.RegisterSysBot(previous) })
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_youban_publish_daily_report(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,report_date date,collected_total int DEFAULT 0,publish_success int DEFAULT 0,publish_failed int DEFAULT 0,publish_processing int DEFAULT 0,source_breakdown_json text,generated_at timestamp,notify_status text DEFAULT 'pending',notify_retry_count int DEFAULT 0,notify_next_retry_at timestamp,notify_lease_until timestamp,notify_token text DEFAULT '',notify_cursor int DEFAULT 0,notify_error text DEFAULT '',UNIQUE(tenant_id,account_id,report_date)) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account(id bigint,tenant_id bigint,account_type text,status int,created_at timestamp,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account_setting(id bigint,tenant_id bigint,account_id bigint,daily_report_enabled int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_source(id bigint,tenant_id bigint,account_id bigint,title text,status int,created_at timestamp,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_channel(id bigint,tenant_id bigint,account_id bigint,title text,cycle_publish_enabled int,created_at timestamp,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch(id bigint,tenant_id bigint,account_id bigint,source_id bigint,source_name_snapshot text,source_chat_id_snapshot text,profile_id bigint,status text,created_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_success_record(id bigint,tenant_id bigint,account_id bigint,action text,status text,created_at timestamp,job_id bigint DEFAULT 0,channel_id bigint) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_job(id bigint,tenant_id bigint,account_id bigint,sent_at timestamp,updated_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_account VALUES(2,1,'admin',1,'2026-10-17',NULL),(3,1,'uploader',1,'2026-10-17',NULL),(4,1,'admin',2,'2026-10-17',NULL),(5,1,'admin',1,'2026-10-17',NULL)`,
			`INSERT INTO hg_youban_publish_collect_source VALUES(1,1,2,'启用的空采集源',1,'2026-10-17',NULL),(2,1,3,'子账户源',1,'2026-10-17',NULL),(3,1,4,'已禁用账户源',1,'2026-10-17',NULL),(4,1,5,'已删源',1,'2026-10-17','2026-10-18'),(5,99,5,'其他租户源',1,'2026-10-17',NULL)`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		now := time.Date(2026, 10, 23, 16, 0, 0, 0, time.UTC)
		s := &sSysPublish{}
		count, err := s.generateDailyReportBatch(ctx, now.Add(-time.Second), 1)
		if err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("zero-activity configured administrator omitted: %d", count)
		}
		count, err = s.generateDailyReportBatch(ctx, now.Add(-time.Second), 1)
		if err != nil {
			return err
		}
		if count != 0 {
			t.Fatal("existing report regenerated")
		}
		row, err := tx.GetOne(`SELECT source_breakdown_json FROM hg_youban_publish_daily_report`)
		if err != nil {
			return err
		}
		var report sysin.DailyReportModel
		if err = json.Unmarshal([]byte(row["source_breakdown_json"].String()), &report); err != nil {
			return err
		}
		if report.ReportDate != "2026-10-23" || report.CollectedTotal != 0 {
			t.Fatal("incorrect China completed day", report)
		}
		original := row["source_breakdown_json"].String()
		report.CollectedTotal = 999
		if err = s.SaveDailyReport(ctx, &report); err != nil {
			return err
		}
		row, err = tx.GetOne(`SELECT source_breakdown_json FROM hg_youban_publish_daily_report`)
		if err != nil {
			return err
		}
		if row["source_breakdown_json"].String() != original {
			t.Fatal("published snapshot was overwritten")
		}
		if _, err = tx.Exec(`INSERT INTO hg_youban_publish_daily_report(tenant_id,account_id,report_date,source_breakdown_json)VALUES(1,2,'2026-10-16','{}')`); err != nil {
			return err
		}
		if err = s.ProcessDailyReports(ctx, now); err != nil {
			return err
		}
		// ProcessDailyReports owns a timeout in production. Restore this test-only
		// outer transaction context after that timeout is canceled.
		tx = tx.Ctx(ctx)
		rows, err := tx.GetAll(`SELECT report_date,notify_status FROM hg_youban_publish_daily_report ORDER BY report_date`)
		if err != nil {
			return err
		}
		if len(rows) != 7 || fake.calls != 7 {
			t.Fatalf("restart compensation or retention failed: reports=%d sends=%d", len(rows), fake.calls)
		}
		for _, row := range rows {
			if row["notify_status"].String() != "sent" {
				t.Fatal("zero-activity report not sent", row)
			}
		}
		if err = s.ProcessDailyReports(ctx, now.Add(time.Minute)); err != nil {
			return err
		}
		if fake.calls != 7 {
			t.Fatal("minute poll resent completed reports")
		}
		tx = tx.Ctx(ctx)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
