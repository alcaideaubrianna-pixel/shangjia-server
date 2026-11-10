package sys

import (
	"context"
	"os"
	"testing"
	"time"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/cache"
	"hotgo/internal/model"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
	"github.com/gogf/gf/v2/os/gcfg"
)

// Reproduces the production tenant/account/source/rule/channel relationship.
// Only metadata is copied; no private message content or credentials are used.
func TestIntroFeeChannelBindingOnlineFixturePostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:35432")
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
	if err := cache.Instance().Set(ctx, collectConfigCacheKey, &sysin.CollectConfigModel{Enabled: 1}, time.Minute); err != nil {
		t.Fatal(err)
	}
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_youban_publish_tenant(id bigint,name text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account(id bigint,tenant_id bigint,account_type text,status int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_rule(id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_content_profile(id bigint,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_state(profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_event(id bigint,tenant_id bigint,account_id bigint,source_chat_id text) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch(id bigint,event_id bigint,tenant_id bigint,account_id bigint,profile_id bigint,rule_id bigint,source_chat_id_snapshot text) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_job(tenant_id bigint,account_id bigint,profile_id bigint,collect_event_id bigint,collect_source_chat_id text) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_origin(tenant_id bigint,account_id bigint,profile_id bigint,event_id bigint,chat_id text,origin_kind text) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_tenant VALUES(78,'test',NULL)`,
			`INSERT INTO hg_youban_publish_account VALUES(615,78,'member',1,NULL)`,
			`INSERT INTO hg_youban_publish_collect_rule VALUES(338,78,615,NULL)`,
			`INSERT INTO hg_content_profile SELECT n,NULL FROM generate_series(1,70) n`,
			`INSERT INTO hg_youban_publish_profile_state SELECT n,78,615,NULL FROM generate_series(1,70) n`,
			`CREATE TEMP TABLE hg_youban_publish_intro_fee_channel_rule(id bigint,tenant_id bigint,account_id bigint,rule_id bigint,bot_id bigint,channel_id text,channel_title text,channel_username text,suffix text,status int,last_seen_at timestamp,updated_at timestamp,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_source(id bigint,tenant_id bigint,account_id bigint,source_chat_id text,title text,source_username text,channel_suffix text,bot_id bigint,status int,collect_enabled int,channel_blacklisted int,last_event_at timestamp,updated_at timestamp,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_source_rule(source_id bigint,tenant_id bigint,rule_id bigint) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_channel_profile(tenant_id bigint,account_id bigint,rule_id bigint,channel_id text,profile_id bigint,updated_at timestamp,deleted_at timestamp,count_projected boolean NOT NULL DEFAULT true) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_channel_count(tenant_id bigint,account_id bigint,rule_id bigint,channel_id text,profile_count bigint,ready boolean,updated_at timestamp,last_seen_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_channel_count VALUES(78,615,338,'4386299825',70,true,now(),now())`,
			`CREATE TEMP TABLE hg_youban_publish_bot_channel_cache(tenant_id bigint,bot_id bigint,chat_id text,chat_title text,chat_username text,chat_type text,message_count int,last_message_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_channel(id bigint,tenant_id bigint,telegram_chat_id text,title text,username text,invite_link text,status text,last_seen_at timestamp,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_bot_channel(tenant_id bigint,bot_id bigint,channel_id bigint,status text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_account_channel(tenant_id bigint,account_id bigint,channel_id bigint,source_id bigint,status text,collect_enabled int,approval_status text,deleted_at timestamp,UNIQUE(tenant_id,account_id,channel_id,source_id)) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_intro_fee_channel_rule VALUES(36,78,615,338,0,'4386299825','汉庭选妃','htbaoyang888','百花阁  Z2',1,now(),now(),NULL)`,
			`INSERT INTO hg_youban_publish_collect_source VALUES(351,78,615,'','百花阁 BOT 采集','','',64,1,1,0,now(),now(),NULL)`,
			`INSERT INTO hg_youban_publish_collect_source_rule VALUES(351,78,338)`,
			`INSERT INTO hg_youban_publish_bot_channel_cache VALUES(78,64,'-1004386299825','汉庭选妃','htbaoyang888','channel',938,'2026-10-12 19:16:54')`,
			`INSERT INTO hg_youban_publish_collect_channel_profile(tenant_id,account_id,rule_id,channel_id,profile_id,updated_at,deleted_at) SELECT 78,615,338,'4386299825',n,now(),NULL FROM generate_series(1,70) n`,
			`INSERT INTO hg_youban_publish_collect_channel VALUES(272067,78,'-1004386299825','汉庭选妃','htbaoyang888','','active',now(),NULL)`,
			`INSERT INTO hg_youban_publish_collect_bot_channel VALUES(78,64,272067,'discovered',NULL)`,
			`INSERT INTO hg_youban_publish_collect_account_channel VALUES(78,615,272067,351,'disabled',0,'rejected',NULL)`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		service := &sSysPublish{}
		checkList := func(wantStatus int, wantReason string) {
			t.Helper()
			for _, include := range []bool{false, true} {
				for _, keyword := range []string{"汉庭", "htbaoyang888", "4386299825", "百花阁  Z2", ""} {
					in := &sysin.IntroFeeChannelRuleListInp{RuleId: 338, BotId: 0, Keyword: keyword, IncludeCandidates: include}
					in.Page, in.PerPage = 1, 20
					rows, total, err := service.IntroFeeChannelRuleList(ctx, in)
					if err != nil {
						t.Fatalf("public list include=%t keyword=%q: %v", include, keyword, err)
					}
					if total != 1 || len(rows) != 1 {
						t.Fatalf("public list include=%t keyword=%q total=%d rows=%d", include, keyword, total, len(rows))
					}
					row := rows[0]
					if row.Id != 36 || row.Status != wantStatus || row.BlockReason != wantReason || row.ProfileCount != 70 || row.Suffix != "百花阁  Z2" || row.Blacklisted {
						t.Fatalf("public list include=%t keyword=%q: row=%+v", include, keyword, row)
					}
					t.Logf("public list include=%t keyword=%q id=%d status=%d blockReason=%q profileCount=%d", include, keyword, row.Id, row.Status, row.BlockReason, row.ProfileCount)
				}
			}
		}
		// First reproduce the current online state: the real relation is enabled.
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_account_channel SET status='enabled',collect_enabled=1,approval_status='approved'`); err != nil {
			return err
		}
		checkList(1, "")
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_channel_count SET ready=false`); err != nil {
			return err
		}
		pendingInput := &sysin.IntroFeeChannelRuleListInp{RuleId: 338, Keyword: "汉庭", IncludeCandidates: true}
		pendingInput.Page, pendingInput.PerPage = 1, 20
		if _, _, err := service.IntroFeeChannelRuleList(ctx, pendingInput); err == nil {
			t.Fatal("unfinished summary was reported as a valid count")
		}
		if _, err := tx.Exec(`DELETE FROM hg_youban_publish_collect_channel_count; UPDATE hg_youban_publish_collect_channel_profile SET count_projected=false`); err != nil {
			return err
		}
		if _, _, err := service.IntroFeeChannelRuleList(ctx, pendingInput); err == nil {
			t.Fatal("missing summary with unprojected relations was reported as zero")
		}
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_channel_profile SET count_projected=true; INSERT INTO hg_youban_publish_collect_channel_count VALUES(78,615,338,'4386299825',70,true,now(),now())`); err != nil {
			return err
		}
		for _, scope := range [][2]int64{{78, 999}, {79, 615}} {
			counts, err := collectChannelProfileCounts(ctx, scope[0], scope[1], 338, []string{"4386299825"})
			if err != nil || counts["4386299825"] != 0 {
				t.Fatalf("cross-account count leaked: %v err=%v", counts, err)
			}
		}
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_account_channel SET status='disabled',collect_enabled=0,approval_status='rejected'`); err != nil {
			return err
		}
		checkList(2, "频道采集已禁用或拒绝")
		check := func(keyword, reason string, status int) {
			t.Helper()
			rows, _, err := service.introFeeChannelRuleCandidatesWithCounts(ctx, 78, 615, 338, 0, keyword, 1, 20, false)
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				if row.ChannelId == "4386299825" {
					if row.BlockReason != reason || row.Status != status {
						t.Fatalf("keyword=%q status=%d reason=%q, want %d %q", keyword, row.Status, row.BlockReason, status, reason)
					}
					return
				}
			}
			t.Fatalf("configured channel missing for keyword %q", keyword)
		}
		for _, keyword := range []string{"汉庭", "htbaoyang888", "4386299825", "百花阁  Z2", ""} {
			check(keyword, "频道采集已禁用或拒绝", 2)
		}
		if _, err := service.saveIntroFeeChannelCollectionStatus(ctx, 78, 615, &sysin.IntroFeeChannelRuleSaveInp{RuleId: 338, ChannelId: "4386299825", Status: 1}, func(context.Context) (int64, error) { return 36, nil }); err != nil {
			return err
		}
		relation, err := tx.Model("hg_youban_publish_collect_account_channel").Where("source_id", 351).One()
		if err != nil || relation["collect_enabled"].Int() != 1 || relation["approval_status"].String() != "approved" {
			t.Fatalf("real source relation was not enabled: %v err=%v", relation, err)
		}
		zero, err := tx.Model("hg_youban_publish_collect_account_channel").Where("source_id", 0).Count()
		if err != nil || zero != 0 {
			t.Fatalf("created phantom source-zero relation: count=%d err=%v", zero, err)
		}
		check("汉庭", "", 1)
		checkList(1, "")
		// More recent unrelated chats must not evict ownership of a configured channel.
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_bot_channel_cache SELECT 78,64,('-100900'||n::text),'其他频道','','channel',1,now() FROM generate_series(1,120) n`); err != nil {
			return err
		}
		check("", "", 1)
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_source SET collect_enabled=0`); err != nil {
			return err
		}
		check("汉庭", "采集源已暂停", 2)
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_source SET collect_enabled=1`); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_source_rule SET tenant_id=79`); err != nil {
			return err
		}
		check("汉庭", "未绑定采集源或规则", 2)
		if _, err := tx.Exec(`DELETE FROM hg_youban_publish_collect_source_rule`); err != nil {
			return err
		}
		check("汉庭", "未绑定采集源或规则", 2)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
