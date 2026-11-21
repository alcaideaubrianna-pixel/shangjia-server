package sys

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
	"github.com/gogf/gf/v2/os/gcfg"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

func TestCollectManageChannelPermissionsPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:35432")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})

	ctx := context.Background()
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

	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_youban_publish_account(id bigint,tenant_id bigint,parent_id bigint,account_type text,status int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account_setting(account_id bigint,tenant_id bigint,profile_detail_view_enabled int DEFAULT 0,shared_resource_enabled int DEFAULT 0,telegram_binding_enabled int DEFAULT 0,group_push_enabled int DEFAULT 0,collect_push_enabled int DEFAULT 0,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_rule(id bigint,tenant_id bigint,account_id bigint,name text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_rule_item(id bigint,rule_id bigint,item_type text,value text,replacement text,sort int) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_source(id bigint,tenant_id bigint,account_id bigint,bot_id bigint,source_chat_id text,channel_suffix text,updated_at timestamp,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_source_rule(source_id bigint,tenant_id bigint,rule_id bigint,status int) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_channel(id bigint,tenant_id bigint,telegram_chat_id text,title text,username text,invite_link text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_account_channel(id bigint,tenant_id bigint,account_id bigint,channel_id bigint,source_id bigint,status text,collect_enabled int,approval_status text,approved_at timestamp,disabled_at timestamp,updated_at timestamp,deleted_at timestamp,disabled_reason text DEFAULT '') ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_intro_fee_channel_rule(id bigserial,tenant_id bigint,account_id bigint,rule_id bigint,bot_id bigint,channel_id text,suffix text,status int DEFAULT 1,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account_notice(id bigserial,tenant_id bigint,account_id bigint,category text,event_key text,text_content text,status text, UNIQUE(event_key,account_id)) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_account VALUES (1,78,0,'admin',1,NULL),(2,78,1,'uploader',1,NULL),(3,78,1,'uploader',1,NULL),(4,79,0,'uploader',1,NULL),(5,78,1,'uploader',2,NULL)`,
			`INSERT INTO hg_youban_publish_account_setting(account_id,tenant_id,collect_push_enabled) VALUES (2,78,1),(4,79,1),(5,78,1)`,
			`INSERT INTO hg_youban_publish_collect_rule VALUES (10,78,1,'A',NULL),(11,78,1,'B',NULL)`,
			`INSERT INTO hg_youban_publish_collect_source VALUES (20,78,1,40,'','',NULL,NULL)`,
			`INSERT INTO hg_youban_publish_collect_source_rule VALUES (20,78,10,1),(20,78,11,1)`,
			`INSERT INTO hg_youban_publish_collect_channel VALUES (30,78,'-1001234567890','test channel','','',NULL)`,
			`INSERT INTO hg_youban_publish_collect_account_channel VALUES (50,78,1,30,20,'enabled',1,'pending',NULL,NULL,NULL,NULL)`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		s := &sSysPublish{}
		rows, total, err := s.BotCollectManageChannels(ctx, 78, 2, &sysin.CollectManageChannelListInp{Keyword: "test"})
		if err != nil || len(rows) != 2 || total != 2 || rows[0].RuleId != 10 || rows[1].RuleId != 11 {
			t.Fatalf("list rows=%+v total=%d err=%v", rows, total, err)
		}
		approve := &sysin.CollectManageChannelActionInp{RelationId: 50, Action: "approve"}
		for _, actor := range []int64{3, 4, 5} {
			if err := s.BotCollectManageChannelAction(ctx, 78, actor, approve); err == nil {
				t.Fatalf("unauthorized actor %d approved the channel", actor)
			}
		}
		if err := s.BotCollectManageChannelAction(ctx, 78, 1, approve); err != nil {
			return err
		}
		if err := s.BotCollectManageChannelAction(ctx, 78, 2, approve); err == nil {
			t.Fatal("a second approval succeeded")
		}
		if err := s.BotCollectManageChannelAction(ctx, 78, 1, &sysin.CollectManageChannelActionInp{RelationId: 50, Action: "disable"}); err != nil {
			return err
		}
		suffix := &sysin.CollectManageChannelActionInp{RelationId: 50, RuleId: 10, Action: "suffix", Suffix: "new"}
		for i := 0; i < 2; i++ {
			if err := s.BotCollectManageChannelAction(ctx, 78, 2, suffix); err != nil {
				return err
			}
		}
		relation, err := tx.GetOne("SELECT status,approval_status FROM hg_youban_publish_collect_account_channel WHERE id=50")
		if err != nil {
			return err
		}
		if relation["status"].String() != "disabled" || relation["approval_status"].String() != "approved" {
			t.Fatalf("suffix changed channel state: %v", relation)
		}
		notices, err := tx.GetAll("SELECT account_id,text_content FROM hg_youban_publish_account_notice ORDER BY id")
		if err != nil {
			return err
		}
		if len(notices) != 3 || notices[0]["account_id"].Int64() != 2 ||
			!strings.Contains(notices[0]["text_content"].String(), "管理员 ID：1") ||
			!strings.Contains(notices[2]["text_content"].String(), "上架账户 ID：2") {
			t.Fatalf("unexpected notices: %v", notices)
		}
		if _, err := tx.Exec("UPDATE hg_youban_publish_account_setting SET collect_push_enabled=0 WHERE account_id=2"); err != nil {
			return err
		}
		if err := s.BotCollectManageChannelAction(ctx, 78, 2, suffix); err == nil {
			t.Fatal("revoked actor can still submit")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
