package sys

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/utility/sourcequeue"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"
	_ "github.com/gogf/gf/contrib/nosql/redis/v2"
	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
)

// Every fixture runs in its own subprocess: DB/cache singletons never reach a
// configured service or leak between cases. No consumer is started.
func TestAIOpsCollectWakeIntegration(t *testing.T) {
	for _, scenario := range []string{"preview", "scope", "gates", "apply", "identity", "queue_failure", "batch", "corrupt_plan", "api_wiring"} {
		t.Run(scenario, func(t *testing.T) {
			if sqlReviewSubprocess(t) {
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "sqlite", Name: filepath.Join(t.TempDir(), "wake.db")}})
			r := miniredis.RunT(t)
			adapter, err := gcfg.NewAdapterContent("cache:\n  adapter: redis\nredis:\n  default:\n    address: '" + r.Addr() + "'\n")
			if err != nil {
				t.Fatal(err)
			}
			g.Cfg().SetAdapter(adapter)
			cache.SetAdapter(ctx)
			execSQL := func(sql string, args ...interface{}) {
				t.Helper()
				if _, err := g.DB().Exec(ctx, sql, args...); err != nil {
					t.Fatal(err)
				}
			}
			execSQL(`CREATE TABLE hg_youban_publish_collect_source(id INTEGER PRIMARY KEY,tenant_id bigint,account_id bigint,source_type text,bot_id bigint,tg_account_id bigint,collect_enabled int,status int,channel_blacklisted int,channel_id bigint,deleted_at datetime)`)
			execSQL(`CREATE TABLE hg_youban_publish_collect_event(id INTEGER PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,source_type text,source_chat_id text,source_message_id bigint,source_unique_key text,created_at datetime,status text,processed_at datetime,received_at datetime)`)
			execSQL(`CREATE TABLE hg_youban_publish_collect_channel(id INTEGER PRIMARY KEY,tenant_id bigint,telegram_chat_id text,title text,deleted_at datetime)`)
			execSQL(`CREATE TABLE hg_youban_publish_collect_account_channel(id INTEGER PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,channel_id bigint,collect_enabled int,status text,approval_status text,disabled_reason text,updated_at datetime,deleted_at datetime)`)
			execSQL(`CREATE TABLE hg_youban_publish_collect_bot_channel(id INTEGER PRIMARY KEY,tenant_id bigint,bot_id bigint,channel_id bigint,status text,updated_at datetime,deleted_at datetime)`)
			execSQL(`CREATE TABLE hg_youban_publish_channel(id INTEGER PRIMARY KEY,tenant_id bigint,target_chat_id text,publish_direction text,status int)`)
			// Configured channel is explicitly denied; the actual event chat must win.
			execSQL(`INSERT INTO hg_youban_publish_collect_source VALUES(351,78,615,'bot',9,0,1,1,0,999,NULL)`)
			execSQL(`INSERT INTO hg_youban_publish_collect_channel VALUES(101,78,'3763752459','actual',NULL),(102,78,'-1003763752459','alias',NULL),(999,78,'11111','configured',NULL),(103,78,'4429787816','target',NULL)`)
			execSQL(`INSERT INTO hg_youban_publish_collect_account_channel VALUES(1,78,615,351,101,1,'enabled','approved','',NULL,NULL),(2,78,615,351,999,0,'disabled','rejected','',NULL,NULL),(3,78,615,351,103,1,'enabled','approved','',NULL,NULL)`)
			execSQL(`INSERT INTO hg_youban_publish_collect_bot_channel VALUES(1,78,9,101,'discovered',NULL,NULL),(2,78,9,999,'removed',NULL,NULL)`)
			execSQL(`INSERT INTO hg_youban_publish_channel VALUES(228,78,'-1004429787816','up',1)`)
			setCache := func(key string, value interface{}) {
				t.Helper()
				if err := cache.Instance().Set(ctx, key, value, time.Hour); err != nil {
					t.Fatal(err)
				}
			}
			setCache(collectConfigCacheKey, &sysin.CollectConfigModel{Enabled: 1})
			setCache(tenantVipCacheKey(78), &sysin.TenantVipStatusModel{TenantId: 78, IsVip: true, Features: []string{sysin.TenantVipFeatureCollectSource}})
			insert := func(id int64, status, chat string, account int64) {
				t.Helper()
				execSQL(`INSERT INTO hg_youban_publish_collect_event VALUES(?,78,?,351,'bot',?,?,?,'2026-11-18 00:00:00',?,NULL,'2026-11-18 00:00:00')`, id, account, chat, id+1000, fmt.Sprintf("event-%d", id), status)
			}
			insert(1, "pending", "3763752459", 0)
			insert(2, "media_ready", "-1003763752459", 615)
			s := &sSysPublish{}
			input := func(ids ...int64) *sysin.AIOpsRepairPlanInp {
				return &sysin.AIOpsRepairPlanInp{TenantId: 78, AccountId: 615, SourceId: 351, EventIds: ids}
			}
			preview := func(ids ...int64) []*sysin.AIOpsCollectWakePreviewItem {
				t.Helper()
				_, items, tenant, owner, err := s.previewAIOpsCollectWake(ctx, input(ids...))
				if err != nil || tenant != 78 || owner != 615 {
					t.Fatalf("preview: tenant=%d owner=%d err=%v", tenant, owner, err)
				}
				return items
			}
			plan := func(ids ...int64) aiOpsRepairPlanPayload {
				return aiOpsRepairPlanPayload{Action: "wake_collect_events", TenantID: 78, AccountID: 615, SourceID: 351, EventIDs: ids, CollectWakeEvents: preview(ids...)}
			}
			assertDecision := func(item *sysin.AIOpsCollectWakePreviewItem, decision sysin.AIOpsCollectWakeDecision) {
				t.Helper()
				if item.Decision != decision || item.Identity == "" {
					t.Fatalf("decision/signature: %+v want %s", item, decision)
				}
			}
			queue, err := sourcequeue.New(collectSourceRuntimeNamespace)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "api_wiring":
				t.Setenv("YOUBAN_AI_OPS_TOKEN", "isolated-collect-wake-api-test")
				execSQL(`CREATE TABLE hg_youban_publish_ai_ops_repair_plan(plan_id text PRIMARY KEY,action text,payload_json text,status text,candidate_count integer,applied_count integer,failed_ids_json text,confirm_token_hash text,error_message text,expires_at datetime,created_at datetime,started_at datetime,finished_at datetime,updated_at datetime)`)
				in := input(1)
				in.Action = "wake_collect_events"
				created, err := s.AIOpsCreateRepairPlan(ctx, in)
				if err != nil {
					t.Fatal(err)
				}
				if created.PlanId == "" || created.ConfirmToken == "" || created.Action != in.Action || created.Candidates != 1 || !reflect.DeepEqual(created.EventIds, []int64{1}) || len(created.WakePreview) != 1 {
					t.Fatalf("create API wiring: %+v", created)
				}
				expires, err := time.Parse(time.RFC3339, created.ExpiresAt)
				if err != nil || !expires.After(time.Now()) {
					t.Fatalf("invalid RFC3339 expiry %q: %v", created.ExpiresAt, err)
				}
				assertDecision(created.WakePreview[0], sysin.CollectWakeEligible)
				if item := created.WakePreview[0]; item.EventId != 1 || item.TenantId != 78 || item.AccountId != 0 || item.OwnerAccountId != 615 || item.SourceId != 351 {
					t.Fatalf("create API preview scope: %+v", item)
				}
				pending, err := s.AIOpsRepairPlanStatus(ctx, &sysin.AIOpsRepairPlanStatusInp{PlanId: created.PlanId})
				if err != nil || pending == nil || pending.Status != "pending" || !reflect.DeepEqual(pending.EventIds, []int64{1}) || pending.WakeProgress != nil {
					t.Fatalf("pending API event IDs/progress: %+v err=%v", pending, err)
				}
				applied, err := s.AIOpsApplyRepairPlan(ctx, &sysin.AIOpsRepairApplyInp{PlanId: created.PlanId, ConfirmToken: created.ConfirmToken})
				if err != nil || applied == nil || applied.PlanId != created.PlanId || applied.Status != "running" {
					t.Fatalf("apply API admission: %+v %v", applied, err)
				}
				deadline := time.NewTimer(5 * time.Second)
				defer deadline.Stop()
				poll := time.NewTicker(10 * time.Millisecond)
				defer poll.Stop()
				var status *sysin.AIOpsRepairPlanStatusModel
				for {
					status, err = s.AIOpsRepairPlanStatus(ctx, &sysin.AIOpsRepairPlanStatusInp{PlanId: created.PlanId})
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(status.EventIds, []int64{1}) {
						t.Fatalf("API status %s lost exact event IDs: %v", status.Status, status.EventIds)
					}
					if status.Status == "completed" {
						break
					}
					if status.Status != "running" {
						t.Fatalf("async wake failed: %+v", status)
					}
					select {
					case <-poll.C:
					case <-deadline.C:
						t.Fatalf("async wake did not finish: %+v", status)
					}
				}
				if status.Action != in.Action || status.Candidates != 1 || status.Applied != 1 || len(status.Failed) != 0 || status.WakeProgress == nil {
					t.Fatalf("completed API result: %+v", status)
				}
				if want := (sysin.AIOpsCollectWakeProgress{Total: 1, Pending: 1}); *status.WakeProgress != want {
					t.Fatalf("queued event falsely completed: %+v want %+v", status.WakeProgress, want)
				}
				for _, stamp := range []string{status.ExpiresAt, status.StartedAt, status.FinishedAt} {
					if _, err := time.Parse(time.RFC3339, stamp); err != nil {
						t.Fatalf("status timestamp %q is not RFC3339: %v", stamp, err)
					}
				}
				for _, lane := range []sourcequeue.Lane{sourcequeue.History, sourcequeue.Realtime} {
					hasWork, err := queue.HasImmediateWork(ctx, sourcequeue.Scope{TenantID: 78, SourceID: 351}, lane)
					if err != nil || hasWork != (lane == sourcequeue.History) {
						t.Fatalf("API wake lane %s: hasWork=%v err=%v", lane, hasWork, err)
					}
				}
				// No queue claim or consumer: accepted wake stays a pending fact.
				row, err := g.DB().GetOne(ctx, `SELECT status,processed_at FROM hg_youban_publish_collect_event WHERE id=1`)
				if err != nil || row["status"].String() != "pending" || !row["processed_at"].IsNil() {
					t.Fatalf("API wake reset business state: %v %v", row, err)
				}
			case "preview":
				items := preview(1, 2, 1)
				if len(items) != 2 || items[0].AccountId != 0 || items[1].AccountId != 615 || items[0].OwnerAccountId != 615 {
					t.Fatalf("dedupe/storage/owner: %+v", items)
				}
				for _, item := range items {
					assertDecision(item, sysin.CollectWakeEligible)
				}
				for i, status := range []string{"processed", "dispatched", "matched", "ignored", "failed", "group_collecting", "waiting_order", "prechecked", "media_pending", "suspended", "unknown"} {
					id := int64(i + 10)
					insert(id, status, "3763752459", 0)
					want := sysin.CollectWakeEligible
					if i < 4 {
						want = sysin.CollectWakeTerminal
					} else if i >= 9 {
						want = sysin.CollectWakeBlocked
					}
					assertDecision(preview(id)[0], want)
				}
				execSQL(`UPDATE hg_youban_publish_collect_event SET processed_at='2026-11-18 01:00:00' WHERE id=1`)
				assertDecision(preview(1)[0], sysin.CollectWakeTerminal)
			case "scope":
				for _, mutate := range []string{"tenant_id=79", "source_id=352", "account_id=616", "account_id=-1", "source_type='account'"} {
					execSQL(`UPDATE hg_youban_publish_collect_event SET ` + mutate + ` WHERE id=1`)
					if _, _, _, _, err := s.previewAIOpsCollectWake(ctx, input(2, 1)); err == nil {
						t.Fatalf("cross scope accepted: %s", mutate)
					}
					execSQL(`UPDATE hg_youban_publish_collect_event SET tenant_id=78,source_id=351,account_id=0,source_type='bot' WHERE id=1`)
				}
				for _, in := range []*sysin.AIOpsRepairPlanInp{nil, input(), input(0), input(-1), input(1, 9999), {TenantId: 78, SourceId: 351, EventIds: []int64{1}}, {TenantId: 79, AccountId: 615, SourceId: 351, EventIds: []int64{1}}} {
					if _, _, _, _, err := s.previewAIOpsCollectWake(ctx, in); err == nil {
						t.Fatalf("invalid scope accepted: %+v", in)
					}
				}
				for _, kind := range []string{"collector", "profile", "number"} {
					in := input(1)
					switch kind {
					case "collector":
						in.CollectorEventIds = []int64{1}
					case "profile":
						in.ProfileIds = []int64{1}
					case "number":
						in.ProfileNos = []string{"1"}
					}
					if _, _, _, _, err := s.previewAIOpsCollectWake(ctx, in); err == nil {
						t.Fatalf("mixed %s accepted", kind)
					}
				}
				execSQL(`UPDATE hg_youban_publish_collect_source SET source_type='account' WHERE id=351`)
				if _, _, _, _, err := s.previewAIOpsCollectWake(ctx, input(1)); err == nil {
					t.Fatal("account source accepted storage zero")
				}
			case "gates":
				insert(3, "pending", "4429787816", 0)
				assertDecision(preview(3)[0], sysin.CollectWakeBlocked)
				for _, change := range []string{"collect_enabled=0", "status=2", "channel_blacklisted=1"} {
					execSQL(`UPDATE hg_youban_publish_collect_source SET ` + change + ` WHERE id=351`)
					assertDecision(preview(1)[0], sysin.CollectWakeBlocked)
					execSQL(`UPDATE hg_youban_publish_collect_source SET collect_enabled=1,status=1,channel_blacklisted=0 WHERE id=351`)
				}
				execSQL(`UPDATE hg_youban_publish_collect_account_channel SET collect_enabled=0 WHERE id=1`)
				assertDecision(preview(1)[0], sysin.CollectWakeBlocked)
				execSQL(`UPDATE hg_youban_publish_collect_account_channel SET collect_enabled=1 WHERE id=1`)
				execSQL(`UPDATE hg_youban_publish_collect_bot_channel SET status='bot_removed' WHERE id=1`)
				assertDecision(preview(1)[0], sysin.CollectWakeBlocked)
				execSQL(`UPDATE hg_youban_publish_collect_bot_channel SET status='discovered' WHERE id=1`)
				setCache(collectConfigCacheKey, &sysin.CollectConfigModel{Enabled: 0})
				assertDecision(preview(1)[0], sysin.CollectWakeBlocked)
				setCache(collectConfigCacheKey, &sysin.CollectConfigModel{Enabled: 1})
				setCache(tenantVipCacheKey(78), &sysin.TenantVipStatusModel{TenantId: 78})
				assertDecision(preview(1)[0], sysin.CollectWakeBlocked)
			case "apply":
				p := plan(1, 2)
				for i := 0; i < 2; i++ {
					result := &sysin.AIOpsRepairApplyModel{}
					if err := s.applyAIOpsCollectWake(ctx, p, result); err != nil || result.Applied != 2 || len(result.Failed) != 0 {
						t.Fatalf("apply %d: %+v err=%v", i, result, err)
					}
				}
				if claim, err := queue.Claim(ctx, sourcequeue.Realtime, time.Minute); err != nil || claim != nil {
					t.Fatalf("history escaped to realtime: %+v %v", claim, err)
				}
				seen := map[int64]bool{}
				for i := 0; i < 2; i++ {
					claim, err := queue.Claim(ctx, sourcequeue.History, time.Minute)
					if err != nil || claim == nil {
						t.Fatalf("history claim: %+v %v", claim, err)
					}
					envelope, err := decodeCollectSourceRuntimeClaim(claim)
					wake := envelope.Process
					if err != nil || !wake.History || seen[wake.EventId] || wake.EventId < 1 || wake.EventId > 2 || len(wake.Observation) == 0 || (wake.EventId == 1 && wake.AccountId != 0) {
						t.Fatalf("wrong history payload: %+v err=%v", wake, err)
					}
					seen[wake.EventId] = true
					if err := queue.Ack(ctx, claim); err != nil {
						t.Fatal(err)
					}
				}
				if claim, err := queue.Claim(ctx, sourcequeue.History, time.Minute); err != nil || claim != nil {
					t.Fatalf("duplicate queue task: %+v %v", claim, err)
				}
				// A wake racing an in-flight lease keeps the stable queue identity;
				// the older ACK must preserve the newly accepted generation.
				one := plan(1)
				if err := s.applyAIOpsCollectWake(ctx, one, &sysin.AIOpsRepairApplyModel{}); err != nil {
					t.Fatal(err)
				}
				old, err := queue.Claim(ctx, sourcequeue.History, time.Minute)
				if err != nil || old == nil {
					t.Fatalf("lease claim: %+v %v", old, err)
				}
				if err := s.applyAIOpsCollectWake(ctx, one, &sysin.AIOpsRepairApplyModel{}); err != nil {
					t.Fatal(err)
				}
				if err := queue.Ack(ctx, old); err != nil {
					t.Fatal(err)
				}
				newer, err := queue.Claim(ctx, sourcequeue.History, time.Minute)
				if err != nil || newer == nil || newer.ID != old.ID {
					t.Fatalf("late ACK lost wake or changed ID: %+v %v", newer, err)
				}
				if err := queue.Ack(ctx, newer); err != nil {
					t.Fatal(err)
				}
				rows, err := g.DB().GetAll(ctx, `SELECT status,processed_at FROM hg_youban_publish_collect_event WHERE id IN (1,2) ORDER BY id`)
				if err != nil || len(rows) != 2 || rows[0]["status"].String() != "pending" || rows[1]["status"].String() != "media_ready" || !rows[0]["processed_at"].IsNil() || !rows[1]["processed_at"].IsNil() {
					t.Fatalf("wake changed business state: %v %v", rows, err)
				}
				execSQL(`UPDATE hg_youban_publish_collect_event SET status='processed',processed_at='2026-11-18 01:00:00' WHERE id=1`)
				execSQL(`UPDATE hg_youban_publish_collect_account_channel SET collect_enabled=0 WHERE id=1`)
				result := &sysin.AIOpsRepairApplyModel{}
				if err := s.applyAIOpsCollectWake(ctx, p, result); err != nil || result.Applied != 0 || len(result.Failed) != 0 {
					t.Fatalf("stale gates: %+v %v", result, err)
				}
				row, err := g.DB().GetOne(ctx, `SELECT status,processed_at FROM hg_youban_publish_collect_event WHERE id=1`)
				if err != nil || row["status"].String() != "processed" || row["processed_at"].IsNil() {
					t.Fatalf("completion reset: %v %v", row, err)
				}
			case "identity":
				p := plan(1, 2)
				execSQL(`UPDATE hg_youban_publish_collect_event SET source_unique_key='replacement' WHERE id=1`)
				// Mutable state changes do not invalidate event identity.
				execSQL(`UPDATE hg_youban_publish_collect_event SET status='failed' WHERE id=2`)
				result := &sysin.AIOpsRepairApplyModel{}
				if err := s.applyAIOpsCollectWake(ctx, p, result); err != nil || result.Applied != 1 || !reflect.DeepEqual(result.Failed, []int64{1}) {
					t.Fatalf("identity fence: %+v %v", result, err)
				}
				execSQL(`UPDATE hg_youban_publish_collect_event SET status='suspended' WHERE id=1`)
				blockedPlan := plan(1)
				execSQL(`UPDATE hg_youban_publish_collect_event SET status='pending' WHERE id=1`)
				result = &sysin.AIOpsRepairApplyModel{}
				if err := s.applyAIOpsCollectWake(ctx, blockedPlan, result); err != nil || result.Applied != 0 || len(result.Failed) != 0 {
					t.Fatalf("preview expanded: %+v %v", result, err)
				}
				// A previously completed event also cannot expand into a wake.
				execSQL(`UPDATE hg_youban_publish_collect_event SET status='processed' WHERE id=1`)
				terminalPlan := plan(1)
				execSQL(`UPDATE hg_youban_publish_collect_event SET status='pending' WHERE id=1`)
				result = &sysin.AIOpsRepairApplyModel{}
				if err := s.applyAIOpsCollectWake(ctx, terminalPlan, result); err != nil || result.Applied != 0 || len(result.Failed) != 0 {
					t.Fatalf("terminal preview expanded: %+v %v", result, err)
				}
				unsigned := plan(1)
				unsigned.CollectWakeEvents[0].Identity = ""
				result = &sysin.AIOpsRepairApplyModel{}
				if err := s.applyAIOpsCollectWake(ctx, unsigned, result); err != nil || result.Applied != 0 || !reflect.DeepEqual(result.Failed, []int64{1}) {
					t.Fatalf("unsigned event accepted: %+v %v", result, err)
				}
			case "queue_failure":
				p := plan(1, 2)
				// Poison only this fixture's sourcequeue key; authorization caches
				// remain readable, so failures happen in the real enqueue path.
				if err := s.applyAIOpsCollectWake(ctx, p, &sysin.AIOpsRepairApplyModel{}); err != nil {
					t.Fatal(err)
				}
				poisoned := false
				for _, key := range r.Keys() {
					if strings.Contains(key, ":history:source:78:351:") && strings.HasSuffix(key, ":payload") {
						r.Del(key)
						r.Set(key, "wrong-type")
						poisoned = true
					}
				}
				if !poisoned {
					t.Fatalf("queue payload key not found: %v", r.Keys())
				}
				result := &sysin.AIOpsRepairApplyModel{}
				if err := s.applyAIOpsCollectWake(ctx, p, result); err != nil || result.Applied != 0 || !reflect.DeepEqual(result.Failed, []int64{1, 2}) {
					t.Fatalf("queue failure IDs lost: %+v %v", result, err)
				}
			case "corrupt_plan":
				for _, change := range []string{"missing", "extra", "nil", "duplicate", "wrong_id", "duplicate_ids", "tenant", "owner", "source", "storage_negative", "storage_other", "decision"} {
					p := plan(1, 2)
					switch change {
					case "missing":
						p.CollectWakeEvents = p.CollectWakeEvents[:1]
					case "extra":
						p.CollectWakeEvents = append(p.CollectWakeEvents, preview(1)[0])
					case "nil":
						p.CollectWakeEvents[0] = nil
					case "duplicate":
						p.CollectWakeEvents[1] = p.CollectWakeEvents[0]
					case "wrong_id":
						p.CollectWakeEvents[0].EventId = 9999
					case "duplicate_ids":
						p.EventIDs = []int64{1, 1}
					case "tenant":
						p.CollectWakeEvents[1].TenantId = 79
					case "owner":
						p.CollectWakeEvents[1].OwnerAccountId = 616
					case "source":
						p.CollectWakeEvents[1].SourceId = 352
					case "storage_negative":
						p.CollectWakeEvents[1].AccountId = -1
					case "storage_other":
						p.CollectWakeEvents[1].AccountId = 616
					case "decision":
						p.CollectWakeEvents[1].Decision = "invalid"
					}
					result := &sysin.AIOpsRepairApplyModel{}
					if err := s.applyAIOpsCollectWake(ctx, p, result); err == nil || result.Applied != 0 {
						t.Fatalf("corrupt plan accepted (%s): %+v err=%v", change, result, err)
					}
					if claim, err := queue.Claim(ctx, sourcequeue.History, time.Minute); err != nil || claim != nil {
						t.Fatalf("corrupt plan partially queued (%s): %+v %v", change, claim, err)
					}
				}
			case "batch":
				for _, tc := range []struct {
					limit   int
					ids     []int64
					allowed bool
				}{
					{0, []int64{1, 2}, true}, {1, []int64{1, 1}, true}, {1, []int64{1, 2}, false},
					{2, []int64{1, 2, 1}, true}, {200, []int64{1, 2}, true},
					{-1, []int64{1}, false}, {201, []int64{1}, false},
				} {
					in := input(tc.ids...)
					in.Limit = tc.limit
					_, _, _, _, err := s.previewAIOpsCollectWake(ctx, in)
					if (err == nil) != tc.allowed {
						t.Fatalf("limit=%d ids=%v allowed=%v err=%v", tc.limit, tc.ids, tc.allowed, err)
					}
				}
				ids := []int64{1, 2}
				for id := int64(3); id <= 201; id++ {
					insert(id, "pending", "3763752459", 0)
					ids = append(ids, id)
				}
				if len(preview(ids[:200]...)) != 200 {
					t.Fatal("200 item batch rejected")
				}
				if _, _, _, _, err := s.previewAIOpsCollectWake(ctx, input(ids...)); err == nil {
					t.Fatal("201 item batch accepted")
				}
			}
			// No raw Collector, content/profile, media or TG Job tables exist.
			// Every successful apply above therefore exercises only the existing
			// business-event enqueue path, without synchronous material work.
			rows, err := g.DB().GetAll(ctx, `SELECT id,status,processed_at FROM hg_youban_publish_collect_event WHERE id IN (1,2) ORDER BY id`)
			if err != nil || len(rows) != 2 {
				t.Fatalf("business facts missing: %v %v", rows, err)
			}
			if scenario == "apply" && rows[1]["status"].String() != "media_ready" {
				t.Fatal("enqueue reset existing state")
			}
		})
	}
}

func TestAIOpsCollectWakeIdentityFields(t *testing.T) {
	event := gdb.Record{"id": gvar.New(1), "tenant_id": gvar.New(78), "account_id": gvar.New(0), "source_id": gvar.New(351), "source_chat_id": gvar.New("3763752459"), "source_message_id": gvar.New(1001), "source_unique_key": gvar.New("original"), "created_at": gvar.New("2026-11-18 00:00:00")}
	identity := collectWakeEventIdentity(event)
	if identity == "" {
		t.Fatal("identity missing")
	}
	for _, field := range []string{"id", "tenant_id", "account_id", "source_id", "source_chat_id", "source_message_id", "source_unique_key", "created_at"} {
		before := event[field]
		event[field] = gvar.New("999")
		if collectWakeEventIdentity(event) == identity {
			t.Fatalf("identity does not bind %s", field)
		}
		event[field] = before
	}
	for _, field := range []string{"status", "processed_at", "updated_at", "source_type", "source_grouped_id"} {
		event[field] = gvar.New("changed")
		if collectWakeEventIdentity(event) != identity {
			t.Fatalf("identity binds mutable/unselected field %s", field)
		}
	}
	delete(event, "created_at")
	if collectWakeEventIdentity(event) != "" {
		t.Fatal("incomplete query minted a valid identity")
	}
}
