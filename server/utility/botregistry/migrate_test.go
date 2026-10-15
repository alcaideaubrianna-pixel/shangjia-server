package botregistry

import (
	"context"
	"os"
	"strings"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func migrationFixture(t *testing.T) context.Context {
	t.Helper()
	link := os.Getenv("YOUBAN_REGISTRY_TEST_PG")
	if link == "" {
		t.Skip("requires disposable local youban_registry_test PostgreSQL")
	}
	if !strings.Contains(link, "127.0.0.1") || !strings.HasSuffix(link, "/youban_registry_test") {
		t.Fatal("refusing non-test database")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Link: link}})
	ctx := context.Background()
	if _, err := g.DB().Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		t.Fatal(err)
	}
	sqls := []string{
		`CREATE TABLE hg_youban_publish_bot(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint DEFAULT 0,created_by bigint,bot_token text,bot_name text,status int,bot_type text DEFAULT 'normal')`,
		`CREATE TABLE hg_youban_bot_bot(id bigint PRIMARY KEY,bot_token text,bot_name text,status int,is_official int DEFAULT 0,is_default int DEFAULT 0)`,
		`CREATE TABLE hg_youban_two_way_bot_bot(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,bot_token text,name text,status int,supergroup_id text,welcome_message text)`,
		`INSERT INTO hg_youban_publish_bot VALUES(1,3,30,30,'111:publish','Publish',1,'normal'),(2,3,0,30,'222:two-way','Dedicated',1,'normal')`,
		`INSERT INTO hg_youban_bot_bot VALUES(1,'333:main','Main',1,1,1),(2,'222:two-way','Mirror',1,0,0)`,
		`INSERT INTO hg_youban_two_way_bot_bot VALUES(1,3,30,'222:two-way','Dedicated',1,'-10099','Original welcome'),(2,3,31,'444:disabled','Other',0,'-10098','Other welcome')`,
		`CREATE TABLE hg_youban_publish_tg_job(id bigint,bot_id bigint,received_bot_id bigint)`,
		`INSERT INTO hg_youban_publish_tg_job VALUES(1,2,1)`,
		`CREATE TABLE hg_youban_bot_user(id bigint,bot_id bigint)`,
		`INSERT INTO hg_youban_bot_user VALUES(1,1)`,
		`CREATE TABLE hg_youban_two_way_bot_topic(id bigint,bot_id bigint,thread_id bigint)`,
		`INSERT INTO hg_youban_two_way_bot_topic VALUES(1,1,88),(2,2,99)`,
		`CREATE TABLE hg_youban_two_way_bot_cooperation_config(id bigint,bot_id bigint,two_way_bot_id bigint)`,
		`INSERT INTO hg_youban_two_way_bot_cooperation_config VALUES(1,2,2)`,
		`CREATE TABLE hg_youban_bot_relay_config(id bigint,bot_id bigint,tenant_id bigint,account_id bigint,owner_id bigint,source_bot_id bigint,enabled int)`,
		`INSERT INTO hg_youban_bot_relay_config VALUES(1,2,3,30,99,2,1)`,
	}
	for _, sql := range sqls {
		if _, err := g.DB().Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx
}
func TestMigrationUnifiesIdentityAndPreservesTopicCreationData(t *testing.T) {
	ctx := migrationFixture(t)
	plan, err := Plan(ctx)
	if err != nil || len(plan) != 3 {
		t.Fatal(plan, err)
	}
	if err := Upgrade(ctx); err != nil {
		t.Fatal(err)
	}
	count, err := g.DB().Model(Table).Ctx(ctx).Count()
	if err != nil || count != 4 {
		t.Fatal(count, err)
	}
	row, err := g.DB().Model(Table).Ctx(ctx).WherePri(2).One()
	if err != nil {
		t.Fatal(err)
	}
	if row["account_id"].Int64() != 30 || row["bot_type"].String() != TypeTwoWay || row["supergroup_id"].String() != "-10099" || row["welcome_message"].String() != "Original welcome" {
		t.Fatal("dedicated Bot ownership or management group lost")
	}
	for _, check := range []struct {
		sql  string
		want int64
	}{
		{"SELECT bot_id FROM hg_youban_publish_tg_job WHERE id=1", 2},
		{"SELECT received_bot_id FROM hg_youban_publish_tg_job WHERE id=1", 1},
		{"SELECT bot_id FROM hg_youban_bot_user WHERE id=1", 3},
		{"SELECT bot_id FROM hg_youban_two_way_bot_topic WHERE id=1", 2},
		{"SELECT bot_id FROM hg_youban_two_way_bot_topic WHERE id=2", 4},
		{"SELECT thread_id FROM hg_youban_two_way_bot_topic WHERE id=1", 88},
		{"SELECT bot_id FROM hg_youban_two_way_bot_cooperation_config WHERE id=1", 2},
		{"SELECT two_way_bot_id FROM hg_youban_two_way_bot_cooperation_config WHERE id=1", 4},
		{"SELECT bot_id FROM hg_youban_bot_relay_config WHERE id=1", 2},
		{"SELECT status FROM " + Table + " WHERE id=4", 2},
	} {
		v, err := g.DB().GetValue(ctx, check.sql)
		if err != nil || v.Int64() != check.want {
			t.Fatal(check.sql, v, err)
		}
	}
	for _, source := range []string{"hg_youban_bot_bot", "hg_youban_publish_bot", "hg_youban_two_way_bot_bot"} {
		exists, err := tableExists(ctx, source)
		if err != nil || exists {
			t.Fatal("old identity store remains", source, err)
		}
	}
	if err := Upgrade(ctx); err != nil {
		t.Fatal("migration is not idempotent", err)
	}
}
func TestMigrationOwnershipConflictRollsBack(t *testing.T) {
	ctx := migrationFixture(t)
	if _, err := g.DB().Exec(ctx, "UPDATE hg_youban_two_way_bot_bot SET tenant_id=99 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if err := Upgrade(ctx); err == nil {
		t.Fatal("cross-tenant Bot merged")
	}
	exists, err := tableExists(ctx, "hg_youban_bot_bot")
	if err != nil || !exists {
		t.Fatal("source tables removed on rollback", err)
	}
	count, err := g.DB().Model(Table).Ctx(ctx).Count()
	if err != nil || count != 0 {
		t.Fatal("partial migration committed", count, err)
	}
}
