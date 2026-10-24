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
	ps "hotgo/addons/youban_publish/service"
)

func TestAccountAuthorizationConcurrencyLocalPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("account_auth_test_%d", time.Now().UnixNano())
	if _, err = root.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = root.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
	node.Namespace = schema
	gdb.SetConfigGroup("default", gdb.ConfigGroup{node})
	db := g.DB()
	t.Cleanup(func() { _ = db.GetCore().ClearTableFieldsAll(context.Background()) })
	sql, err := os.ReadFile("../../resource/sql/install.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Use the production table definitions and binding constraints verbatim.
	content := string(sql)
	for _, table := range []string{profileSessionTable, accountBindTbl} {
		marker := "CREATE TABLE IF NOT EXISTS " + table + " ("
		start := strings.Index(content, marker)
		if start < 0 {
			t.Fatalf("missing production table %s", table)
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
	indexes, err := os.ReadFile("../../resource/sql/account_management.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	for repeat := 0; repeat < 2; repeat++ {
		for _, sql := range strings.Split(string(indexes), ";") {
			if strings.TrimSpace(sql) != "" {
				if _, err = db.Exec(ctx, sql); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	for _, sql := range []string{
		`CREATE TABLE authorization_created(id bigint primary key)`,
		`INSERT INTO hg_youban_bot_profile_session(id,bot_id,telegram_user_id,chat_id,app,account_id,tenant_id,scene,status,payload_json,expires_at) VALUES(1,7,'99','99','api',99,1,'account_share','available','',NOW()+INTERVAL '1 hour')`,
		`INSERT INTO hg_youban_bot_profile_session(id,bot_id,telegram_user_id,chat_id,app,account_id,tenant_id,scene,status,payload_json,expires_at) VALUES(2,7,'11','11','api',99,1,'account_authorize','active','{"shareId":1,"username":"recipient"}',NOW()+INTERVAL '1 hour'),(3,7,'11','11','api',99,1,'account_authorize','active','{"shareId":1,"username":"recipient"}',NOW()+INTERVAL '1 hour')`,
	} {
		if _, err = db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	var previous ps.ISysPublish
	func() { defer func() { _ = recover() }(); previous = ps.SysPublish() }()
	fake := &accountAuthorizationTestService{ISysPublish: previous, tenant: 1}
	ps.RegisterSysPublish(fake)
	t.Cleanup(func() { ps.RegisterSysPublish(previous) })
	var wg sync.WaitGroup
	gate := make(chan struct{})
	results := make(chan bool, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			<-gate
			created, err := (&sSysBot{}).consumeAccountAuthorization(ctx, 7, models.User{ID: 11, Username: "recipient"}, "11", &profileSessionRow{Id: id})
			results <- err == nil && created != nil
		}(int64(2 + i%2))
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
	if successes != 1 || fake.calls != 1 {
		t.Fatalf("concurrent consumption created multiple accounts: successes=%d calls=%d", successes, fake.calls)
	}
	count, err := db.GetCount(ctx, `SELECT COUNT(*) FROM hg_youban_bot_account_bind WHERE app='api' AND telegram_user_id='11' AND account_id=123 AND status=1`)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected one real binding, got %d", count)
	}
	audit, err := db.GetOne(ctx, `SELECT payload_json,status FROM hg_youban_bot_profile_session WHERE id=1`)
	if err != nil {
		t.Fatal(err)
	}
	if audit["status"].String() != "completed" || strings.Contains(audit["payload_json"].String(), "secret-must-not-be-persisted") {
		t.Fatalf("bad committed audit: %v", audit)
	}
}
