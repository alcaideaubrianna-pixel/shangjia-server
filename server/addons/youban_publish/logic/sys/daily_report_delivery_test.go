package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botService "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/library/dailyreport"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type dailyReportTestBot struct {
	botService.ISysBot
	calls  int
	failAt int
	texts  []string
}

func (b *dailyReportTestBot) NotifyAccount(ctx context.Context, in *botsysin.NotifyAccountInp) error {
	b.calls++
	if in.App != "api" || in.AccountId != 2 || !in.RequireDelivery || in.ParseMode != "HTML" {
		return fmt.Errorf("invalid notification: %+v", in)
	}
	if b.calls == b.failAt {
		return fmt.Errorf("injected Telegram failure")
	}
	b.texts = append(b.texts, in.Text)
	return nil
}

func TestDailyReportDeliveryLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	var previous botService.ISysBot
	func() { defer func() { _ = recover() }(); previous = botService.SysBot() }()
	fake := &dailyReportTestBot{ISysBot: previous, failAt: 2}
	botService.RegisterSysBot(fake)
	t.Cleanup(func() { botService.RegisterSysBot(previous) })
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_youban_publish_daily_report(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,report_date date,source_breakdown_json text,notify_status text DEFAULT 'pending',notify_retry_count int DEFAULT 0,notify_next_retry_at timestamp,notify_lease_until timestamp,notify_token text DEFAULT '',notify_cursor int DEFAULT 0,notify_error varchar(512) DEFAULT '') ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account(id bigint,tenant_id bigint,account_type text,status int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account_setting(id bigint,tenant_id bigint,account_id bigint,daily_report_enabled int,deleted_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_account VALUES(2,1,'admin',1,NULL)`,
			`INSERT INTO hg_youban_publish_account_setting VALUES(1,1,2,1,NULL)`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))
		report := &sysin.DailyReportModel{TenantID: 1, AccountID: 2, ReportDate: "2026-10-23", CollectedTotal: 40}
		for i := 0; i < 40; i++ {
			report.Sources = append(report.Sources, &sysin.DailyReportSourceRow{SourceTitle: strings.Repeat("源", 190) + fmt.Sprint(i), Count: 1})
		}
		raw, _ := json.Marshal(report)
		expected := dailyreport.Messages(report)
		if len(expected) < 2 {
			t.Fatal("fixture must span multiple messages")
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_daily_report(tenant_id,account_id,report_date,source_breakdown_json)VALUES(1,2,'2026-10-23',?)`, string(raw)); err != nil {
			return err
		}
		s := &sSysPublish{}
		if err := s.deliverDailyReports(ctx, now, 50); err == nil {
			t.Fatal("send failure swallowed")
		}
		row, err := tx.GetOne(`SELECT * FROM hg_youban_publish_daily_report WHERE id=1`)
		if err != nil {
			return err
		}
		if row["notify_status"].String() != "failed" || row["notify_cursor"].Int() != 1 || row["notify_retry_count"].Int() != 1 {
			t.Fatal("failed progress not saved", row)
		}
		if err = s.deliverDailyReports(ctx, now, 50); err != nil {
			return err
		}
		if fake.calls != 2 {
			t.Fatal("retry ignored backoff")
		}
		if err = s.deliverDailyReports(ctx, now.Add(6*time.Minute), 50); err != nil {
			return err
		}
		if len(fake.texts) != len(expected) {
			t.Fatalf("lost or duplicated chunks: %d vs %d", len(fake.texts), len(expected))
		}
		for i, text := range expected {
			if fake.texts[i] != text {
				t.Fatalf("chunk %d mismatched", i)
			}
		}
		if err = s.deliverDailyReports(ctx, now.Add(7*time.Minute), 50); err != nil {
			return err
		}
		if fake.calls != len(expected)+1 {
			t.Fatal("completed report sent again")
		}
		// Recover an expired lease, while a live lease remains untouched.
		if _, err = tx.Exec(`UPDATE hg_youban_publish_daily_report SET notify_status='sending',notify_cursor=1,notify_token='old',notify_lease_until=?`, now.Add(-time.Minute)); err != nil {
			return err
		}
		if err = s.deliverDailyReports(ctx, now, 50); err != nil {
			return err
		}
		row, err = tx.GetOne(`SELECT notify_status,notify_token FROM hg_youban_publish_daily_report WHERE id=1`)
		if err != nil {
			return err
		}
		if row["notify_status"].String() != "sent" || row["notify_token"].String() == "old" {
			t.Fatal("expired lease not recovered", row)
		}
		calls := fake.calls
		if _, err = tx.Exec(`UPDATE hg_youban_publish_daily_report SET notify_status='sending',notify_token='live',notify_lease_until=?`, now.Add(time.Minute)); err != nil {
			return err
		}
		if err = s.deliverDailyReports(ctx, now, 50); err != nil {
			return err
		}
		if fake.calls != calls {
			t.Fatal("live lease stolen")
		}
		if err = s.finishDailyReportDelivery(ctx, 1, "old", "failed", "stale"); err != nil {
			return err
		}
		row, err = tx.GetOne(`SELECT notify_status FROM hg_youban_publish_daily_report WHERE id=1`)
		if err != nil {
			return err
		}
		if row["notify_status"].String() != "sending" {
			t.Fatal("stale worker changed current lease")
		}
		if _, err = tx.Exec(`UPDATE hg_youban_publish_account_setting SET daily_report_enabled=0`); err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE hg_youban_publish_daily_report SET notify_status='pending',notify_next_retry_at=NULL`); err != nil {
			return err
		}
		if err = s.deliverDailyReports(ctx, now, 50); err != nil {
			return err
		}
		if fake.calls != calls {
			t.Fatal("disabled report still sent")
		}
		row, err = tx.GetOne(`SELECT notify_status FROM hg_youban_publish_daily_report WHERE id=1`)
		if err != nil {
			return err
		}
		if row["notify_status"].String() != "skipped" {
			t.Fatal("disabled state not persisted")
		}
		if _, err = tx.Exec(`UPDATE hg_youban_publish_account_setting SET daily_report_enabled=1`); err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE hg_youban_publish_daily_report SET notify_status='failed',notify_cursor=0,notify_retry_count=4,notify_next_retry_at=NULL`); err != nil {
			return err
		}
		fake.failAt = fake.calls + 1
		if err = s.deliverDailyReports(ctx, now, 50); err == nil {
			t.Fatal("final failure swallowed")
		}
		row, err = tx.GetOne(`SELECT notify_retry_count FROM hg_youban_publish_daily_report WHERE id=1`)
		if err != nil {
			return err
		}
		if row["notify_retry_count"].Int() != 5 {
			t.Fatal("retry count not bounded", row)
		}
		calls = fake.calls
		if err = s.deliverDailyReports(ctx, now.Add(time.Hour), 50); err != nil {
			return err
		}
		if fake.calls != calls {
			t.Fatal("exhausted notification retried indefinitely")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
