package sys

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
)

// Explicit opt-in loopback PostgreSQL fixture. This test never reads a DSN
// from production configuration and creates only transaction-local tables.
func TestLegacyBotZeroAccountBranchLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_MULTISOURCE_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	previousDB := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previousDB) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: "test", Name: "postgres"}})
	t.Cleanup(func() { _ = g.DB().GetCore().ClearTableFieldsAll(context.Background()) })
	previousConfig := g.Cfg().GetAdapter()
	adapter, err := gcfg.NewAdapterContent("youbanPublish:\n  profileOriginEnabled: true\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	t.Cleanup(func() { g.Cfg().SetAdapter(previousConfig) })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		defer tx.Ctx(ctx)
		for _, statement := range []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_event(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,source_type text,bot_id bigint,source_chat_id text,source_grouped_id text,source_message_id bigint,source_unique_key text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,rule_id bigint,event_id bigint,profile_id bigint,status text,review_id bigint DEFAULT 0,match_json text,error_message text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_content_profile(id bigint PRIMARY KEY,source_key text,deleted_at timestamp,has_verification_video int,image_count int,video_count int,age int,is_virgin int,height int,weight int,cup_size text) ON COMMIT DROP`,
			`CREATE TEMP TABLE ` + publishProfileStateTable + `(profile_id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_origin(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,source_id bigint,event_id bigint,origin_kind text) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_channel_profile(profile_id bigint,tenant_id bigint,account_id bigint,source_id bigint,rule_id bigint,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE ` + publishTgJobTable + `(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,operation_no text,status text) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_event(id,tenant_id,account_id,source_id,source_type,bot_id,source_chat_id,source_grouped_id,source_message_id,source_unique_key) VALUES(100,78,0,351,'bot',64,'-1003974614787','album',1001,'bot:351:group:album')`,
			`INSERT INTO hg_youban_publish_collect_dispatch(id,tenant_id,account_id,source_id,rule_id,event_id,profile_id,status) VALUES(500,78,0,351,338,100,42,'pending')`,
			`INSERT INTO hg_content_profile VALUES(42,'collect:bot:351:group:album:338',NULL,0,4,0,22,0,165,50,'C')`,
			`INSERT INTO ` + publishProfileStateTable + ` VALUES(42,78,615,NULL)`,
			`INSERT INTO hg_youban_publish_profile_origin VALUES(1,78,0,42,351,100,'collect')`,
			`INSERT INTO hg_youban_publish_collect_channel_profile VALUES(42,78,0,351,338,NULL)`,
			`INSERT INTO ` + publishTgJobTable + ` VALUES(900,78,0,42,'collect:500','sent')`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		rawEvent, err := tx.GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=100`)
		if err != nil {
			return err
		}
		branch, err := collectEventForBranch(rawEvent, gdb.Record{"id": gvar.New(351), "tenant_id": gvar.New(78), "account_id": gvar.New(615)})
		if err != nil {
			return err
		}
		rule := gdb.Record{"id": gvar.New(338)}
		legacy, err := existingLegacyCollectProfile(ctx, tx, branch, rule)
		if err != nil {
			return err
		}
		if legacy["id"].Int64() != 42 || legacy["legacy_foreign"].Bool() {
			t.Fatal("same-branch raw-zero evidence must reuse the real-owner profile")
		}
		dispatch, err := findCollectBranchDispatch(ctx, tx.Model(publishCollectDispatchTable).Ctx(ctx), branch, rule)
		if err != nil {
			return err
		}
		if dispatch["id"].Int64() != 500 || dispatch["account_id"].Int64() != 0 {
			t.Fatal("accepted dispatch identity/account must not migrate")
		}
		matched, _, err := (&sSysPublish{}).resumeExistingCollectDispatch(ctx, dispatch, branch, nil, rule, "正文")
		if err != nil || !matched {
			t.Fatalf("sent legacy-account operation was treated as missing: matched=%v err=%v", matched, err)
		}
		jobs, err := tx.GetValue(`SELECT count(*) FROM ` + publishTgJobTable)
		if err != nil {
			return err
		}
		if jobs.Int() != 1 {
			t.Fatal("successful old operation must not create another TG job")
		}
		// Origin-only proof must still work, bounded by the exact legacy key.
		if _, err := tx.Exec(`DELETE FROM hg_youban_publish_collect_dispatch WHERE id=500`); err != nil {
			return err
		}
		legacy, err = existingLegacyCollectProfile(ctx, tx, branch, rule)
		if err != nil || legacy["id"].Int64() != 42 {
			t.Fatalf("origin-only exact-key mapping failed: %v", err)
		}
		// A real account-zero profile is NOT an ingress alias. Refuse instead
		// of filtering it away and creating a second profile under account 615.
		if _, err := tx.Exec(`UPDATE ` + publishProfileStateTable + ` SET account_id=0 WHERE profile_id=42`); err != nil {
			return err
		}
		if _, err := existingLegacyCollectProfile(ctx, tx, branch, rule); err == nil {
			t.Fatal("account-zero business profile must require explicit repair")
		}
		if err := validateCollectBranchProfileOwner(ctx, dispatch, branch); err == nil {
			t.Fatal("existing dispatch must not migrate a profile owned by account zero")
		}
		stored, err := tx.GetValue(`SELECT account_id FROM hg_youban_publish_collect_event WHERE id=100`)
		if err != nil {
			return err
		}
		if stored.Int64() != 0 || rawEvent["account_id"].Int64() != 0 || branch["account_id"].Int64() != 615 {
			t.Fatal("raw storage owner and business branch owner were conflated")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
