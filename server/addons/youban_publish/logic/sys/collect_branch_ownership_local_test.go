package sys

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"

	"hotgo/internal/library/cache"
)

func TestCollectBranchOwnershipLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_MULTISOURCE_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	previousDB := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", previousDB)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{
		Type: "pgsql", Host: "127.0.0.1", Port: port, User: "test", Name: "postgres",
	}})
	defer func() { _ = g.DB().GetCore().ClearTableFieldsAll(context.Background()) }()
	previousConfig := g.Cfg().GetAdapter()
	adapter, err := gcfg.NewAdapterContent("cache:\n  adapter: memory\nyoubanPublish:\n  profileOriginEnabled: false\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	cache.SetAdapter(context.Background())
	defer func() {
		g.Cfg().SetAdapter(previousConfig)
		cache.SetAdapter(context.Background())
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		defer tx.Ctx(ctx)
		for _, statement := range []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_event(
				id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,source_type text,
				bot_id bigint,source_chat_id text,source_grouped_id text,source_message_id bigint,source_unique_key text
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch(
				id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,rule_id bigint,
				event_id bigint,profile_id bigint,status text
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch_channel(dispatch_id bigint,channel_id bigint) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_channel_profile(
				id bigint,tenant_id bigint,account_id bigint,source_id bigint,rule_id bigint,profile_id bigint
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_content_profile(
				id bigint PRIMARY KEY,source_key text,deleted_at timestamp,has_verification_video int,image_count int,
				video_count int,age int,is_virgin int,height int,weight int,cup_size text
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_state(
				profile_id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,deleted_at timestamp
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_fingerprint(
				profile_id bigint,tenant_id bigint,account_id bigint,channel_id bigint,layer text,
				signature text,item_total int,signature_count int
			) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_event VALUES
				(1,87,628,457,'bot',80,'-1003954550954','g',1,'source457'),
				(2,87,628,487,'bot',80,'3954550954','g',1,'source487'),
				(3,87,628,457,'bot',80,'3954550954','g',1,'retry457')`,
			`INSERT INTO hg_content_profile VALUES
				(500,'collect:v3:87:628:bot:80:-1003954550954:group:g',NULL,1,3,1,20,0,160,50,'C')`,
			`INSERT INTO hg_youban_publish_profile_state VALUES(500,87,628,NULL)`,
			`INSERT INTO hg_youban_publish_collect_dispatch VALUES(100,87,628,457,449,1,500,'sent')`,
			`INSERT INTO hg_youban_publish_collect_dispatch_channel VALUES(100,1001)`,
			`INSERT INTO hg_youban_publish_collect_channel_profile VALUES(1,87,628,457,449,500)`,
			`INSERT INTO hg_youban_publish_profile_fingerprint VALUES(500,87,628,1001,'text_hash','raw-signature',1,1)`,
		} {
			if _, err := tx.Ctx(ctx).Exec(statement); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		event, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=1`)
		if err != nil {
			return err
		}
		rule := gdb.Record{"id": g.NewVar(449), collectRuleSourceIDField: g.NewVar(457)}
		profile, err := existingLegacyCollectProfile(ctx, tx, event, rule)
		if err != nil || profile["id"].Int64() != 500 || !profile["legacy_sent"].Bool() {
			t.Fatalf("lost same-branch sent profile: %v %v", profile, err)
		}
		other, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=2`)
		if err != nil {
			return err
		}
		otherRule := gdb.Record{"id": g.NewVar(476), collectRuleSourceIDField: g.NewVar(487), "target_channel_ids": g.NewVar([]int64{1001})}
		profile, err = existingLegacyCollectProfile(ctx, tx, other, otherRule)
		if err != nil || !profile.IsEmpty() {
			t.Fatalf("other branch inherited the sent profile: %v %v", profile, err)
		}
		service := &sSysPublish{}
		items := []profileFingerprint{{ChannelID: 1001, Layer: "text_hash", Signature: "raw-signature", ItemTotal: 1, SignatureCount: 1}}
		hit, err := service.findLegacyCollectRuleDuplicate(ctx, event, rule, items)
		if err != nil || hit == nil || hit.ProfileID != 500 {
			t.Fatalf("same branch lost legacy dedupe: %v %v", hit, err)
		}
		hit, err = service.findLegacyCollectRuleDuplicate(ctx, other, otherRule, items)
		if err != nil || hit != nil {
			t.Fatalf("legacy dedupe crossed branch boundary: %v %v", hit, err)
		}
		retry, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=3`)
		if err != nil {
			return err
		}
		rules := []gdb.Record{
			{"id": g.NewVar(449), collectRuleSourceIDField: g.NewVar(457), "target_channel_ids": g.NewVar([]int64{1001})},
			{"id": g.NewVar(450), collectRuleSourceIDField: g.NewVar(457), "target_channel_ids": g.NewVar([]int64{1001})},
		}
		filtered, err := service.filterCollectRulesByClaimedSourceGroup(ctx, retry, rules)
		if err != nil || len(filtered) != 1 || filtered[0]["id"].Int64() != 450 {
			t.Fatalf("same target consumed another rule: %v %v", filtered, err)
		}
		filtered, err = service.filterCollectRulesByClaimedSourceGroup(ctx, other, []gdb.Record{otherRule})
		if err != nil || len(filtered) != 1 {
			t.Fatalf("same target consumed another source: %v %v", filtered, err)
		}
		if _, err := tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_collect_dispatch VALUES(101,87,628,487,476,2,500,'sent')`); err != nil {
			return err
		}
		if _, err := existingLegacyCollectProfile(ctx, tx, event, rule); err == nil {
			t.Fatal("historically shared profile was silently reassigned")
		}
		hit, err = service.findLegacyCollectRuleDuplicate(ctx, event, rule, items)
		if err != nil || hit != nil {
			t.Fatalf("shared legacy fingerprint allowed profile replacement: %v %v", hit, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
