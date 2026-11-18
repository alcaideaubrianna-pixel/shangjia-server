package sys

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/sqlite/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gtime"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

type aiOpsReplayAsyncTestCollector struct {
	collectorservice.ICollector
	started chan context.Context
	release chan struct{}
	calls   atomic.Int32
}

func (f *aiOpsReplayAsyncTestCollector) BotUpdateReplayCandidates(context.Context, int64, []int64) ([]*collectorservice.BotUpdateReplayCandidate, error) {
	// Authorization still reads the actual source and cached VIP below. This
	// fake supplies no channel candidates: these tests target async lifecycle.
	return nil, nil
}

func (f *aiOpsReplayAsyncTestCollector) ReplayBotUpdatesForSource(ctx context.Context, tenantID, sourceID int64, ids []int64) (*collectorservice.BotUpdateReplayResult, error) {
	f.calls.Add(1)
	f.started <- ctx
	select {
	case <-f.release:
		return &collectorservice.BotUpdateReplayResult{Queued: len(ids), Failed: []int64{}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestAIOpsSourceReplayAsyncLifecycle(t *testing.T) {
	const childEnv = "YOUBAN_REPLAY_ASYNC_TEST_CHILD"
	if os.Getenv(childEnv) != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestAIOpsSourceReplayAsyncLifecycle$", "-test.count=1", "-test.timeout=30s")
		cmd.Env = append(os.Environ(), childEnv+"=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated replay lifecycle: %v\n%s", err, output)
		}
		return
	}
	ctx := context.Background()
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "sqlite", Name: filepath.Join(t.TempDir(), "replay.db")}})
	adapter, err := gcfg.NewAdapterContent("cache:\n  adapter: memory\n")
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(adapter)
	cache.SetAdapter(ctx)
	for key, value := range map[string]interface{}{
		collectConfigCacheKey: &sysin.CollectConfigModel{Enabled: 1},
		tenantVipCacheKey(78): &sysin.TenantVipStatusModel{TenantId: 78, IsVip: true, Features: []string{sysin.TenantVipFeatureCollectSource}},
	} {
		if err := cache.Instance().Set(ctx, key, value, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	execSQL := func(sql string, args ...interface{}) {
		t.Helper()
		if _, err := g.DB().Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	execSQL(`CREATE TABLE hg_youban_publish_ai_ops_repair_plan(plan_id text PRIMARY KEY,action text,payload_json text,status text,applied_count integer,failed_ids_json text,confirm_token_hash text,error_message text,expires_at datetime,started_at datetime,finished_at datetime,updated_at datetime)`)
	execSQL(`CREATE TABLE hg_youban_publish_collect_source(id integer PRIMARY KEY,tenant_id bigint,account_id bigint,bot_id bigint,source_type text,collect_enabled integer,status integer,deleted_at datetime)`)
	execSQL(`INSERT INTO hg_youban_publish_collect_source VALUES(351,78,469,5,'bot',1,1,NULL)`)
	s := &sSysPublish{}
	newPlan := func(id, status, action string) *sysin.AIOpsRepairApplyInp {
		t.Helper()
		payload := aiOpsRepairPlanPayload{Action: action, SourceID: 351, TenantID: 78, AccountID: 469, CollectorEventIDs: []int64{1, 2}}
		execSQL(`INSERT INTO hg_youban_publish_ai_ops_repair_plan(plan_id,action,payload_json,status,confirm_token_hash,expires_at) VALUES(?,?,?,?,?,?)`, id, action, mustJSONString(payload), status, collectHash("test-confirm"), gtime.Now().Add(time.Hour))
		return &sysin.AIOpsRepairApplyInp{PlanId: id, ConfirmToken: "test-confirm"}
	}
	readPlan := func(id string) gdb.Record {
		t.Helper()
		row, err := g.DB().GetOne(ctx, `SELECT * FROM hg_youban_publish_ai_ops_repair_plan WHERE plan_id=?`, id)
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	t.Run("busy_does_not_claim", func(t *testing.T) {
		in := newPlan("busy", "pending", "replay_bot_source_updates")
		aiOpsSourceReplaySlots <- struct{}{}
		aiOpsSourceReplaySlots <- struct{}{}
		defer func() { <-aiOpsSourceReplaySlots; <-aiOpsSourceReplaySlots }()
		if _, err := s.AIOpsApplyRepairPlan(ctx, in); err == nil || !strings.Contains(err.Error(), "繁忙") {
			t.Fatalf("expected busy: %v", err)
		}
		row := readPlan(in.PlanId)
		if row["status"].String() != "pending" || !row["started_at"].IsEmpty() {
			t.Fatalf("busy claimed plan: %v", row)
		}
	})
	t.Run("canceled_apply_does_not_claim_or_leak_slot", func(t *testing.T) {
		in := newPlan("canceled-apply", "pending", "replay_bot_source_updates")
		requestCtx, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := s.AIOpsApplyRepairPlan(requestCtx, in); err == nil {
			t.Fatal("canceled apply unexpectedly succeeded")
		}
		if readPlan(in.PlanId)["status"].String() != "pending" || len(aiOpsSourceReplaySlots) != 0 {
			t.Fatal("canceled apply claimed plan or leaked admission")
		}
	})
	t.Run("claim_error_releases_slot", func(t *testing.T) {
		in := newPlan("claim-error", "pending", "replay_bot_source_updates")
		execSQL(`CREATE TRIGGER deny_claim BEFORE UPDATE ON hg_youban_publish_ai_ops_repair_plan WHEN NEW.plan_id='claim-error' BEGIN SELECT RAISE(ABORT,'claim failure'); END`)
		defer execSQL(`DROP TRIGGER deny_claim`)
		if _, err := s.AIOpsApplyRepairPlan(ctx, in); err == nil {
			t.Fatal("claim failure was ignored")
		}
		if len(aiOpsSourceReplaySlots) != 0 || readPlan(in.PlanId)["status"].String() != "pending" {
			t.Fatal("failed claim leaked slot or changed pending")
		}
	})
	t.Run("apply_returns_running_request_cancel_does_not_cancel_worker", func(t *testing.T) {
		in := newPlan("async", "pending", "replay_bot_source_updates")
		fake := &aiOpsReplayAsyncTestCollector{ICollector: collectorservice.Collector(), started: make(chan context.Context, 1), release: make(chan struct{})}
		collectorservice.RegisterCollector(fake)
		defer collectorservice.RegisterCollector(fake.ICollector)
		defer func() {
			select {
			case <-fake.release:
			default:
				close(fake.release)
			}
		}()
		requestCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		result, err := s.AIOpsApplyRepairPlan(requestCtx, in)
		if err != nil || result.Status != "running" {
			t.Fatalf("apply must immediately return running: result=%v err=%v", result, err)
		}
		cancel()
		select {
		case workerCtx := <-fake.started:
			if workerCtx.Err() != nil {
				t.Fatalf("request canceled worker: %v", workerCtx.Err())
			}
			deadline, ok := workerCtx.Deadline()
			if !ok || time.Until(deadline) <= 4*time.Minute || time.Until(deadline) > 5*time.Minute {
				t.Fatalf("worker missing independent five-minute bound: %v", deadline)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("background collector did not start; plan=%v", readPlan(in.PlanId))
		}
		if readPlan(in.PlanId)["status"].String() != "running" || len(aiOpsSourceReplaySlots) != 1 {
			t.Fatal("blocked worker lost durable running state or slot")
		}
		if _, err := s.AIOpsApplyRepairPlan(ctx, in); err == nil {
			t.Fatal("running plan was automatically replayed")
		}
		close(fake.release)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if readPlan(in.PlanId)["status"].String() != "running" && len(aiOpsSourceReplaySlots) == 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		row := readPlan(in.PlanId)
		if row["status"].String() != "completed" || row["applied_count"].Int() != 2 || row["finished_at"].IsEmpty() || len(aiOpsSourceReplaySlots) != 0 || fake.calls.Load() != 1 {
			t.Fatalf("async result not durably completed once: %v calls=%d slots=%d", row, fake.calls.Load(), len(aiOpsSourceReplaySlots))
		}
	})
	t.Run("canceled_execution_still_persists_failure", func(t *testing.T) {
		in := newPlan("canceled-worker", "running", "replay_bot_source_updates")
		workerCtx, cancel := context.WithCancel(ctx)
		runAIOpsSourceReplayPlan(workerCtx, in.PlanId, func(ctx context.Context, result *sysin.AIOpsRepairApplyModel) error {
			result.Applied = 1
			cancel()
			return ctx.Err()
		})
		row := readPlan(in.PlanId)
		if row["status"].String() != "failed" || row["applied_count"].Int() != 1 || !strings.Contains(row["error_message"].String(), "context canceled") || row["finished_at"].IsEmpty() {
			t.Fatalf("canceled execution prevented terminal write: %v", row)
		}
	})
	t.Run("panic_is_recorded_failed", func(t *testing.T) {
		in := newPlan("panic", "running", "replay_bot_source_updates")
		runAIOpsSourceReplayPlan(ctx, in.PlanId, func(context.Context, *sysin.AIOpsRepairApplyModel) error { panic("replay panic") })
		row := readPlan(in.PlanId)
		if row["status"].String() != "failed" || !strings.Contains(row["error_message"].String(), "replay panic") || row["finished_at"].IsEmpty() {
			t.Fatalf("panic was not recorded: %v", row)
		}
	})
	t.Run("failed_result_write_stays_running_and_never_replays", func(t *testing.T) {
		in := newPlan("unknown-result", "running", "replay_bot_source_updates")
		execSQL(`CREATE TRIGGER deny_result BEFORE UPDATE ON hg_youban_publish_ai_ops_repair_plan WHEN NEW.plan_id='unknown-result' BEGIN SELECT RAISE(ABORT,'result write failure'); END`)
		defer execSQL(`DROP TRIGGER deny_result`)
		calls := 0
		runAIOpsSourceReplayPlan(ctx, in.PlanId, func(context.Context, *sysin.AIOpsRepairApplyModel) error {
			calls++
			return errors.New("uncertain replay result")
		})
		if row := readPlan(in.PlanId); row["status"].String() != "running" || !row["finished_at"].IsEmpty() || calls != 1 {
			t.Fatalf("unknown outcome falsely completed or retried: %v calls=%d", row, calls)
		}
		if _, err := s.AIOpsApplyRepairPlan(ctx, in); err == nil {
			t.Fatal("unknown running plan accepted for replay")
		}
	})
	t.Run("unknown_action_and_unknown_status_never_claim", func(t *testing.T) {
		for _, tc := range []struct{ id, status, action string }{
			{"unknown-action", "pending", "unknown"},
			{"unknown-status", "unknown", "replay_bot_source_updates"},
		} {
			in := newPlan(tc.id, tc.status, tc.action)
			if _, err := s.AIOpsApplyRepairPlan(ctx, in); err == nil || readPlan(in.PlanId)["status"].String() != tc.status || len(aiOpsSourceReplaySlots) != 0 {
				t.Fatalf("unknown plan was claimed: %s err=%v", tc.id, err)
			}
		}
	})
}
