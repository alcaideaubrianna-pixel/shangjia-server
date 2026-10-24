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
	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botService "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type accountNoticeTestBot struct {
	botService.ISysBot
	calls int
	fail  bool
	texts []string
}

func (b *accountNoticeTestBot) NotifyAccount(ctx context.Context, in *botsysin.NotifyAccountInp) error {
	b.calls++
	if in.AccountId != 2 || !in.RequireDelivery || in.ParseMode != "HTML" {
		return fmt.Errorf("wrong notice destination")
	}
	if b.fail {
		return fmt.Errorf("injected Telegram failure")
	}
	b.texts = append(b.texts, in.Text)
	return nil
}
func TestAccountDeletionNoticeLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	var previous botService.ISysBot
	func() { defer func() { _ = recover() }(); previous = botService.SysBot() }()
	fake := &accountNoticeTestBot{ISysBot: previous, fail: true}
	botService.RegisterSysBot(fake)
	t.Cleanup(func() { botService.RegisterSysBot(previous) })
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_youban_publish_account(id bigint PRIMARY KEY,tenant_id bigint,parent_id bigint,account_type text,username text,nickname text,status int DEFAULT 1,deleted_by bigint,deleted_at timestamp,updated_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account_setting(tenant_id bigint,account_id bigint,account_event_enabled int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account_notice(id bigserial PRIMARY KEY,tenant_id bigint NOT NULL,account_id bigint NOT NULL,event_key varchar(128) NOT NULL,category varchar(32) NOT NULL DEFAULT 'account',text_content text NOT NULL,status varchar(16) NOT NULL DEFAULT 'pending',attempts int NOT NULL DEFAULT 0,next_retry_at timestamp,lease_until timestamp,lease_token varchar(64) NOT NULL DEFAULT '',error_message varchar(512) NOT NULL DEFAULT '',created_at timestamp,updated_at timestamp,UNIQUE(event_key,account_id)) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_account(id,tenant_id,parent_id,account_type,username,nickname)VALUES(2,1,0,'admin','manager','管理员'),(3,1,2,'uploader','child','<子账户>'),(4,2,5,'uploader','other','其他租户'),(6,1,2,'uploader','rollback','需回滚'),(7,1,2,'uploader','quiet','静默')`,
			`INSERT INTO hg_youban_publish_account_setting VALUES(1,2,1,NULL)`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		s := &sSysPublish{}
		operator := &sysin.AccountModel{Id: 2, Username: "manager", Nickname: "管理员"}
		now := time.Now()
		if err := s.deleteAccountsWithNotices(ctx, operator, []int64{3}, 1); err != nil {
			return err
		}
		if fake.calls != 0 {
			t.Fatal("delete performed network I/O before commit")
		}
		count, err := tx.GetCount(`SELECT COUNT(*) FROM hg_youban_publish_account_notice`)
		if err != nil {
			return err
		}
		if count != 1 {
			t.Fatal("no durable deletion event")
		}
		if err = s.deleteAccountsWithNotices(ctx, operator, []int64{3}, 1); err == nil {
			t.Fatal("repeated delete not rejected")
		}
		if err = s.deleteAccountsWithNotices(ctx, operator, []int64{4}, 1); err == nil {
			t.Fatal("other tenant deletion permitted")
		}
		row, err := tx.GetOne(`SELECT deleted_at FROM hg_youban_publish_account WHERE id=4`)
		if err != nil {
			return err
		}
		if !row["deleted_at"].IsNil() {
			t.Fatal("foreign tenant account deleted")
		}
		// A notice insert failure must roll back the account deletion itself.
		if _, err = tx.Exec(`ALTER TABLE hg_youban_publish_account_notice ADD CONSTRAINT notice_reject_new CHECK(account_id<>2) NOT VALID`); err != nil {
			return err
		}
		if err = s.deleteAccountsWithNotices(ctx, operator, []int64{6}, 1); err == nil {
			t.Fatal("notice persistence failure ignored")
		}
		row, err = tx.GetOne(`SELECT deleted_at FROM hg_youban_publish_account WHERE id=6`)
		if err != nil {
			return err
		}
		if !row["deleted_at"].IsNil() {
			t.Fatal("notice failure left account deleted")
		}
		if _, err = tx.Exec(`ALTER TABLE hg_youban_publish_account_notice DROP CONSTRAINT notice_reject_new`); err != nil {
			return err
		}
		if err = s.processAccountNotices(ctx, now, 20); err == nil {
			t.Fatal("send failure ignored")
		}
		row, err = tx.GetOne(`SELECT status,attempts FROM hg_youban_publish_account_notice`)
		if err != nil {
			return err
		}
		if row["status"].String() != "failed" || row["attempts"].Int() != 1 {
			t.Fatal("retry state not persisted", row)
		}
		if err = s.processAccountNotices(ctx, now.Add(time.Second), 20); err != nil {
			return err
		}
		if fake.calls != 1 {
			t.Fatal("retry backoff ignored")
		}
		fake.fail = false
		if err = s.processAccountNotices(ctx, now.Add(2*time.Minute), 20); err != nil {
			return err
		}
		if len(fake.texts) != 1 || !strings.Contains(fake.texts[0], "&lt;子账户&gt;") {
			t.Fatal("missing escaped deletion notice", fake.texts)
		}
		if err = s.processAccountNotices(ctx, now.Add(3*time.Minute), 20); err != nil {
			return err
		}
		if fake.calls != 2 {
			t.Fatal("completed notice sent again")
		}
		if _, err = tx.Exec(`UPDATE hg_youban_publish_account_setting SET account_event_enabled=0`); err != nil {
			return err
		}
		if err = s.deleteAccountsWithNotices(ctx, operator, []int64{7}, 1); err != nil {
			return err
		}
		if err = s.processAccountNotices(ctx, now.Add(4*time.Minute), 20); err != nil {
			return err
		}
		if fake.calls != 2 {
			t.Fatal("disabled account notice sent")
		}
		row, err = tx.GetOne(`SELECT status FROM hg_youban_publish_account_notice ORDER BY id DESC LIMIT 1`)
		if err != nil {
			return err
		}
		if row["status"].String() != "skipped" {
			t.Fatal("disabled notice not skipped")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
