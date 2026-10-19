package sys

import (
	"context"
	"os"
	"strings"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
	"github.com/gogf/gf/v2/os/gcfg"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/cache"
	"hotgo/internal/model"
)

// Replay source 351/rule 338 with legacy disabled flags in disposable local tables.
// No worker, Telegram request, download or upload is started.
func TestCollectRuleLifecycleLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL at 127.0.0.1:35432")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	ctx := context.WithValue(context.Background(), consts.ContextHTTPKey, &model.Context{User: &model.Identity{Id: 615}})
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
	previous := cache.Instance().GetAdapter()
	cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	t.Cleanup(func() { cache.Instance().SetAdapter(previous) })
	service := &sSysPublish{}
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, ddl := range []string{
			`CREATE TEMP TABLE hg_youban_publish_tenant(id bigint PRIMARY KEY,name text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account(id bigint PRIMARY KEY,tenant_id bigint,status int,account_type text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_channel(id bigint PRIMARY KEY,tenant_id bigint,channel_title text,status int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_source(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_chat_id text,collect_enabled int,status int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_source_rule(id bigserial PRIMARY KEY,tenant_id bigint,source_id bigint UNIQUE,rule_id bigint,status int,sort int,created_at timestamp,updated_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_rule(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,name text,global_enabled int,review_enabled int,dedupe_enabled int,dedupe_days int,full_match_enabled int,block_link int,block_username int,block_plain_text int,header_enabled int,header_markdown text,footer_enabled int,footer_markdown text,sort int,status int,created_by bigint,updated_by bigint,deleted_by bigint,created_at timestamp,updated_at timestamp,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_rule_channel(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,rule_id bigint,channel_id bigint,created_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_rule_item(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,rule_id bigint,item_type text,value text,replacement text,sort int,created_at timestamp,updated_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_tenant VALUES(78,'test',NULL)`,
			`INSERT INTO hg_youban_publish_account VALUES(615,78,1,'member',NULL)`,
			`INSERT INTO hg_youban_publish_channel VALUES(1,78,'测试频道',1,NULL)`,
			`INSERT INTO hg_youban_publish_collect_source VALUES(351,78,615,'',1,1,NULL),(352,78,615,'-1001234567890',1,1,NULL)`,
			`INSERT INTO hg_youban_publish_collect_rule(id,tenant_id,account_id,name,global_enabled,status) VALUES(338,78,615,'351专属规则',0,0),(339,78,615,'352专属规则',0,2),(340,78,999,'其他账号规则',0,0),(341,79,615,'其他租户规则',0,0),(342,78,615,'未绑定规则',0,0)`,
			`INSERT INTO hg_youban_publish_collect_source_rule(tenant_id,source_id,rule_id,status,sort) VALUES(78,351,338,0,1),(78,352,339,0,1)`,
			`INSERT INTO hg_youban_publish_collect_rule_channel(rule_id,channel_id) VALUES(338,1),(340,1),(342,1)`,
			`INSERT INTO hg_youban_publish_collect_rule_item(rule_id,item_type,value,sort) VALUES(338,'keyword','测试',1),(340,'keyword','其他账号',1),(342,'keyword','未绑定',1)`,
		} {
			if _, err := tx.Exec(ddl); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		service.refreshCollectSourceCache(ctx)
		service.refreshCollectEventRulesCache(ctx, 78, 615)
		event, err := g.DB().Model("hg_youban_publish_collect_source").Ctx(ctx).Fields("id AS source_id,source_chat_id").Where("id", 351).One()
		if err != nil {
			return err
		}
		// Legacy rule AND binding flags cannot stop a running source.
		rules, err := service.collectEventRules(ctx, event, 78, 615)
		if err != nil || len(rules) != 1 || rules[0]["id"].Int64() != 338 {
			t.Fatalf("source 351 failed with disabled legacy rule: rules=%v err=%v", rules, err)
		}
		// Channel routing must also ignore legacy rule flags, while honoring source switches.
		event["source_chat_id"].Set("-1001234567890")
		routed, err := service.resolveCollectSourceContext(ctx, event, 78, 615)
		if err != nil || routed.RuleSourceID != 352 {
			t.Fatalf("channel routing skipped legacy rule: route=%+v err=%v", routed, err)
		}
		if _, err = tx.Exec(`UPDATE hg_youban_publish_collect_source SET collect_enabled=0 WHERE id=352`); err != nil {
			return err
		}
		service.refreshCollectSourceCache(ctx)
		routed, err = service.resolveCollectSourceContext(ctx, event, 78, 615)
		if err != nil || routed.RuleSourceID != 351 {
			t.Fatalf("paused source still owns channel routing: route=%+v err=%v", routed, err)
		}
		// Binding saves accept historical disabled rules; ownership boundaries remain strict.
		if err = service.saveCollectSourceRules(ctx, tx, 78, 351, []int64{338}); err != nil {
			return err
		}
		for _, id := range []int64{340, 341} {
			if err = service.saveCollectSourceRules(ctx, tx, 78, 351, []int64{id}); err == nil {
				t.Fatalf("bound foreign rule %d", id)
			}
		}
		// Public save cannot reintroduce a disabled rule, including status=2 from old clients.
		if _, err = service.CollectRuleSave(ctx, &sysin.CollectRuleSaveInp{Id: 338, Name: "351专属规则", TargetChannelIds: []int64{1}, Status: 2}); err != nil {
			return err
		}
		status, err := tx.Model("hg_youban_publish_collect_rule").Where("id", 338).Value("status")
		if err != nil || status.Int() != 1 {
			t.Fatalf("save disabled rule: status=%v err=%v", status, err)
		}
		// A paused source still owns its rule. Neither deletion nor mixed-owner deletion may damage it.
		for _, ids := range [][]int64{{338}, {339}, {338, 340}, {340}, {341}} {
			if err = service.CollectRuleDelete(ctx, &sysin.IdsInp{Ids: ids}); err == nil {
				t.Fatalf("deleted bound or foreign rules: %v", ids)
			}
		}
		count, err := tx.Model(collectRuleChannelTable).WhereIn("rule_id", []int64{338, 340}).Count()
		if err != nil || count != 2 {
			t.Fatalf("rejected delete damaged rule channels: count=%d err=%v", count, err)
		}
		if err = service.CollectRuleDelete(ctx, &sysin.IdsInp{Ids: []int64{342}}); err != nil {
			return err
		}
		count, err = tx.Model(collectRuleItemTable).Where("rule_id", 342).Count()
		if err != nil || count != 0 {
			t.Fatalf("unbound rule cleanup failed: count=%d err=%v", count, err)
		}
		// Missing rules stay a real configuration error, never an unrelated fallback match.
		if _, err = tx.Exec(`UPDATE hg_youban_publish_collect_rule SET deleted_at=NOW() WHERE id=338`); err != nil {
			return err
		}
		service.refreshCollectEventRulesCache(ctx, 78, 615)
		event["source_chat_id"].Set("")
		if _, err = service.collectEventRules(ctx, event, 78, 615); err == nil || !strings.Contains(err.Error(), "不存在或已删除") {
			t.Fatalf("deleted rule accepted: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
