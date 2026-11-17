package sys

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/internal/library/cache"
)

// This checks the shared business-event boundary, not Telegram transport or a
// profile/publish replay. Every SQL table is transaction-local and disposable.
func TestCollectMaterialCrossLaneLocalPostgres(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cache.SetAdapter(ctx)

	cases := []struct {
		name, status, group                         string
		verifyFirst, delayedConsumer, outsideWindow bool
	}{
		{name: "old_pending_display_new_verify", status: "pending", group: "pending"},
		{name: "processed_ready_late_verify", status: "processed", group: "ready"},
		{name: "processed_complete_late_verify", status: "processed", group: "complete"},
		{name: "dispatched_ready_late_verify", status: "dispatched", group: "ready"},
		{name: "dispatched_complete_late_verify", status: "dispatched", group: "complete"},
		{name: "verify_first_then_display_business_insert", status: "pending", group: "pending", verifyFirst: true},
		{name: "consumer_delayed_six_hours_received_ten_seconds_apart", status: "pending", group: "pending", delayedConsumer: true},
		{name: "source_message_timestamps_six_hours_apart_outside_twenty_minutes", status: "pending", group: "pending", outsideWindow: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			redis.FlushAll() // isolated miniredis only; no shared or configured Redis.
			err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
				ddl := []string{
					`CREATE TEMP TABLE hg_youban_publish_collect_event(
 id bigint PRIMARY KEY,tenant_id bigint DEFAULT 1,account_id bigint DEFAULT 0,tg_account_id bigint DEFAULT 0,
 source_id bigint DEFAULT 1,source_chat_id text DEFAULT '-1004411799859',source_message_id bigint,source_grouped_id text DEFAULT '',
 raw_text text DEFAULT '',media_count int DEFAULT 1,media_json text DEFAULT '[]',status text DEFAULT 'pending',material_role text DEFAULT 'pending',material_group_status text DEFAULT '',
 material_parent_event_id bigint DEFAULT 0,error_message text DEFAULT '',processed_at timestamp,
 received_at timestamp,created_at timestamp,updated_at timestamp) ON COMMIT DROP`,
					`CREATE TEMP TABLE hg_youban_publish_collect_event_log(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,event_id bigint,stage text,status text,message text,meta_text text,created_at timestamp) ON COMMIT DROP`,
					`CREATE TEMP TABLE hg_youban_publish_collect_dispatch(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,event_id bigint) ON COMMIT DROP`,
					`CREATE TEMP TABLE hg_youban_publish_profile_origin(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,event_id bigint) ON COMMIT DROP`,
				}
				columns := make([]string, 0)
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
				if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
					return err
				}
				now := time.Now()
				// Business received_at below is the preserved Telegram source-message
				// timestamp. It is not the raw envelope arrival/queue-consumption
				// time; the Bot adapter's date conversion is tested separately.
				verifyAt := now.Add(-10 * time.Second)
				displayAt := verifyAt.Add(-10 * time.Second)
				displayCreated := now.Add(-time.Minute)
				verifyCreated := now.Add(-5 * time.Second)
				if tc.delayedConsumer {
					displayAt = now.Add(-6 * time.Hour)
					verifyAt = displayAt.Add(10 * time.Second)
					displayCreated = displayAt
					// Message timestamps remain close although its new business projection is
					// consumed six hours after the old display projection.
				}
				if tc.outsideWindow {
					displayAt = now.Add(-6 * time.Hour)
					displayCreated = displayAt
				}
				insertDisplay := func() error {
					var processed any
					if tc.status == "processed" || tc.status == "dispatched" {
						processed = gtime.New(displayCreated)
					}
					_, err := tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_collect_event
 (id,source_message_id,raw_text,status,material_role,material_group_status,received_at,created_at,updated_at,processed_at)
 VALUES(1,100,?,?,'display',?,?,?,?,?)`, strings.Repeat("本地展示资料", 8), tc.status, tc.group, gtime.New(displayAt), gtime.New(displayCreated), gtime.New(displayCreated), processed)
					if err != nil {
						return err
					}
					_, err = tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_collect_event_media(id,event_id,media_type,source_file_id,cache_status) VALUES(1,1,'photo','local-display','ready')`)
					return err
				}
				if !tc.verifyFirst {
					if err := insertDisplay(); err != nil {
						return err
					}
				}
				if _, err := tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_collect_event
 (id,source_message_id,source_chat_id,raw_text,status,material_role,received_at,created_at,updated_at)
 VALUES(2,101,'4411799859','','pending','pending',?,?,?)`, gtime.New(verifyAt), gtime.New(verifyCreated), gtime.New(verifyCreated)); err != nil {
					return err
				}
				if _, err := tx.Ctx(ctx).Exec(`INSERT INTO hg_youban_publish_collect_event_media(id,event_id,media_type,source_file_id,cache_status) VALUES(2,2,'video','local-verify','pending')`); err != nil {
					return err
				}
				verify, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=2`)
				if err != nil {
					return err
				}
				service := NewSysPublish()
				if tc.verifyFirst {
					payload := collectProcessQueuePayload{TenantId: 1, AccountId: 0, SourceId: 1, SourceChatId: "4411799859", EventId: 2}
					err := service.processCollectMessageWindow(ctx, payload, "4411799859", gdb.Result{verify}, map[int64]struct{}{2: {}}, false)
					if !isCollectProcessRetryError(err) {
						t.Fatalf("verify before business display must remain retryable, got %v", err)
					}
					current, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=2`)
					if err != nil {
						return err
					}
					if current["status"].String() == "ignored" || current["status"].String() == "failed" || current["material_parent_event_id"].Int64() != 0 || !current["processed_at"].IsNil() {
						t.Fatalf("verify-first was terminated or misbound: %v", current)
					}
					if err := insertDisplay(); err != nil {
						return err
					}
					verify = current
				}
				display, err := service.findStoredCollectDisplayEvent(ctx, verify)
				if err != nil {
					return err
				}
				if tc.outsideWindow {
					if !display.IsEmpty() {
						t.Fatalf("six-hour source-message timestamp gap matched: %v", display)
					}
					// Also test the commit guard: even an explicitly supplied stale parent
					// must fail canonical time validation rather than create a wrong binding.
					stale, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=1`)
					if err != nil {
						return err
					}
					outcome, bindErr := service.bindCollectMaterialPair(ctx, stale, verify)
					if bindErr != nil && !isCollectProcessRetryError(bindErr) {
						return bindErr
					}
					if outcome.IsBound() {
						t.Fatalf("out-of-window explicit bind succeeded: %v", outcome)
					}
					current, err := tx.Ctx(ctx).GetOne(`SELECT material_parent_event_id,material_role FROM hg_youban_publish_collect_event WHERE id=2`)
					if err != nil {
						return err
					}
					if current["material_parent_event_id"].Int64() != 0 || current["material_role"].String() != "pending" {
						t.Fatalf("out-of-window verification mutated: %v", current)
					}
					t.Log("limit: source-message timestamp gaps over 20m are not paired automatically; raw arrival or queue delay alone does not establish this gap")
					return nil
				}
				if display["id"].Int64() != 1 {
					stored, readErr := tx.Ctx(ctx).GetAll(`SELECT id,tenant_id,account_id,tg_account_id,source_id,source_chat_id,source_message_id,received_at,created_at FROM hg_youban_publish_collect_event ORDER BY id`)
					t.Logf("fixture=%v err=%v verifyAt=%v aliases=%v", stored, readErr, verify["received_at"].GTime(), tgChannelCacheLookupIds(verify["source_chat_id"].String()))
					t.Fatalf("old-lane display not found: %v", display)
				}
				outcome, err := service.bindCollectMaterialPair(ctx, display, verify)
				if err != nil {
					return err
				}
				if !outcome.IsBound() {
					t.Fatalf("cross-lane binding refused: %v", outcome)
				}
				current, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=2`)
				if err != nil {
					return err
				}
				if current["material_parent_event_id"].Int64() != 1 || current["material_role"].String() != "verify" || current["material_group_status"].String() != "paired" {
					t.Fatalf("binding not persisted: %v", current)
				}
				parent, err := tx.Ctx(ctx).GetOne(`SELECT * FROM hg_youban_publish_collect_event WHERE id=1`)
				if err != nil {
					return err
				}
				if parent["material_group_status"].String() != "paired" || parent["status"].String() != tc.status {
					t.Fatalf("parent lifecycle changed: %v", parent)
				}
				if tc.status == "processed" || tc.status == "dispatched" {
					if !collectMaterialLateVerificationRepairCandidate(parent) {
						t.Fatal("late verification parent is not eligible for repair")
					}
				}
				t.Log("validated business-event lookup/bind only; this does not assert final profile rebuild or Telegram delivery")
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
