package sys

import (
	"context"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"

	"hotgo/internal/library/cache"
)

func TestCollectRulesRejectMissingSource(t *testing.T) {
	if _, err := (&sSysPublish{}).collectEventRules(context.Background(), gdb.Record{}, 87, 628); err == nil {
		t.Fatal("missing source must fail before cache or database access")
	}
}

// Reproduce sources 457/487 on one channel using disposable local tables.
// No collector, publish worker, Telegram request or production connection runs.
func TestCollectRulesKeepSourceOwnershipLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_MULTISOURCE_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	previousDB := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", previousDB)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{
		Type: "pgsql", Host: "127.0.0.1", Port: port, User: "test", Name: "postgres",
	}})
	defer func() { _ = g.DB().GetCore().ClearTableFieldsAll(context.Background()) }()
	previousConfig := g.Cfg().GetAdapter()
	adapter, err := gcfg.NewAdapterContent("cache:\n  adapter: memory\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	cache.SetAdapter(ctx)
	defer func() {
		g.Cfg().SetAdapter(previousConfig)
		cache.SetAdapter(context.Background())
	}()
	service := &sSysPublish{}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, statement := range []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_source(
				id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_chat_id text,
				collect_enabled int,status int,deleted_at timestamp
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_source_rule(
				id bigint PRIMARY KEY,tenant_id bigint,source_id bigint,rule_id bigint,status int,sort int
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_rule(
				id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,name text,
				global_enabled int,status int,sort int,deleted_at timestamp
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_rule_channel(rule_id bigint,channel_id bigint) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_rule_item(
				id bigint PRIMARY KEY,rule_id bigint,item_type text,value text,replacement text,sort int
			) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_source VALUES
				(457,87,628,'3954550954',1,1,NULL),(487,87,628,'-1003954550954',1,1,NULL)`,
			`INSERT INTO hg_youban_publish_collect_source_rule VALUES
				(1203,87,457,449,1,1),(1202,87,487,476,1,1),
				(1204,87,457,901,1,2),(1205,87,457,902,1,3)`,
			`INSERT INTO hg_youban_publish_collect_rule VALUES
				(449,87,628,'Shanghai',0,1,1,NULL),(476,87,628,'Guangdong',0,1,1,NULL),
				(901,88,628,'other tenant',0,1,1,NULL),(902,87,629,'other account',0,1,1,NULL)`,
			`INSERT INTO hg_youban_publish_collect_rule_channel VALUES(449,1001),(476,1002)`,
			`INSERT INTO hg_youban_publish_collect_rule_item VALUES
				(1,449,'intro_fee_suffix','Shanghai','',1),(2,476,'intro_fee_suffix','Guangdong','',1)`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		if err := cache.Instance().Set(ctx, collectEventRulesCacheAccountVersionKey(87, 628), "multisource", time.Minute); err != nil {
			return err
		}
		// Old channel routing and rule cache entries must not affect this rollout.
		version := service.collectSourceCacheVersion(ctx)
		if err := cache.Instance().Set(ctx, "youban_publish:collect:source_route:"+version+":87:628:3954550954", 487, time.Minute); err != nil {
			return err
		}
		if err := cache.Instance().Set(ctx, "youban_publish:collect:event_rules:multisource:87:628:457",
			[]g.Map{{"id": 476, collectRuleSourceIDField: 487}}, time.Minute); err != nil {
			return err
		}
		check := func(sourceID, ruleID, channelID int64, suffix string) {
			t.Helper()
			for _, chatID := range []string{"3954550954", "-1003954550954"} {
				event := gdb.Record{"source_id": g.NewVar(sourceID), "source_chat_id": g.NewVar(chatID)}
				rules, err := service.collectEventRules(ctx, event, 87, 628)
				if err != nil || len(rules) != 1 {
					t.Fatalf("source %d: rules=%v err=%v", sourceID, rules, err)
				}
				if rules[0]["id"].Int64() != ruleID || collectRuleSourceID(event, rules[0]) != sourceID ||
					rules[0]["intro_fee_suffix"].String() != suffix ||
					!reflect.DeepEqual(collectRuleTargetChannelIds(rules[0]), []int64{channelID}) {
					t.Fatalf("source %d inherited another source's rule, suffix or target", sourceID)
				}
			}
		}
		// Warm both caches and revisit in alternating order.
		for i := 0; i < 3; i++ {
			check(457, 449, 1001, "Shanghai")
			check(487, 476, 1002, "Guangdong")
		}
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_rule SET status=2 WHERE id=449`); err != nil {
			return err
		}
		service.refreshCollectEventRulesCache(ctx, 87, 628)
		if _, err := service.collectEventRules(ctx, gdb.Record{"source_id": g.NewVar(457)}, 87, 628); err == nil {
			t.Fatal("disabled own rule must not fall back to source 487")
		}
		check(487, 476, 1002, "Guangdong")
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_source_rule SET status=2 WHERE source_id=487`); err != nil {
			return err
		}
		service.refreshCollectEventRulesCache(ctx, 87, 628)
		if _, err := service.collectEventRules(ctx, gdb.Record{"source_id": g.NewVar(487)}, 87, 628); err == nil {
			t.Fatal("disabled source binding must not fall back to another source")
		}
		// Multiple bindings remain a real fan-out list, not a first-row lookup.
		for _, statement := range []string{
			`UPDATE hg_youban_publish_collect_rule SET status=1 WHERE id=449`,
			`INSERT INTO hg_youban_publish_collect_rule VALUES(450,87,628,'second branch',0,1,2,NULL)`,
			`INSERT INTO hg_youban_publish_collect_source_rule VALUES(1206,87,457,450,1,4)`,
			`INSERT INTO hg_youban_publish_collect_rule_channel VALUES(450,1003)`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		service.refreshCollectEventRulesCache(ctx, 87, 628)
		rules, err := service.collectEventRules(ctx, gdb.Record{"source_id": g.NewVar(457)}, 87, 628)
		if err != nil || len(rules) != 2 || rules[0]["id"].Int64() != 449 || rules[1]["id"].Int64() != 450 {
			t.Fatalf("one source lost a rule branch: %v %v", rules, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCollectDispatchPendingWithExistingJobDoesNotResubmitLocalPostgres(t *testing.T) {
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
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec(`CREATE TEMP TABLE hg_youban_publish_tg_job(
			id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,operation_no text,status text
		) ON COMMIT DROP`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_tg_job VALUES(200,87,628,500,'collect:100','sent')`); err != nil {
			return err
		}
		if _, err := tx.Exec(`CREATE TEMP TABLE ` + publishProfileStateTable + `(
			profile_id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,deleted_at timestamp
		) ON COMMIT DROP`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO ` + publishProfileStateTable + ` VALUES(500,87,628,NULL)`); err != nil {
			return err
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		event := gdb.Record{"id": g.NewVar(1), "tenant_id": g.NewVar(87), "account_id": g.NewVar(628), "source_id": g.NewVar(457)}
		dispatch := gdb.Record{"id": g.NewVar(100), "tenant_id": g.NewVar(87), "account_id": g.NewVar(628), "source_id": g.NewVar(457), "profile_id": g.NewVar(500), "status": g.NewVar("pending")}
		rule := gdb.Record{"id": g.NewVar(449), "review_enabled": g.NewVar(0)}
		service := &sSysPublish{}
		matched, _, err := service.resumeExistingCollectDispatch(ctx, dispatch, event, nil, rule, "")
		if err != nil || !matched {
			t.Fatalf("existing branch job was not preserved: %v %v", matched, err)
		}
		rule["review_enabled"] = g.NewVar(1)
		if matched, _, err = service.resumeExistingCollectDispatch(ctx, dispatch, event, nil, rule, ""); err != nil || !matched {
			t.Fatalf("pending handed-off branch was converted into a review: %v %v", matched, err)
		}
		// Changing review settings must not turn an already sent branch into a new review.
		dispatch["status"] = g.NewVar("sent")
		rule["review_enabled"] = g.NewVar(1)
		if matched, _, err = service.resumeExistingCollectDispatch(ctx, dispatch, event, nil, rule, ""); err != nil || !matched {
			t.Fatalf("sent branch was resubmitted for review: %v %v", matched, err)
		}
		dispatch["status"] = g.NewVar("skipped")
		if matched, _, err = service.resumeExistingCollectDispatch(ctx, dispatch, event, nil, rule, ""); err != nil || matched {
			t.Fatalf("skipped branch incorrectly counted as submitted: %v %v", matched, err)
		}
		row, err := tx.GetOne(`SELECT id,status FROM hg_youban_publish_tg_job WHERE id=200`)
		if err != nil {
			return err
		}
		if row["status"].String() != "sent" {
			t.Fatal("retry changed an already sent Telegram job")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
