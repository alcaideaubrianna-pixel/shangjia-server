//go:build youban_account_integration

package sys

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
	"github.com/gogf/gf/v2/os/gcfg"
	publishlogic "hotgo/addons/youban_publish/logic/sys"
	pi "hotgo/addons/youban_publish/model/input/sysin"
	ps "hotgo/addons/youban_publish/service"
	"hotgo/internal/library/cache"
)

// Uses actual account creation, password hashing and permission persistence;
// no Telegram or payment request is made by the transactional consumption path.
func TestAccountAuthorizationRealServicesLocalPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("account_auth_real_%d", time.Now().UnixNano())
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = root.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
	node.Namespace = schema
	gdb.SetConfigGroup("default", gdb.ConfigGroup{node})
	db := g.DB()
	t.Cleanup(func() { _ = db.GetCore().ClearTableFieldsAll(context.Background()) })
	if !cache.Initialized() {
		adapter := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
		original := g.Cfg().MustGet(ctx, "cache.adapter").Val()
		if err := adapter.Set("cache.adapter", "memory"); err != nil {
			t.Fatal(err)
		}
		cache.SetAdapter(ctx)
		if err := adapter.Set("cache.adapter", original); err != nil {
			t.Fatal(err)
		}
	}
	previousCache := cache.Instance().GetAdapter()
	cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	t.Cleanup(func() { cache.Instance().SetAdapter(previousCache) })
	for _, file := range []string{"../../resource/sql/install.pgsql.sql", "../../../youban_publish/resource/sql/install.pgsql.sql"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		content := string(raw)
		tables := []string{profileSessionTable, accountBindTbl}
		quoted := ""
		if strings.Contains(file, "youban_publish") {
			tables = []string{"hg_youban_publish_account", "hg_youban_publish_tenant", "hg_youban_publish_account_setting"}
			quoted = "\""
		}
		for _, table := range tables {
			marker := "CREATE TABLE IF NOT EXISTS " + quoted + table + quoted + " ("
			start := strings.Index(content, marker)
			if start < 0 {
				t.Fatalf("missing table %s", table)
			}
			end := strings.Index(content[start:], ";") + start
			if _, err = db.Exec(ctx, content[start:end]); err != nil {
				t.Fatal(err)
			}
		}
		for _, line := range strings.Split(content, "\n") {
			if strings.HasPrefix(line, "CREATE UNIQUE INDEX") && strings.Contains(line, " ON "+accountBindTbl+" ") {
				if _, err = db.Exec(ctx, line); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	for _, sql := range []string{
		`ALTER TABLE hg_youban_publish_account ADD COLUMN last_active_at timestamp,ADD COLUMN public_follow_enabled smallint NOT NULL DEFAULT 0`,
		`ALTER TABLE hg_youban_publish_account_setting ADD COLUMN profile_detail_view_enabled smallint NOT NULL DEFAULT 0,ADD COLUMN group_push_enabled smallint NOT NULL DEFAULT 0`,
		`INSERT INTO hg_youban_publish_tenant(id,name) VALUES(1,'测试租户')`,
		`INSERT INTO hg_youban_publish_account(id,tenant_id,merchant_id,parent_id,account_type,username,nickname,status) VALUES(99,1,1,0,'admin','inviter','邀请管理员',1)`,
		`INSERT INTO hg_youban_bot_profile_session(id,bot_id,telegram_user_id,chat_id,app,account_id,tenant_id,scene,status,payload_json,expires_at) VALUES(1,7,'99','99','api',99,1,'account_share','available','',NOW()+INTERVAL '1 hour'),(2,7,'11','11','api',99,1,'account_authorize','active','{"shareId":1,"username":"recipient"}',NOW()+INTERVAL '1 hour')`,
	} {
		if _, err = db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	previous := ps.SysPublish()
	ps.RegisterSysPublish(publishlogic.NewSysPublish())
	t.Cleanup(func() { ps.RegisterSysPublish(previous) })
	created, err := (&sSysBot{}).consumeAccountAuthorization(ctx, 7, models.User{ID: 11, Username: "recipient"}, "11", &profileSessionRow{Id: 2})
	if err != nil {
		t.Fatal(err)
	}
	if created == nil || created.Id <= 0 || len(created.Password) != 12 {
		t.Fatalf("invalid real account result: %+v", created)
	}
	account, err := db.GetOne(ctx, `SELECT * FROM hg_youban_publish_account WHERE id=?`, created.Id)
	if err != nil {
		t.Fatal(err)
	}
	if account["parent_id"].Int64() != 99 || account["tenant_id"].Int64() != 1 || account["account_type"].String() != "uploader" || account["password_hash"].String() == "" || account["password_hash"].String() == created.Password {
		t.Fatalf("incorrect account persistence: id %d", created.Id)
	}
	capability, err := ps.SysPublish().AccountCapability(ctx, "api", created.Id)
	if err != nil {
		t.Fatal(err)
	}
	if capability.TelegramBindingEnabled != 1 || capability.ProfileDetailViewEnabled != 0 || capability.GroupPushEnabled != 0 {
		t.Fatalf("authorization granted excess permission: %+v", capability)
	}
	binding, err := db.GetOne(ctx, `SELECT account_id FROM hg_youban_bot_account_bind WHERE telegram_user_id='11' AND status=1`)
	if err != nil {
		t.Fatal(err)
	}
	if binding["account_id"].Int64() != created.Id {
		t.Fatal("real account was not bound")
	}
	// The same administrator can subsequently manage the created uploader through
	// the existing setting API; an unrelated administrator cannot.
	_, err = ps.SysPublish().PatchAccountSetting(quickPushMediaUploadContext(ctx, 99), &pi.AccountSettingPatchInp{AccountId: created.Id, Field: "suffixContent", Value: "测试后缀"})
	if err != nil {
		t.Fatal(err)
	}
	// A real binding insertion failure must roll back the newly created account,
	// granted permissions, source consumption and recipient confirmation together.
	for _, sql := range []string{
		`ALTER TABLE hg_youban_bot_account_bind ADD CONSTRAINT reject_test_recipient CHECK(telegram_user_id <> '22')`,
		`INSERT INTO hg_youban_bot_profile_session(id,bot_id,telegram_user_id,chat_id,app,account_id,tenant_id,scene,status,payload_json,expires_at) VALUES(3,7,'99','99','api',99,1,'account_share','available','',NOW()+INTERVAL '1 hour'),(4,7,'22','22','api',99,1,'account_authorize','active','{"shareId":3,"username":"recipient2"}',NOW()+INTERVAL '1 hour')`,
	} {
		if _, err := db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	service := &sSysBot{}
	if _, err := service.consumeAccountAuthorization(ctx, 7, models.User{ID: 22, Username: "recipient2"}, "22", &profileSessionRow{Id: 4}); err == nil {
		t.Fatal("binding failure not returned")
	}
	count, err := db.GetCount(ctx, `SELECT COUNT(*) FROM hg_youban_publish_account WHERE username='recipient2'`)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("failed binding left an orphan real account")
	}
	states, err := db.GetAll(ctx, `SELECT id,status FROM hg_youban_bot_profile_session WHERE id IN (3,4) ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || states[0]["status"].String() != "available" || states[1]["status"].String() != "active" {
		t.Fatalf("failed binding consumed share/session: %v", states)
	}
	if _, err := db.Exec(ctx, `ALTER TABLE hg_youban_bot_account_bind DROP CONSTRAINT reject_test_recipient`); err != nil {
		t.Fatal(err)
	}
	if retried, err := service.consumeAccountAuthorization(ctx, 7, models.User{ID: 22, Username: "recipient2"}, "22", &profileSessionRow{Id: 4}); err != nil || retried == nil {
		t.Fatalf("real service retry failed: %v", err)
	}
	// Competing confirmations now exercise the actual Publish service as well.
	if _, err := db.Exec(ctx, `INSERT INTO hg_youban_bot_profile_session(id,bot_id,telegram_user_id,chat_id,app,account_id,tenant_id,scene,status,payload_json,expires_at) VALUES(5,7,'99','99','api',99,1,'account_share','available','',NOW()+INTERVAL '1 hour'),(6,7,'33','33','api',99,1,'account_authorize','active','{"shareId":5,"username":"recipient3"}',NOW()+INTERVAL '1 hour')`); err != nil {
		t.Fatal(err)
	}
	gate := make(chan struct{})
	results := make(chan bool, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate
			result, err := service.consumeAccountAuthorization(ctx, 7, models.User{ID: 33, Username: "recipient3"}, "33", &profileSessionRow{Id: 6})
			results <- err == nil && result != nil
		}()
	}
	close(gate)
	wg.Wait()
	close(results)
	successes := 0
	for success := range results {
		if success {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("real service concurrency created %d accounts", successes)
	}
	count, err = db.GetCount(ctx, `SELECT COUNT(*) FROM hg_youban_publish_account WHERE username='recipient3'`)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("unexpected actual account count: %d", count)
	}
}
