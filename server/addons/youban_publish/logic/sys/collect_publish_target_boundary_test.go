package sys

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"hotgo/internal/library/cache"
)

// A subprocess prevents default DB/Redis singleton state leaking to other tests.
// SQLite and miniredis are disposable; no configured connection is used.
func TestCollectPublishTargetBusinessBoundary(t *testing.T) {
	if sqlReviewSubprocess(t) {
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "sqlite", Name: filepath.Join(t.TempDir(), "boundary.db")}})
	r := miniredis.RunT(t)
	adapter, err := gcfg.NewAdapterContent("cache:\n  adapter: redis\nredis:\n  default:\n    address: '" + r.Addr() + "'\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cache.SetAdapter(ctx)
	execSQL := func(sql string, args ...interface{}) {
		t.Helper()
		if _, err := g.DB().Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	createTargets := func() {
		execSQL(`CREATE TABLE hg_youban_publish_channel(id INTEGER PRIMARY KEY,tenant_id bigint,target_chat_id text,publish_direction text,status int)`)
		execSQL(`INSERT INTO hg_youban_publish_channel VALUES(228,78,'4429787816','up',1)`)
	}
	createTargets()
	s := &sSysPublish{}
	event := func(tenant int64, chat string) gdb.Record {
		return gdb.Record{"id": gvar.New(16246132), "tenant_id": gvar.New(tenant), "account_id": gvar.New(0), "source_id": gvar.New(351), "source_type": gvar.New("bot"), "source_chat_id": gvar.New(chat), "source_message_id": gvar.New(425215)}
	}
	t.Run("f30261_telegram_aliases_and_tenant_isolation", func(t *testing.T) {
		for _, chat := range []string{"4429787816", "-1004429787816"} {
			if _, blocked, err := s.checkCollectEventPublishTarget(ctx, event(78, chat)); err != nil || !blocked {
				t.Fatalf("internal channel 228 / Telegram %s escaped: blocked=%v err=%v", chat, blocked, err)
			}
		}
		for _, e := range []gdb.Record{event(79, "-1004429787816"), event(78, "-1003763752459")} {
			if _, blocked, err := s.checkCollectEventPublishTarget(ctx, e); err != nil || blocked {
				t.Fatalf("unrelated scope blocked: blocked=%v err=%v", blocked, err)
			}
		}
	})
	t.Run("receipt_is_scoped_and_not_shared_with_parent", func(t *testing.T) {
		allow, blocked, err := s.checkCollectEventPublishTarget(ctx, event(79, "4429787816"))
		if err != nil || blocked {
			t.Fatalf("unrelated tenant: blocked=%v err=%v", blocked, err)
		}
		if _, blocked, err := s.checkCollectEventPublishTarget(allow, event(78, "-1004429787816")); err != nil || !blocked {
			t.Fatalf("allow receipt leaked across tenants: blocked=%v err=%v", blocked, err)
		}
		execSQL(`DROP TABLE hg_youban_publish_channel`)
		// A failed query would now error: successful repeated checks prove the
		// receipt eliminates SQL for fanout, including equivalent chat aliases.
		for i := 0; i < 3; i++ {
			if _, blocked, err := s.checkCollectEventPublishTarget(allow, event(79, "-1004429787816")); err != nil || blocked {
				t.Fatalf("same scoped receipt repeated query: %v %v", blocked, err)
			}
		}
		for _, tc := range []struct {
			ctx   context.Context
			event gdb.Record
		}{
			{allow, event(78, "4429787816")},
			{allow, event(79, "3763752459")},
			{ctx, event(79, "4429787816")},
		} {
			if _, _, err := s.checkCollectEventPublishTarget(tc.ctx, tc.event); err == nil {
				t.Fatal("different scope or original context bypassed live SQL failure")
			}
		}
		createTargets()
	})
	t.Run("blocked_and_errors_never_mint_receipts", func(t *testing.T) {
		blockedCtx, blocked, err := s.checkCollectEventPublishTarget(ctx, event(78, "-1004429787816"))
		if err != nil || !blocked {
			t.Fatalf("target allowed: %v %v", blocked, err)
		}
		execSQL(`DROP TABLE hg_youban_publish_channel`)
		failedCtx, _, err := s.checkCollectEventPublishTarget(blockedCtx, event(78, "4429787816"))
		if err == nil {
			t.Fatal("blocked result minted allow receipt")
		}
		if _, _, err := s.checkCollectEventPublishTarget(failedCtx, event(78, "4429787816")); err == nil {
			t.Fatal("failed lookup minted allow receipt")
		}
		createTargets()
	})
	t.Run("old_review_commit_blocks_before_metadata_or_profile_tables", func(t *testing.T) {
		prepared := &collectPreparedMaterial{Content: &collectContentResult{}}
		// No metadata/profile tables exist. An empty title also proves that
		// policy rejection happens before even the subsequent title check.
		for _, chat := range []string{"4429787816", "-1004429787816"} {
			profileID, err := s.commitCollectPreparedProfile(ctx, event(78, chat), prepared, gdb.Record{}, "")
			if profileID != 0 || err == nil || !strings.Contains(err.Error(), "禁止创建资料") {
				t.Fatalf("old review escaped target policy: profileID=%d err=%v", profileID, err)
			}
		}
		execSQL(`DROP TABLE hg_youban_publish_channel`)
		defer createTargets()
		profileID, err := s.commitCollectPreparedProfile(ctx, event(78, "-1004429787816"), prepared, gdb.Record{}, "")
		if profileID != 0 || err == nil || !strings.Contains(err.Error(), "检查采集事件发布频道回流失败") {
			t.Fatalf("old review allowed failed target lookup: profileID=%d err=%v", profileID, err)
		}
	})
	t.Run("commit_inherits_process_receipt_without_target_sql", func(t *testing.T) {
		normal := event(78, "-1003763752459")
		allow, blocked, err := s.checkCollectEventPublishTarget(ctx, normal)
		if err != nil || blocked {
			t.Fatalf("normal process scope rejected: blocked=%v err=%v", blocked, err)
		}
		// Match the context enrichment used by processing/rule branches.
		branchCtx := collectBusinessEventContext(allow, normal, nil, 1)
		execSQL(`DROP TABLE hg_youban_publish_channel`)
		defer createTargets()
		prepared := &collectPreparedMaterial{Content: &collectContentResult{}}
		profileID, err := s.commitCollectPreparedProfile(branchCtx, normal, prepared, gdb.Record{}, "")
		// Reaching the next validation with the target table absent proves no
		// extra lookup, while avoiding metadata extraction or profile writes.
		if profileID != 0 || err == nil || !strings.Contains(err.Error(), "采集资料标题为空") {
			t.Fatalf("commit repeated target SQL or skipped title validation: profileID=%d err=%v", profileID, err)
		}
		for _, other := range []gdb.Record{event(79, "-1003763752459"), event(78, "-1004429787816")} {
			profileID, err := s.commitCollectPreparedProfile(branchCtx, other, prepared, gdb.Record{}, "")
			if profileID != 0 || err == nil || !strings.Contains(err.Error(), "检查采集事件发布频道回流失败") {
				t.Fatalf("commit receipt leaked to another scope: profileID=%d err=%v", profileID, err)
			}
		}
	})
	t.Run("direct_rule_uses_actual_chat_not_internal_target_ids", func(t *testing.T) {
		rule := gdb.Record{"id": gvar.New(1), "target_channel_ids": gvar.New([]int64{123})}
		matched, reason, err := s.dispatchCollectEventByRule(ctx, event(78, "-1004429787816"), nil, rule)
		if err != nil || matched || !strings.Contains(reason, "禁止回流") {
			t.Fatalf("direct target branch escaped: matched=%v reason=%q err=%v", matched, reason, err)
		}
		// An internal primary key equal to the Telegram number is not a loop.
		// No rule/source tables exist, so the legitimate next gate must fail;
		// the old integer comparison would falsely return a successful skip.
		rule["target_channel_ids"] = gvar.New([]int64{3763752459})
		matched, reason, err = s.dispatchCollectEventByRule(ctx, event(78, "-1003763752459"), nil, rule)
		if err == nil || matched || strings.Contains(reason, "禁止回流") {
			t.Fatalf("internal ID treated as Telegram ID: matched=%v reason=%q err=%v", matched, reason, err)
		}
	})
	execSQL(`CREATE TABLE hg_youban_publish_collect_event(id INTEGER PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,source_type text,source_chat_id text,source_message_id bigint,status text,error_message text,processed_at datetime,updated_at datetime,received_at datetime,material_role text,material_parent_event_id bigint)`)
	execSQL(`CREATE TABLE hg_youban_publish_collect_event_log(id INTEGER PRIMARY KEY,tenant_id bigint,account_id bigint,event_id bigint,stage text,status text,message text,meta_text text,created_at datetime)`)
	execSQL(`CREATE TABLE hg_youban_publish_collect_event_media(id INTEGER PRIMARY KEY,event_id bigint,cache_status text,error_message text,updated_at datetime)`)
	execSQL(`CREATE TABLE hg_youban_publish_collect_dispatch(id INTEGER PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,event_id bigint,status text,error_message text,finished_at datetime,updated_at datetime)`)
	t.Run("old_pending_event_ignored_before_source_and_material_work", func(t *testing.T) {
		execSQL(`INSERT INTO hg_youban_publish_collect_event(id,tenant_id,account_id,source_id,source_type,source_chat_id,source_message_id,status,material_role,material_parent_event_id) VALUES(16246132,78,0,351,'bot','-1004429787816',425215,'pending','display',0)`)
		execSQL(`INSERT INTO hg_youban_publish_collect_event_media(id,event_id,cache_status) VALUES(1,16246132,'pending')`)
		// Source/content/profile/channel-map tables intentionally do not exist.
		// Reaching any of those paths before the guard cannot pass this test.
		for i := 0; i < 2; i++ {
			if err := s.processCollectEvent(ctx, 16246132, 78, 0); err != nil {
				t.Fatal(err)
			}
		}
		row, err := g.DB().GetOne(ctx, `SELECT status,error_message,processed_at FROM hg_youban_publish_collect_event WHERE id=16246132`)
		if err != nil || row["status"].String() != "ignored" || !strings.Contains(row["error_message"].String(), "禁止回流") || row["processed_at"].IsEmpty() {
			t.Fatalf("event not terminally ignored: %v err=%v", row, err)
		}
		media, err := g.DB().GetOne(ctx, `SELECT cache_status FROM hg_youban_publish_collect_event_media WHERE id=1`)
		if err != nil || media["cache_status"].String() != "canceled" {
			t.Fatalf("ignored media still active: %v err=%v", media, err)
		}
	})
	t.Run("direct_pending_dispatch_cannot_create_publish_jobs", func(t *testing.T) {
		execSQL(`INSERT INTO hg_youban_publish_collect_dispatch VALUES(91,78,0,351,16246132,'pending','',NULL,NULL)`)
		execSQL(`INSERT INTO hg_youban_publish_collect_dispatch VALUES(92,79,0,351,16246132,'pending','',NULL,NULL)`)
		if err := s.submitCollectProfileDispatch(ctx, 91, 834927, event(78, "-1004429787816")); err != nil {
			t.Fatal(err)
		}
		row, err := g.DB().GetOne(ctx, `SELECT status FROM hg_youban_publish_collect_dispatch WHERE id=91`)
		if err != nil || row["status"].String() != "skipped" {
			t.Fatalf("dispatch not skipped: %v %v", row, err)
		}
		row, err = g.DB().GetOne(ctx, `SELECT status FROM hg_youban_publish_collect_dispatch WHERE id=92`)
		if err != nil || row["status"].String() != "pending" {
			t.Fatalf("unrelated dispatch changed: %v %v", row, err)
		}
	})
	t.Run("database_failure_does_not_acknowledge_pending_work", func(t *testing.T) {
		execSQL(`INSERT INTO hg_youban_publish_collect_event(id,tenant_id,account_id,source_id,source_type,source_chat_id,source_message_id,status,material_role,material_parent_event_id) VALUES(16246133,78,0,351,'bot','-1004429787816',425216,'pending','display',0)`)
		execSQL(`INSERT INTO hg_youban_publish_collect_dispatch VALUES(94,78,0,351,16246133,'pending','',NULL,NULL)`)
		execSQL(`DROP TABLE hg_youban_publish_channel`)
		if err := s.processCollectEvent(ctx, 16246133, 78, 0); err == nil {
			t.Fatal("event query failure was acknowledged")
		}
		if err := s.submitCollectProfileDispatch(ctx, 94, 834927, event(78, "-1004429787816")); err == nil {
			t.Fatal("dispatch query failure was acknowledged")
		}
		for _, table := range []string{"hg_youban_publish_collect_event", "hg_youban_publish_collect_dispatch"} {
			id := int64(16246133)
			if strings.HasSuffix(table, "dispatch") {
				id = 94
			}
			row, err := g.DB().GetOne(ctx, "SELECT status FROM "+table+" WHERE id=?", id)
			if err != nil || row["status"].String() != "pending" {
				t.Fatalf("query failure changed %s: %v err=%v", table, row, err)
			}
		}
		createTargets()
	})
}
