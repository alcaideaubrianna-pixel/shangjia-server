package sys

import (
	"context"
	"errors"
	"fmt"
	"hotgo/addons/youban_publish/model/input/sysin"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	botin "hotgo/addons/youban_bot/model/input/sysin"
	botservice "hotgo/addons/youban_bot/service"
)

type preferenceTestBot struct {
	botservice.ISysBot
	calls int
	err   error
}

func (b *preferenceTestBot) NotifyAccount(context.Context, *botin.NotifyAccountInp) error {
	b.calls++
	return b.err
}

func TestAccountNotificationPreferenceLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	var previous botservice.ISysBot
	func() { defer func() { _ = recover() }(); previous = botservice.SysBot() }()
	fake := &preferenceTestBot{ISysBot: previous}
	botservice.RegisterSysBot(fake)
	t.Cleanup(func() { botservice.RegisterSysBot(previous) })
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec(`CREATE TEMP TABLE hg_youban_publish_account_setting(tenant_id bigint,account_id bigint,daily_report_enabled int,channel_alert_enabled int,account_event_enabled int,task_failure_enabled int,deleted_at timestamp) ON COMMIT DROP`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_account_setting VALUES(1,2,0,0,0,0,NULL),(2,2,1,1,1,1,NULL),(1,3,0,0,0,0,NOW())`); err != nil {
			return err
		}
		s := &sSysPublish{}
		for _, kind := range []string{"daily", "channel", "account", "task"} {
			calls := fake.calls
			if err := s.notifyAccountWithPreference(ctx, 1, kind, &botin.NotifyAccountInp{AccountId: 2}); err != nil {
				return err
			}
			if fake.calls != calls {
				t.Fatalf("disabled %s sent a notification", kind)
			}
			for _, scope := range [][2]int64{{2, 2}, {1, 3}, {1, 4}} {
				if err := s.notifyAccountWithPreference(ctx, scope[0], kind, &botin.NotifyAccountInp{AccountId: scope[1]}); err != nil {
					return err
				}
			}
			if fake.calls != calls+3 {
				t.Fatalf("%s tenant isolation/defaults incorrect", kind)
			}
		}
		for _, kind := range []string{"membership", "unknown"} {
			if err := s.notifyAccountWithPreference(ctx, 1, kind, &botin.NotifyAccountInp{AccountId: 2}); err == nil {
				t.Fatalf("invalid notification kind accepted: %s", kind)
			}
		}
		if err := s.notifyAccountWithPreference(ctx, 0, "channel", &botin.NotifyAccountInp{AccountId: 2}); err == nil {
			t.Fatal("missing tenant accepted")
		}
		if err := s.notifyAccountWithPreference(ctx, 1, "channel", nil); err == nil {
			t.Fatal("nil input accepted")
		}
		fake.err = errors.New("Telegram unavailable")
		if err := s.notifyAccountWithPreference(ctx, 2, "channel", &botin.NotifyAccountInp{AccountId: 2}); !errors.Is(err, fake.err) {
			t.Fatalf("delivery error lost: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPublishFailureNotificationLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	migration, err := os.ReadFile("../../resource/sql/account_notice.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	tableSQL := strings.Replace(strings.Split(string(migration), ";")[0], "CREATE TABLE IF NOT EXISTS", "CREATE TEMP TABLE", 1) + " ON COMMIT DROP"
	var previous botservice.ISysBot
	func() { defer func() { _ = recover() }(); previous = botservice.SysBot() }()
	fake := &preferenceTestBot{ISysBot: previous}
	botservice.RegisterSysBot(fake)
	t.Cleanup(func() { botservice.RegisterSysBot(previous) })
	err = g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			tableSQL,
			`CREATE TEMP TABLE hg_youban_publish_account_setting(tenant_id bigint,account_id bigint,task_failure_enabled int,deleted_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_account_setting VALUES(1,2,0,NULL)`,
			`CREATE TEMP TABLE hg_youban_publish_account(id bigint,tenant_id bigint,parent_id bigint,account_type text,username text,nickname text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_state(tenant_id bigint,account_id bigint,profile_id bigint,publish_operation_no text,publish_task_status text,publish_task_updated_at timestamp,updated_at timestamp,deleted_at timestamp) ON COMMIT DROP`,
			fmt.Sprintf(`CREATE TEMP TABLE %s(tenant_id bigint,account_id bigint,profile_id bigint,task_status text,updated_at timestamp,deleted_at timestamp) ON COMMIT DROP`, publishNoteIndexTable),
			`INSERT INTO hg_youban_publish_account VALUES(2,1,0,'admin','manager','管理员',NULL),(3,1,2,'uploader','child','<子账户>',NULL),(4,2,2,'uploader','foreign','跨租户',NULL)`,
			`INSERT INTO hg_youban_publish_profile_state VALUES(1,3,10,'current','publishing',NULL,NULL,NULL)`,
			fmt.Sprintf(`INSERT INTO %s VALUES(1,3,10,'publishing',NULL,NULL)`, publishNoteIndexTable),
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		s := &sSysPublish{}
		job := telegramJobRecord{TenantId: 1, AccountId: 3, ProfileId: 10, OperationNo: "old"}
		if err := s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed); err != nil {
			return err
		}
		count, err := tx.GetCount(`SELECT COUNT(*) FROM hg_youban_publish_account_notice`)
		if err != nil {
			return err
		}
		if count != 0 {
			t.Fatal("stale operation generated alert")
		}
		job.OperationNo = "current"
		for i := 0; i < 2; i++ {
			if err := s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed); err != nil {
				return err
			}
		}
		rows, err := tx.GetAll(`SELECT * FROM hg_youban_publish_account_notice`)
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0]["account_id"].Int64() != 2 || rows[0]["category"].String() != "task" || !strings.Contains(rows[0]["text_content"].String(), "&lt;子账户&gt;") {
			t.Fatalf("wrong task alert: %v", rows)
		}
		if err := s.enqueueProfileFailureNoticeTx(ctx, tx, 2, 4, 11, "foreign"); err != nil {
			return err
		}
		count, err = tx.GetCount(`SELECT COUNT(*) FROM hg_youban_publish_account_notice`)
		if err != nil {
			return err
		}
		if count != 1 {
			t.Fatal("alert leaked across tenant")
		}
		// Re-entering the same operation's failure state must not enqueue twice.
		if err := s.enqueueProfileFailureNoticeTx(ctx, tx, 1, 3, 10, "current"); err != nil {
			return err
		}
		count, err = tx.GetCount(`SELECT COUNT(*) FROM hg_youban_publish_account_notice`)
		if err != nil {
			return err
		}
		if count != 1 {
			t.Fatal("operation alert not idempotent")
		}
		if err := s.processAccountNotices(ctx, time.Now(), 20); err != nil {
			return err
		}
		if fake.calls != 0 {
			t.Fatal("disabled task failure preference sent a notification")
		}
		skipped, err := tx.GetCount(`SELECT COUNT(*) FROM hg_youban_publish_account_notice WHERE status='skipped'`)
		if err != nil {
			return err
		}
		if skipped != 1 {
			t.Fatal("disabled notification was left pending")
		}
		// Notification persistence is atomic with the state projection.
		if _, err := tx.Exec(`ALTER TABLE hg_youban_publish_account_notice ADD CONSTRAINT fail_notice CHECK(event_key NOT LIKE 'publish-failed:1:12:%')`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_profile_state VALUES(1,3,12,'rollback','publishing',NULL,NULL,NULL)`); err != nil {
			return err
		}
		job.ProfileId = 12
		job.OperationNo = "rollback"
		if err := s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed); err == nil {
			t.Fatal("notice persistence failure was hidden")
		}
		state, err := tx.GetOne(`SELECT publish_task_status FROM hg_youban_publish_profile_state WHERE profile_id=12`)
		if err != nil {
			return err
		}
		if state["publish_task_status"].String() != "publishing" {
			t.Fatal("state changed without durable notice")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
