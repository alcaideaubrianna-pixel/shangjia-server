package sys

import (
	"context"
	"os"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestBuildDailyReportLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL at 127.0.0.1:35432")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, q := range []string{
			`CREATE TEMP TABLE hg_youban_publish_daily_report(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,report_date date,collected_total int,publish_success int,publish_failed int,publish_processing int,source_breakdown_json text,generated_at timestamp,UNIQUE(tenant_id,account_id,report_date)) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_source(id bigint,tenant_id bigint,title text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch(id bigint,tenant_id bigint,account_id bigint,source_id bigint,source_name_snapshot text,source_chat_id_snapshot text,profile_id bigint,status text,created_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_channel(id bigint,tenant_id bigint,account_id bigint,title text,deleted_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_channel VALUES(1,1,2,'目标频道',NULL)`,
			`CREATE TEMP TABLE hg_youban_publish_success_record(id bigint,tenant_id bigint,account_id bigint,action text,status text,created_at timestamp,job_id bigint DEFAULT 0,channel_id bigint DEFAULT 1) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_job(id bigint,tenant_id bigint,account_id bigint,sent_at timestamp,updated_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_source VALUES (10,1,'来源A',NULL),(11,1,'来源B',NULL)`,
			`INSERT INTO hg_youban_publish_collect_dispatch VALUES (1,1,2,10,'','chat-a',100,'sent','2026-10-24 01:00'),(2,1,2,10,'','chat-a',100,'sent','2026-10-24 01:01'),(3,1,2,10,'','chat-a',101,'sent','2026-10-24 02:00'),(4,1,2,10,'来源A-第二频道','chat-a-2',104,'sent','2026-10-24 02:30'),(5,1,2,11,'来源B','chat-b',102,'sent','2026-10-24 03:00'),(6,1,2,10,'','chat-a',103,'failed','2026-10-24 04:00')`,
			`INSERT INTO hg_youban_publish_collect_dispatch VALUES (7,1,2,11,'改名后的A','chat-a',100,'sent','2026-10-24 05:00'),(8,99,2,10,'其他租户','chat-a',999,'sent','2026-10-24 05:00'),(9,1,999,10,'其他账户','chat-a',998,'sent','2026-10-24 05:00'),(10,1,2,10,'次日','chat-a',997,'sent','2026-10-25 00:00')`,
			`INSERT INTO hg_youban_publish_success_record(id,tenant_id,account_id,action,status,created_at) VALUES (1,1,2,'collect_publish','success','2026-10-24 05:00'),(2,1,2,'cycle_publish','failed','2026-10-24 06:00'),(3,1,2,'collect_publish','pending','2026-10-24 07:00')`,
			`INSERT INTO hg_youban_publish_tg_job VALUES(4,1,2,'2026-10-24 12:00','2026-10-24 12:00'),(5,1,2,'2026-10-25 00:01','2026-10-25 00:01'),(6,1,2,NULL,'2026-10-24 13:00')`,
			`INSERT INTO hg_youban_publish_success_record(id,tenant_id,account_id,action,status,created_at,job_id) VALUES(4,1,2,'collect_publish','success','2026-10-23 23:00',4),(5,1,2,'collect_publish','success','2026-10-24 23:00',5),(6,1,2,'collect_publish','failed','2026-10-23 23:00',6)`,
		} {
			if _, e := tx.Exec(q); e != nil {
				return e
			}
		}
		g.DB().GetCore().ClearTableFieldsAll(ctx)
		report, e := (&sSysPublish{}).BuildDailyReport(ctx, 1, 2, time.Date(2026, 10, 24, 12, 0, 0, 0, time.FixedZone("CST", 8*3600)))
		if e != nil {
			return e
		}
		if report.CollectedTotal != 4 || len(report.Sources) != 3 || report.Sources[0].Count != 2 || report.PublishSuccess != 2 || report.PublishFailed != 1 || report.CycleFailed != 1 || report.PublishProcessing != 1 {
			t.Fatalf("unexpected report: %+v", report)
		}
		if report.Sources[0].SourceChatID != "chat-a" || report.Sources[0].SourceTitle != "改名后的A" {
			t.Fatalf("mutable source title split or outdated: %+v", report.Sources)
		}
		if len(report.Channels) != 2 || report.Channels[0].Title != "目标频道" || report.Channels[0].Success != 2 || report.Channels[1].Failed != 1 {
			t.Fatalf("wrong target breakdown: %+v", report.Channels)
		}
		if e = (&sSysPublish{}).SaveDailyReport(ctx, report); e != nil {
			return e
		}
		// The browsing API only exposes completed days. Persist an equivalent
		// snapshot for yesterday to test round-trip independently of wall date.
		yesterday := time.Now().In(time.FixedZone("CST", 8*3600)).AddDate(0, 0, -1).Format("2006-01-02")
		report.ReportDate = yesterday
		if e = (&sSysPublish{}).SaveDailyReport(ctx, report); e != nil {
			return e
		}
		stored, e := (&sSysPublish{}).DailyReports(ctx, &sysin.DailyReportListInp{TenantID: 1, AccountID: 2, Days: 7})
		if e != nil || len(stored) != 1 || stored[0].Sources[0].Count != 2 {
			t.Fatalf("stored report mismatch: %+v err:%v", stored, e)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDailyReportSevenCompletedDays(t *testing.T) {
	today := time.Date(2026, 11, 1, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))
	if got := dailyReportOldestDate(today, 7); got != "2026-10-25" {
		t.Fatalf("oldest=%s", got)
	}
	if got := dailyReportOldestDate(today, 1); got != "2026-10-31" {
		t.Fatalf("yesterday=%s", got)
	}
}
