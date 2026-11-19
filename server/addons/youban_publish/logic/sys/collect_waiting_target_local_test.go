package sys

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/internal/library/cache"
)

func TestCollectWaitingTargetWindowLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:55484")
	}
	if sqlReviewSubprocess(t) {
		return
	}
	t.Setenv("YOUBAN_COLLECT_VERIFY_WINDOW", "20m")
	previousDB := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previousDB) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "postgres"}})
	redis := miniredis.RunT(t)
	previousConfig := g.Cfg().GetAdapter()
	adapter, err := gcfg.NewAdapterContent("cache:\n  adapter: redis\nredis:\n  default:\n    address: '" + redis.Addr() + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	t.Cleanup(func() { g.Cfg().SetAdapter(previousConfig) })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cache.SetAdapter(ctx)
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		ddl := []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_event(
 id bigint PRIMARY KEY,tenant_id bigint DEFAULT 78,account_id bigint DEFAULT 615,tg_account_id bigint DEFAULT 0,
 source_id bigint DEFAULT 351,source_chat_id text DEFAULT '-1004411799859',source_message_id bigint,source_grouped_id text DEFAULT '',
 raw_text text DEFAULT '',media_count int DEFAULT 1,media_json text DEFAULT '[]',status text DEFAULT 'pending',material_role text DEFAULT 'pending',material_group_status text DEFAULT '',
 material_parent_event_id bigint DEFAULT 0,error_message text DEFAULT '',processed_at timestamp,
 received_at timestamp,created_at timestamp,updated_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_event_log(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,event_id bigint,stage text,status text,message text,meta_text text,created_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,event_id bigint) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_origin(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,event_id bigint) ON COMMIT DROP`,
		}
		columns := []string{}
		for _, field := range strings.Split(collectEventMediaQueryFields(), ",") {
			typ := "text"
			switch field {
			case "id":
				typ = "bigint PRIMARY KEY"
			case "event_id":
				typ = "bigint"
			case "created_at", "updated_at", "next_retry_at":
				typ = "timestamp"
			}
			columns = append(columns, field+" "+typ)
		}
		ddl = append(ddl, "CREATE TEMP TABLE hg_youban_publish_collect_event_media("+strings.Join(columns, ",")+") ON COMMIT DROP")
		for _, sql := range ddl {
			if _, err := tx.Ctx(ctx).Exec(sql); err != nil {
				return err
			}
		}
		at := gtime.New(time.Now().Add(-10 * time.Second))
		if _, err := tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_collect_event(id,source_message_id,raw_text,status,material_group_status,received_at,created_at,updated_at)
 SELECT n,CASE WHEN n=9 THEN 99 ELSE 99+n END,?,'group_collecting',CASE WHEN n IN (1,2) THEN 'waiting_verify' ELSE 'pending' END,?,?,? FROM generate_series(1,11) n`, strings.Repeat("本地展示资料", 8), at, at, at); err != nil {
			return err
		}
		// 4 is the passive verification immediately after display 1. Other rows
		// deliberately challenge every scope/message/time/processed predicate.
		updates := []string{
			`UPDATE hg_youban_publish_collect_event SET source_message_id=105 WHERE id=2`,
			`UPDATE hg_youban_publish_collect_event SET source_message_id=101,raw_text='',status='ignored',material_role='verify',material_group_status='unmatched',processed_at=LOCALTIMESTAMP WHERE id=4`,
			`UPDATE hg_youban_publish_collect_event SET tenant_id=79 WHERE id=5`,
			`UPDATE hg_youban_publish_collect_event SET account_id=616 WHERE id=6`,
			`UPDATE hg_youban_publish_collect_event SET source_id=352 WHERE id=7`,
			`UPDATE hg_youban_publish_collect_event SET source_chat_id='-1004411799860' WHERE id=8`,
			`UPDATE hg_youban_publish_collect_event SET received_at=received_at+INTERVAL '21 minutes' WHERE id=10`,
			`UPDATE hg_youban_publish_collect_event SET processed_at=LOCALTIMESTAMP WHERE id=11`,
		}
		for _, sql := range updates {
			if _, err := tx.Ctx(ctx).Exec(sql); err != nil {
				return err
			}
		}
		if _, err := tx.Ctx(ctx).Exec(`UPDATE hg_youban_publish_collect_event SET error_message=? WHERE id=4`, collectMaterialVerifyUnmatchedMessage); err != nil {
			return err
		}
		if _, err := tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_collect_event_media(id,event_id,media_type,source_file_id,cache_status) VALUES(1,1,'photo','display','ready'),(4,4,'video','verify','pending')`); err != nil {
			return err
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		target, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=1`)
		if err != nil {
			return err
		}
		payload := collectProcessQueuePayload{TenantId: 78, AccountId: 615, SourceId: 351, SourceChatId: "-1004411799859", EventId: 1}
		if err := dirtyCollectTargetBatch(ctx, payload); err != nil {
			return err
		}
		service := NewSysPublish()
		rows, truncated, err := service.loadTargetCollectMaterialWindow(ctx, payload, target, target["received_at"].GTime())
		if err != nil {
			return err
		}
		if truncated {
			t.Fatal("small fixture unexpectedly truncated")
		}
		got := map[int64]gdb.Record{}
		for _, row := range rows {
			got[row["id"].Int64()] = row
		}
		if len(got) != 3 || got[1].IsEmpty() || got[3].IsEmpty() || got[4].IsEmpty() {
			t.Fatalf("expected waiting target 1 + active 3 + passive verify 4, got IDs %v", collectTargetBatchProcessIDs(rows, 4))
		}
		for _, id := range []int64{2, 5, 6, 7, 8, 9, 10, 11} {
			if !got[id].IsEmpty() {
				t.Fatalf("scope/time/waiting predicate leaked id=%d", id)
			}
		}
		active := collectTargetBatchProcessIDs(rows, 1)
		if _, ok := active[1]; !ok {
			t.Fatal("target would remain uncovered")
		}
		if _, ok := active[4]; ok {
			t.Fatal("ignored verification became active polling")
		}
		explicit := collectTargetBatchProcessIDs(rows, 4)
		if _, ok := explicit[4]; !ok {
			t.Fatal("explicit verification target lost its own task")
		}
		// Context remains usable by real pairing despite not being actively polled.
		outcome, err := service.bindCollectMaterialPair(ctx, target, got[4])
		if err != nil {
			return err
		}
		if !outcome.IsBound() {
			t.Fatalf("passive verification context could not bind: %v", outcome)
		}
		child, err := tx.Ctx(ctx).GetOne(`SELECT material_parent_event_id FROM hg_youban_publish_collect_event WHERE id=4`)
		if err != nil {
			return err
		}
		if child["material_parent_event_id"].Int64() != 1 {
			t.Fatal("passive pair not persisted")
		}
		// Legacy payload zero may load an already-owned stored event, but
		// only that target's storage account, never foreign account 616.
		zero := payload
		zero.AccountId = 0
		legacyRows, _, err := service.loadTargetCollectMaterialWindow(ctx, zero, target, target["received_at"].GTime())
		if err != nil {
			return err
		}
		foundTarget := false
		for _, row := range legacyRows {
			if row["account_id"].Int64() != 615 {
				t.Fatalf("legacy zero payload mixed storage accounts: %v", row)
			}
			foundTarget = foundTarget || row["id"].Int64() == 1
		}
		if !foundTarget {
			t.Fatal("payload zero could not load stored account 615 target")
		}
		mismatch := payload
		mismatch.AccountId = 616
		if _, _, err := service.loadTargetCollectMaterialWindow(ctx, mismatch, target, target["received_at"].GTime()); err == nil {
			t.Fatal("different positive accounts were accepted")
		}
		// Conversely a real-owner payload may resume an old Bot storage-zero
		// event, without adding a contradictory account=615 predicate.
		if _, err := tx.Ctx(ctx).Exec(`UPDATE hg_youban_publish_collect_event SET account_id=0 WHERE id=1`); err != nil {
			return err
		}
		if _, err := tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_collect_event(id,account_id,source_message_id,raw_text,received_at,created_at,updated_at) VALUES(12,0,112,?,?,?,?)`, strings.Repeat("本地展示资料", 8), at, at, at); err != nil {
			return err
		}
		storedZero, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=1`)
		if err != nil {
			return err
		}
		zeroRows, _, err := service.loadTargetCollectMaterialWindow(ctx, payload, storedZero, storedZero["received_at"].GTime())
		if err != nil {
			return err
		}
		zeroIDs := map[int64]bool{}
		for _, row := range zeroRows {
			if row["account_id"].Int64() != 0 {
				t.Fatalf("storage-zero query mixed business accounts: %v", row)
			}
			zeroIDs[row["id"].Int64()] = true
		}
		if !zeroIDs[1] || !zeroIDs[12] {
			t.Fatalf("payload 615 failed to load storage-zero context: %v", zeroIDs)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCollectDisplayRetryUsesBusinessIngestDeadline(t *testing.T) {
	t.Setenv("YOUBAN_COLLECT_VERIFY_WINDOW", "20m")
	now := time.Now()
	display := gdb.Record{"received_at": gvar.New(gtime.New(now.Add(-6 * time.Hour))), "created_at": gvar.New(gtime.New(now.Add(-5 * time.Second)))}
	if collectMaterialUnpairedRetryDelay(display, now) != collectMaterialWaitingVerifyRetryDelay {
		t.Fatal("new business event should retain its verification wait")
	}
	got := collectDisplayRetryDelayFromEvents(display, nil, now)
	if got != collectMaterialWaitingVerifyRetryDelay {
		t.Fatalf("fresh ingress retry=%s", got)
	}
	verifies := gdb.Result{{"received_at": gvar.New(gtime.New(now.Add(-10 * time.Second)))}}
	got = collectDisplayRetryDelayFromEvents(display, verifies, now)
	if got != 50*time.Second {
		t.Fatalf("paired video settle behavior changed: %s", got)
	}
	verifies[0]["received_at"] = gvar.New(gtime.New(now.Add(time.Hour)))
	got = collectDisplayRetryDelayFromEvents(display, verifies, now)
	if got < 20*time.Minute || got > 21*time.Minute {
		t.Fatalf("paired settle exceeded business deadline: %s", got)
	}
	display["created_at"] = gvar.New(gtime.New(now.Add(-22 * time.Minute)))
	if collectMaterialUnpairedRetryDelay(display, now) != collectMaterialVerifyRetryDelay {
		t.Fatal("expired business wait did not close")
	}
	if got := collectDisplayRetryDelayFromEvents(display, nil, now); got != collectMaterialVerifyRetryDelay {
		t.Fatalf("expired retry=%s", got)
	}
}
