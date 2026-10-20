package sys

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestProfileOriginLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires disposable PostgreSQL on 127.0.0.1:55484")
	}
	if sqlReviewSubprocess(t) {
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "postgres"}})
	adapter, err := gcfg.NewAdapterContent(`{"youbanPublish":{"profileOriginEnabled":true}}`)
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	migration, err := os.ReadFile(filepath.Join("..", "..", "resource", "sql", "migrations", "profile_origin.pgsql.sql"))
	if err != nil {
		t.Fatal(err)
	}
	// Private schema, never touches other tests' or application tables.
	db := g.DB()
	schema := "origin_regression"
	if _, err = db.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
	// All DDL and observations stay on a single pinned transaction.
	sentinel := errors.New("rollback test schema")
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("SET LOCAL search_path TO " + schema); err != nil {
			return err
		}
		if _, err := tx.Exec(string(migration)); err != nil {
			return err
		}
		for _, sql := range []string{
			"CREATE TABLE hg_content_profile(id bigint PRIMARY KEY,source_type text,source_key text,deleted_at timestamp)",
			"CREATE TABLE hg_youban_publish_profile_state(profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp)",
			"CREATE TABLE hg_youban_publish_collect_dispatch(id bigint,tenant_id bigint,account_id bigint,profile_id bigint,event_id bigint,source_id bigint,source_type_snapshot text,source_chat_id_snapshot text,source_message_id_snapshot bigint,source_name_snapshot text,source_username_snapshot text,created_at timestamp)",
			"CREATE TABLE hg_youban_publish_collect_event(id bigint,tenant_id bigint,account_id bigint,source_type text,source_chat_id text,source_message_id bigint,bot_id bigint,tg_account_id bigint,source_grouped_id text,source_unique_key text,received_at timestamp)",
			"CREATE TABLE hg_youban_publish_collect_source(id bigint,tenant_id bigint,account_id bigint,title text,source_username text,follow_account_id bigint,deleted_at timestamp)",
			"CREATE TABLE hg_youban_publish_tg_job(id bigint,tenant_id bigint,account_id bigint,profile_id bigint,collect_source_id bigint,collect_event_id bigint,collect_source_chat_id text,collect_source_message_id bigint,created_at timestamp)",
			"INSERT INTO hg_content_profile VALUES(654490,'youban_collect','collect:v2:56:561:-1004439615649:group:14333524971101069',NULL),(654491,'youban_collect','collect:v2:56:561:-1004439615649:message:2000',NULL)",
			"INSERT INTO hg_youban_publish_profile_state VALUES(654490,56,561,NULL),(654491,56,561,NULL)",
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		event := originTestEvent(4794056, "account")
		if err := saveCollectProfileOriginTx(ctx, tx, 654490, event); err != nil {
			return err
		}
		// Retry must preserve the original snapshot, not today's title.
		event["source_name_snapshot"] = g.NewVar("changed title")
		if err := saveCollectProfileOriginTx(ctx, tx, 654490, event); err != nil {
			return err
		}
		bot := originTestEvent(4794057, "bot")
		bot["source_chat_id"] = g.NewVar("-1004439615649")
		bot["bot_id"] = g.NewVar(9)
		if err := saveCollectProfileOriginTx(ctx, tx, 654490, bot); err != nil {
			return err
		}
		origins, err := loadProfileOrigins(ctx, []int64{654490})
		if err != nil {
			return err
		}
		if len(origins[654490]) != 2 || origins[654490][0].ChannelTitle != "historical title" || origins[654490][0].CollectMethod != "account" || origins[654490][1].CollectMethod != "bot" {
			t.Fatalf("lost immutable or multi-collector origin: %+v", origins)
		}
		// All operational records are absent: the permanent origins still resolve.
		rows, err := tx.GetAll("SELECT * FROM hg_youban_publish_profile_origin WHERE profile_id=654490")
		if err != nil {
			return err
		}
		if len(rows) != 2 {
			t.Fatalf("expected two origins, got %d", len(rows))
		}
		// Failure anywhere in the enclosing profile transaction rolls both back.
		failure := errors.New("injected failure")
		nestedErr := db.Transaction(ctx, func(ctx context.Context, nested gdb.TX) error {
			if _, err := nested.Exec("INSERT INTO hg_content_profile VALUES(999,'youban_collect','rollback',NULL)"); err != nil {
				return err
			}
			if err := saveCollectProfileOriginTx(ctx, nested, 999, event); err != nil {
				return err
			}
			return failure
		})
		if !errors.Is(nestedErr, failure) {
			return nestedErr
		}
		row, err := tx.GetOne("SELECT (SELECT count(*) FROM hg_content_profile WHERE id=999) AS profiles,(SELECT count(*) FROM hg_youban_publish_profile_origin WHERE profile_id=999) AS origins")
		if err != nil {
			return err
		}
		if row["profiles"].Int() != 0 || row["origins"].Int() != 0 {
			t.Fatal("transaction leaked profile/origin")
		}
		// Scoped historical preview and apply remain idempotent and never alter profiles.
		s := new(sSysPublish)
		preview, err := s.BackfillProfileOrigins(ctx, 56, 561, 654490, 1, false)
		if err != nil {
			return err
		}
		if preview.Scanned != 1 || preview.Partial != 1 || preview.Inserted != 0 {
			t.Fatalf("unexpected preview %+v", preview)
		}
		applied, err := s.BackfillProfileOrigins(ctx, 56, 561, 654490, 1, true)
		if err != nil {
			return err
		}
		if applied.Inserted != 1 {
			t.Fatalf("unexpected apply %+v", applied)
		}
		repeated, err := s.BackfillProfileOrigins(ctx, 56, 561, 654490, 1, true)
		if err != nil {
			return err
		}
		if repeated.Inserted != 0 {
			t.Fatal("backfill not idempotent")
		}
		if _, err := s.BackfillProfileOrigins(ctx, 0, 561, 0, 1, true); err == nil {
			t.Fatal("unscoped backfill accepted")
		}

		for _, sql := range []string{
			"INSERT INTO hg_content_profile VALUES(654492,'youban_collect','collect:v2:56:561:-1004439615649:message:2001',NULL)",
			"INSERT INTO hg_youban_publish_profile_state VALUES(654492,56,561,NULL)",
			"INSERT INTO hg_youban_publish_collect_source VALUES(348,56,561,'renamed now','new_name',0,NOW())",
			"INSERT INTO hg_youban_publish_collect_event VALUES(4794058,56,561,'account','4439615649',2001,0,54,'','account:54:message:2001',NOW())",
			"INSERT INTO hg_youban_publish_collect_dispatch VALUES(226070,56,561,654492,4794058,348,'account','4439615649',2001,'original title','original_name',NOW())",
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		legacy, err := loadProfileOrigins(ctx, []int64{654492})
		if err != nil {
			return err
		}
		if len(legacy[654492]) != 1 || legacy[654492][0].ChannelTitle != "original title" || legacy[654492][0].CollectMethod != "account" {
			t.Fatalf("mutable config overrode snapshot: %+v", legacy[654492][0])
		}
		archived, err := s.BackfillProfileOrigins(ctx, 56, 561, 654491, 1, true)
		if err != nil {
			return err
		}
		if archived.Complete != 1 || archived.Inserted != 1 {
			t.Fatalf("snapshot backfill %+v", archived)
		}
		for _, table := range []string{"hg_youban_publish_collect_dispatch", "hg_youban_publish_collect_event", "hg_youban_publish_collect_source"} {
			if _, err := tx.Exec("DELETE FROM " + table); err != nil {
				return err
			}
		}
		afterCleanup, err := loadProfileOrigins(ctx, []int64{654492})
		if err != nil {
			return err
		}
		if len(afterCleanup[654492]) != 1 || afterCleanup[654492][0].TgAccountId != 54 || afterCleanup[654492][0].ChannelUsername != "original_name" {
			t.Fatalf("cleanup lost provenance: %+v", afterCleanup)
		}
		base := db.Model("hg_content_profile p").Ctx(ctx).InnerJoin("hg_youban_publish_profile_state ps", "ps.profile_id=p.id").Where("p.id", 654492)
		matched, err := applyProfileOriginSourceFilter(base, &sysin.ProfileListInp{CollectSourceId: 348, CollectSourceChatId: "-1004439615649"}).Count()
		if err != nil {
			return err
		}
		if matched != 1 {
			t.Fatal("permanent source filter lost archived profile")
		}
		resolved := new(sysin.AIOpsTelegramMessageContextModel)
		if err := s.resolveAIOpsOriginMessage(ctx, aiOpsTelegramMessageRef{ChatId: "-1004439615649", MessageId: 2001}, resolved); err != nil {
			return err
		}
		if !resolved.Found || resolved.ProfileId != 654492 || resolved.TenantId != 56 {
			t.Fatalf("MCP source lookup failed %+v", resolved)
		}
		// Source validation failure must not leave a committed profile.
		bad := originTestEvent(999, "unexpected")
		if err := saveCollectProfileOriginTx(ctx, tx, 999, bad); err == nil {
			t.Fatal("new writer accepted unknown method")
		}
		if err := deleteProfileOriginsTx(ctx, tx, []int64{654492}); err != nil {
			return err
		}
		deleted, err := tx.GetOne("SELECT count(*) AS n FROM hg_youban_publish_profile_origin WHERE profile_id=654492")
		if err != nil {
			return err
		}
		if deleted["n"].Int() != 0 {
			t.Fatal("profile purge retained origin")
		}
		// Disabled rollout must not require the new table or enforce new validation.
		if err := adapter.SetContent(`{"youbanPublish":{"profileOriginEnabled":false}}`); err != nil {
			return err
		}
		if _, err := tx.Exec("DROP TABLE hg_youban_publish_profile_origin"); err != nil {
			return err
		}
		if err := saveCollectProfileOriginTx(ctx, tx, 0, nil); err != nil {
			return err
		}
		if _, err := loadProfileOrigins(ctx, []int64{654490}); err != nil {
			return err
		}
		legacyBase := db.Model("hg_content_profile p").Ctx(ctx).InnerJoin("hg_youban_publish_profile_state ps", "ps.profile_id=p.id").Where("p.id", 654490)
		legacyFilter, err := s.applyProfileNonKeywordFilters(ctx, legacyBase, &sysin.ProfileListInp{CollectSourceId: 348})
		if err != nil {
			return err
		}
		legacyCount, err := legacyFilter.Count()
		if err != nil {
			return err
		}
		if legacyCount != 0 {
			t.Fatal("disabled gate changed old filter behavior")
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
}

func TestProfileOriginConcurrentLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires disposable PostgreSQL on 127.0.0.1:55484")
	}
	if sqlReviewSubprocess(t) {
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "postgres"}})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db := g.DB()
	migration, err := os.ReadFile(filepath.Join("..", "..", "resource", "sql", "migrations", "profile_origin.pgsql.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	const profileId int64 = 990000002
	cleanup := func() {
		_, _ = db.Exec(context.Background(), "DELETE FROM hg_youban_publish_profile_origin WHERE tenant_id=990000000 AND account_id=990000001 AND profile_id=990000002")
	}
	cleanup()
	defer cleanup()
	errCh := make(chan error, 16)
	for i := 0; i < 16; i++ {
		go func() {
			event := originTestEvent(990000003, "account")
			event["tenant_id"] = g.NewVar(990000000)
			event["account_id"] = g.NewVar(990000001)
			errCh <- db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
				_, err := insertProfileOriginTx(ctx, tx, collectProfileOrigin(event, profileId))
				return err
			})
		}()
	}
	for i := 0; i < 16; i++ {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
	row, err := db.GetOne(ctx, "SELECT count(*) AS n FROM hg_youban_publish_profile_origin WHERE tenant_id=990000000 AND account_id=990000001 AND profile_id=990000002")
	if err != nil {
		t.Fatal(err)
	}
	if row["n"].Int() != 1 {
		t.Fatalf("concurrent retry produced %d origins", row["n"].Int())
	}
}
