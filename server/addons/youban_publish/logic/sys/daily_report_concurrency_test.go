package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botService "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/internal/model/entity"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type concurrentReportBot struct {
	botService.ISysBot
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (b *concurrentReportBot) NotifyAccount(ctx context.Context, in *botsysin.NotifyAccountInp) error {
	if b.calls.Add(1) == 1 {
		close(b.entered)
	}
	select {
	case <-b.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Uses committed rows and separate pooled connections: wrapping this test in
// one transaction would accidentally serialize all contenders on one connection.
func TestDailyReportConcurrentWorkersLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	node := gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}
	root, err := gdb.New(node)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("daily_report_concurrency_%d", time.Now().UnixNano())
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = root.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
	node.Namespace = schema
	gdb.SetConfigGroup("default", gdb.ConfigGroup{node})
	db := g.DB()
	t.Cleanup(func() { _ = db.GetCore().ClearTableFieldsAll(context.Background()) })
	for _, sql := range []string{
		`CREATE TABLE hg_youban_publish_daily_report(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,report_date date,source_breakdown_json text,notify_status text DEFAULT 'pending',notify_retry_count int DEFAULT 0,notify_next_retry_at timestamp,notify_lease_until timestamp,notify_token text DEFAULT '',notify_cursor int DEFAULT 0,notify_error varchar(512) DEFAULT '')`,
		`CREATE TABLE hg_youban_publish_account(id bigint,tenant_id bigint,account_type text,status int,deleted_at timestamp)`,
		`CREATE TABLE hg_youban_publish_account_setting(id bigint,tenant_id bigint,account_id bigint,daily_report_enabled int,deleted_at timestamp)`,
		`INSERT INTO hg_youban_publish_account VALUES(2,1,'admin',1,NULL)`,
		`INSERT INTO hg_youban_publish_account_setting VALUES(1,1,2,1,NULL)`,
	} {
		if _, err = db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(&sysin.DailyReportModel{TenantID: 1, AccountID: 2, ReportDate: "2026-10-23"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO hg_youban_publish_daily_report(tenant_id,account_id,report_date,source_breakdown_json) VALUES(1,2,'2026-10-23',?)`, string(raw)); err != nil {
		t.Fatal(err)
	}
	var previous botService.ISysBot
	func() { defer func() { _ = recover() }(); previous = botService.SysBot() }()
	fake := &concurrentReportBot{ISysBot: previous, entered: make(chan struct{}), release: make(chan struct{})}
	botService.RegisterSysBot(fake)
	t.Cleanup(func() { botService.RegisterSysBot(previous) })
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))
	service := &sSysPublish{}
	winner := make(chan error, 1)
	go func() { winner <- service.deliverDailyReports(ctx, now, 50) }()
	select {
	case <-fake.entered:
	case err := <-winner:
		t.Fatalf("worker stopped before sending: %v", err)
	case <-ctx.Done():
		t.Fatal("first worker never reached sender")
	}
	// All contenders have selected the same pending snapshot before the winner
	// completes its external send. They must fail the conditional claim.
	errors := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- service.deliverDailyReport(ctx, &dailyReportDeliveryRow{ID: 1, TenantID: 1, AccountID: 2, Payload: string(raw)}, now)
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Error(err)
		}
	}
	close(fake.release)
	if err = <-winner; err != nil {
		t.Fatal(err)
	}
	if got := fake.calls.Load(); got != 1 {
		t.Fatalf("concurrent delivery calls = %d, want 1", got)
	}
	row, err := db.GetOne(ctx, `SELECT notify_status,notify_cursor FROM hg_youban_publish_daily_report WHERE id=1`)
	if err != nil {
		t.Fatal(err)
	}
	if row["notify_status"].String() != "sent" || row["notify_cursor"].Int() != 1 {
		t.Fatalf("delivery not completed: %v", row)
	}
	if err = service.deliverDailyReports(ctx, now, 50); err != nil {
		t.Fatal(err)
	}
	if fake.calls.Load() != 1 {
		t.Fatal("completed report resent")
	}

	// The independent durable account-notice worker must obey the same claim
	// invariant, including when every scheduler selected an identical snapshot.
	migration, err := os.ReadFile("../../resource/sql/account_notice.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `ALTER TABLE hg_youban_publish_account_setting ADD COLUMN account_event_enabled int DEFAULT 1`); err != nil {
		t.Fatal(err)
	}
	if err = db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO hg_youban_publish_account_notice(tenant_id,account_id,event_key,text_content,created_at) VALUES(1,2,'test-delete','test',?)`, now); err != nil {
		t.Fatal(err)
	}
	fake = &concurrentReportBot{ISysBot: previous, entered: make(chan struct{}), release: make(chan struct{})}
	botService.RegisterSysBot(fake)
	winner = make(chan error, 1)
	go func() { winner <- service.processAccountNotices(ctx, now, 20) }()
	select {
	case <-fake.entered:
	case err := <-winner:
		t.Fatalf("worker stopped before sending: %v", err)
	case <-ctx.Done():
		t.Fatal("notice worker never reached sender")
	}
	errors = make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- service.deliverAccountNotice(ctx, &entity.YoubanPublishAccountNotice{Id: 1, TenantId: 1, AccountId: 2, Category: "account"}, now)
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Error(err)
		}
	}
	close(fake.release)
	if err = <-winner; err != nil {
		t.Fatal(err)
	}
	if fake.calls.Load() != 1 {
		t.Fatalf("notice sent %d times", fake.calls.Load())
	}
	if err = service.processAccountNotices(ctx, now, 20); err != nil {
		t.Fatal(err)
	}
	if fake.calls.Load() != 1 {
		t.Fatal("completed notice resent")
	}
	row, err = db.GetOne(ctx, `SELECT status FROM hg_youban_publish_account_notice WHERE id=1`)
	if err != nil {
		t.Fatal(err)
	}
	if row["status"].String() != "sent" {
		t.Fatalf("notice not completed: %v", row)
	}
}
