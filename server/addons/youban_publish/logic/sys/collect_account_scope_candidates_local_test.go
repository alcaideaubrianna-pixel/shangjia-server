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
)

// Explicitly opt in to an isolated local database. No production connection or
// migration is used; all fixtures live in transaction-local temporary tables.
func TestCollectAccountScopeCandidatesLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_MULTISOURCE_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if number, err := strconv.Atoi(port); err != nil || number < 1024 || number > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	previous := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", previous)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{
		Type: "pgsql", Host: "127.0.0.1", Port: port, User: "test", Name: "postgres",
	}})
	defer func() { _ = g.DB().GetCore().ClearTableFieldsAll(context.Background()) }()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, statement := range []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_event(
				id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,source_type text,
				source_chat_id text,source_grouped_id text,source_message_id bigint
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch(
				id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,rule_id bigint,
				event_id bigint,profile_id bigint,status text
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch_channel(dispatch_id bigint,channel_id bigint) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_channel_profile(
				id bigint,tenant_id bigint,account_id bigint,source_id bigint,rule_id bigint,profile_id bigint
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_content_profile(id bigint PRIMARY KEY,source_key text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_state(
				profile_id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,deleted_at timestamp
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_fingerprint(
				profile_id bigint,tenant_id bigint,account_id bigint,channel_id bigint,layer text,
				signature text,item_total int,signature_count int
			) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_event VALUES
				(1,78,0,351,'bot','-1003715905169','g',1),
				(2,78,615,351,'bot','3715905169','g',1),
				(3,78,0,351,'bot','-1003715905169','g',1),
				(4,78,0,352,'bot','-1003715905169','g',1),
				(5,79,0,351,'bot','-1003715905169','g',1),
				(6,78,999,351,'bot','-1003715905169','g',1)`,
			`INSERT INTO hg_youban_publish_collect_dispatch VALUES
				(100,78,0,351,338,1,500,'sent'),
				(101,78,615,351,339,2,501,'sent'),
				(102,78,0,352,338,4,502,'sent'),
				(103,79,0,351,338,5,503,'sent'),
				(104,78,999,351,338,6,504,'sent')`,
			`INSERT INTO hg_youban_publish_collect_dispatch_channel VALUES
				(100,1001),(101,1001),(102,1002),(103,1003),(104,1004)`,
			`INSERT INTO hg_content_profile VALUES
				(500,'collect:v3:78:0:bot:64:-1003715905169:group:g',NULL)`,
			`INSERT INTO hg_youban_publish_profile_state VALUES(500,78,615,NULL)`,
			`INSERT INTO hg_youban_publish_profile_fingerprint VALUES(500,78,0,1001,'text_hash','raw-signature',1,1)`,
			`INSERT INTO hg_youban_publish_collect_channel_profile VALUES(1,78,0,351,338,500)`,
		} {
			if _, err := tx.Ctx(ctx).Exec(statement); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		raw, err := tx.Ctx(ctx).GetOne(`SELECT id,tenant_id,account_id,source_id,source_type,source_chat_id,source_grouped_id,source_message_id FROM hg_youban_publish_collect_event WHERE id=3`)
		if err != nil {
			return err
		}
		branch, err := collectEventForBranch(raw, gdb.Record{
			"id": g.NewVar(351), "tenant_id": g.NewVar(78), "account_id": g.NewVar(615),
		})
		if err != nil {
			return err
		}
		if raw["account_id"].Int64() != 0 || branch["account_id"].Int64() != 615 ||
			collectEventStorageAccountID(branch) != 0 || !reflect.DeepEqual(collectEventAccountScope(branch), []int64{615, 0}) {
			t.Fatal("branch ownership must preserve raw account 0 and resolve owner 615")
		}
		makeRule := func(id int64, channels ...int64) gdb.Record {
			return gdb.Record{
				"id": g.NewVar(id), collectRuleSourceIDField: g.NewVar(351),
				"target_channel_ids": g.NewVar(channels),
			}
		}
		service := &sSysPublish{}
		rule := makeRule(338, 1001, 1002, 1003, 1004)
		filtered, err := service.filterCollectRulesByClaimedSourceGroup(ctx, branch, []gdb.Record{
			rule, makeRule(339, 1001), makeRule(340, 1001),
		})
		if err != nil {
			return err
		}
		if len(filtered) != 2 || filtered[0]["id"].Int64() != 338 ||
			!reflect.DeepEqual(collectRuleTargetChannelIds(filtered[0]), []int64{1002, 1003, 1004}) ||
			filtered[1]["id"].Int64() != 340 {
			t.Fatalf("raw/owner claims lost or foreign tenant/account/source/rule crossed boundary: %v", filtered)
		}

		assertHit(true)
		for _, foreign := range []struct {
			tenant, account, source, rule int64
		}{
			{79, 0, 351, 338}, {78, 999, 351, 338}, {78, 0, 352, 338}, {78, 0, 351, 340},
		} {
			if _, err := tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_collect_dispatch VALUES(200,?,?,?,?,3,500,'sent')`,
				foreign.tenant, foreign.account, foreign.source, foreign.rule); err != nil {
				return err
			}
			assertHit(false)
			if _, err := tx.Ctx(ctx).Exec(`DELETE FROM hg_youban_publish_collect_dispatch WHERE id=200`); err != nil {
				return err
			}
			if _, err := tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_collect_channel_profile VALUES(2,?,?,?,?,500)`,
				foreign.tenant, foreign.account, foreign.source, foreign.rule); err != nil {
				return err
			}
			assertHit(false)
			if _, err := tx.Ctx(ctx).Exec(`DELETE FROM hg_youban_publish_collect_channel_profile WHERE id=2`); err != nil {
				return err
			}
		}
		assertHit(true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
