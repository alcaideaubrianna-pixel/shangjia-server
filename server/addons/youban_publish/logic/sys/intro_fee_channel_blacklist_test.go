package sys

import (
	"context"
	"os"
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func TestIntroFeeChannelBlacklistClaimOwnership(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  introFeeChannelBlacklistRelation
		want bool
	}{
		{"actual enabled", introFeeChannelBlacklistRelation{SourceId: 1516, CollectEnabled: 1, Status: "enabled", ApprovalStatus: "approved"}, true},
		{"legacy generic", introFeeChannelBlacklistRelation{Status: "disabled", ApprovalStatus: "rejected"}, true},
		{"manual disabled", introFeeChannelBlacklistRelation{SourceId: 1516, Status: "disabled", ApprovalStatus: "rejected"}, false},
		{"pending", introFeeChannelBlacklistRelation{SourceId: 1516, Status: "pending", ApprovalStatus: "pending"}, false},
		{"purge", introFeeChannelBlacklistRelation{SourceId: 1516, Status: "disabled", ApprovalStatus: "rejected", DisabledReason: "purge"}, false},
		{"enabled with other reason", introFeeChannelBlacklistRelation{SourceId: 1516, CollectEnabled: 1, Status: "enabled", ApprovalStatus: "approved", DisabledReason: "manual"}, false},
		{"deleted enabled", introFeeChannelBlacklistRelation{SourceId: 1516, CollectEnabled: 1, Status: "enabled", ApprovalStatus: "approved", DeletedAt: gtime.Now()}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := introFeeChannelBlacklistCanClaim(tc.row); got != tc.want {
				t.Fatalf("claim=%t want=%t row=%+v", got, tc.want, tc.row)
			}
		})
	}
}

// Source only: opt-in disposable localhost PG, no environment DSN or consumers.
func TestIntroFeeChannelBlacklistPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:35432")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{
		Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres",
	}})
	ctx := context.Background()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, statement := range []string{
			`CREATE TEMP TABLE hg_youban_publish_intro_fee_channel_rule(id bigint,tenant_id bigint,account_id bigint,rule_id bigint,bot_id bigint,channel_id text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_source(id bigint,tenant_id bigint,account_id bigint,source_chat_id text,bot_id bigint,channel_blacklisted int,status int,collect_enabled int,updated_at timestamp,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_channel(id bigint,tenant_id bigint,telegram_chat_id text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_source_rule(source_id bigint,tenant_id bigint,rule_id bigint) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_bot_channel(tenant_id bigint,bot_id bigint,channel_id bigint,status text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_bot_channel_cache(tenant_id bigint,bot_id bigint,chat_id text) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_source_rule VALUES(1516,62,1435)`,
			`INSERT INTO hg_youban_publish_bot_channel_cache VALUES(62,51,'-1003291507707')`,
			`CREATE TEMP TABLE hg_youban_publish_collect_account_channel(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,channel_id bigint,source_id bigint,collect_enabled int,status text,approval_status text,disabled_reason text DEFAULT '',approved_at timestamp,disabled_at timestamp,created_at timestamp,updated_at timestamp,deleted_at timestamp,UNIQUE(tenant_id,account_id,channel_id,source_id)) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_intro_fee_channel_rule VALUES(56,62,567,1435,0,'3291507707',NULL),(57,62,567,1435,0,'3291507708',NULL)`,
			`INSERT INTO hg_youban_publish_collect_channel VALUES(13925664,62,'-1003291507707',NULL),(13925665,62,'3291507707',NULL),(13925666,62,'3291507708',NULL),(13925667,63,'3291507707',NULL)`,
			`INSERT INTO hg_youban_publish_collect_source VALUES(1516,62,567,'different-root-chat',51,0,1,1,now(),NULL)`,
			`INSERT INTO hg_youban_publish_collect_account_channel(tenant_id,account_id,channel_id,source_id,collect_enabled,status,approval_status,disabled_reason) VALUES
			 (62,567,13925664,0,0,'disabled','rejected',''),
			 (62,567,13925664,1516,1,'enabled','approved',''),
			 (62,567,13925665,1516,1,'enabled','approved',''),
			 (62,567,13925664,1517,0,'disabled','rejected','manual'),
			 (62,567,13925664,1518,0,'pending','pending',''),
			 (62,567,13925664,1519,0,'disabled','rejected','purge'),
			 (62,567,13925664,1520,0,'disabled','rejected',''),
			 (62,567,13925666,1516,1,'enabled','approved',''),
			 (62,568,13925664,1516,1,'enabled','approved',''),
			 (63,567,13925667,1516,1,'enabled','approved','')`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		service := &sSysPublish{}
		set := func(value int) {
			t.Helper()
			if err := service.setIntroFeeChannelBlacklist(ctx, 62, 567, 1435, 0, "-1003291507707", value); err != nil {
				t.Fatal(err)
			}
		}
		checkRead := func(want bool) {
			t.Helper()
			rows := []*sysin.IntroFeeChannelRuleItem{{ChannelId: "3291507707", BotId: 0}, {ChannelId: "-1003291507707", BotId: 51}, {ChannelId: "3291507708"}}
			if err := service.fillIntroFeeChannelBlacklist(ctx, 62, 567, rows); err != nil {
				t.Fatal(err)
			}
			if rows[0].Blacklisted != want || rows[1].Blacklisted != want || rows[2].Blacklisted {
				t.Fatalf("alias blacklist mismatch: %+v %+v %+v", rows[0], rows[1], rows[2])
			}
		}
		checkRead(false)
		set(1) // Saved rule with botID=0 must be accepted.
		if _, err := tx.Exec(`DELETE FROM hg_youban_publish_intro_fee_channel_rule WHERE id=56`); err != nil {
			return err
		}
		set(1) // Resolver proves BOT ownership despite a different source root chat.
		checkRead(true)
		marked, err := tx.Model("hg_youban_publish_collect_account_channel").Where("disabled_reason", introFeeChannelBlacklistReason).Count()
		if err != nil || marked != 4 {
			t.Fatalf("expected two alias markers and two owned relations, count=%d err=%v", marked, err)
		}
		// A later rejection must not be undone or leave a partially cleared marker.
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_account_channel SET approval_status='rejected' WHERE source_id=1516 AND channel_id=13925665`); err != nil {
			return err
		}
		if err := service.setIntroFeeChannelBlacklist(ctx, 62, 567, 1435, 0, "3291507707", 0); err == nil {
			t.Fatal("later rejection was overwritten during restore")
		}
		checkRead(true)
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_account_channel SET approval_status='approved' WHERE source_id=1516 AND channel_id=13925665`); err != nil {
			return err
		}
		set(0)
		set(0)
		checkRead(false)
		restored, err := tx.Model("hg_youban_publish_collect_account_channel").Where("tenant_id", 62).Where("account_id", 567).
			WhereIn("channel_id", []int64{13925664, 13925665}).Where("source_id", 1516).
			Where("collect_enabled", 1).Where("status", "enabled").Where("approval_status", "approved").Where("disabled_reason", "").Count()
		if err != nil || restored != 2 {
			t.Fatalf("restore failed count=%d err=%v", restored, err)
		}
		preserved, err := tx.Model("hg_youban_publish_collect_account_channel").WhereIn("source_id", []int64{1517, 1518, 1519, 1520}).Where("collect_enabled", 0).Count()
		if err != nil || preserved != 4 {
			t.Fatalf("manual/pending/purge/blank disabled relation changed count=%d err=%v", preserved, err)
		}
		untouched, err := tx.Model("hg_youban_publish_collect_account_channel").Where("source_id", 1516).
			Where("collect_enabled", 1).Count()
		if err != nil || untouched != 5 {
			t.Fatalf("other tenant/account/channel changed count=%d err=%v", untouched, err)
		}
		source, err := tx.Model("hg_youban_publish_collect_source").Where("id", 1516).One()
		if err != nil || source["status"].Int() != 1 || source["collect_enabled"].Int() != 1 || source["channel_blacklisted"].Int() != 0 {
			t.Fatalf("shared BOT source was changed: %v err=%v", source, err)
		}
		// Unrelated generic reason must reject atomically, including aliases.
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_account_channel SET disabled_reason='purge' WHERE channel_id=13925665 AND source_id=0`); err != nil {
			return err
		}
		if err := service.setIntroFeeChannelBlacklist(ctx, 62, 567, 1435, 0, "3291507707", 1); err == nil {
			t.Fatal("generic purge was overwritten")
		}
		remaining, err := tx.Model("hg_youban_publish_collect_account_channel").Where("disabled_reason", introFeeChannelBlacklistReason).Count()
		if err != nil || remaining != 0 {
			t.Fatalf("rejected blacklist leaked partial updates count=%d err=%v", remaining, err)
		}
		if err := service.setIntroFeeChannelBlacklist(ctx, 62, 567, 1435, 0, "3291507707", 2); err == nil {
			t.Fatal("invalid blacklist state accepted")
		}
		if err := service.setIntroFeeChannelBlacklist(ctx, 62, 567, 1436, 0, "3291507707", 1); err == nil {
			t.Fatal("another rule's source was accepted as ownership")
		}
		if err := service.setIntroFeeChannelBlacklist(ctx, 62, 567, 1435, 0, "unowned-chat", 1); err == nil {
			t.Fatal("unowned channel accepted")
		}
		if err := service.fillIntroFeeChannelBlacklist(ctx, 62, 0, []*sysin.IntroFeeChannelRuleItem{{ChannelId: "3291507707"}}); err == nil {
			t.Fatal("unscoped blacklist read accepted")
		}
		// Legacy direct flags are OR-ed; an unflagged newer source cannot win.
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_collect_source VALUES(1521,62,567,'3291507708',51,1,1,1,now(),NULL),(1522,62,567,'3291507708',51,0,1,1,now(),NULL)`); err != nil {
			return err
		}
		legacy := []*sysin.IntroFeeChannelRuleItem{{ChannelId: "-1003291507708"}}
		if err := service.fillIntroFeeChannelBlacklist(ctx, 62, 567, legacy); err != nil || !legacy[0].Blacklisted {
			t.Fatalf("legacy OR read failed err=%v row=%+v", err, legacy[0])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
