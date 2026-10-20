package sys

import (
	"context"
	"errors"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
)

// Real PostgreSQL, private transaction fixtures; never starts a Telegram worker.
func TestCollectSourceDownHistoricalLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SOURCE_DOWN_LOCAL_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL 35432")
	}
	before := gdb.GetConfig("default")
	oldAdapter := g.Cfg().GetAdapter()
	t.Cleanup(func() { gdb.SetConfigGroup("default", before); g.Cfg().SetAdapter(oldAdapter) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	setGate := func(enabled bool) {
		content := `{"youbanPublish":{"profileOriginEnabled":false}}`
		if enabled {
			content = `{"youbanPublish":{"profileOriginEnabled":true}}`
		}
		a, err := gcfg.NewAdapterContent(content)
		if err != nil {
			t.Fatal(err)
		}
		g.Cfg().SetAdapter(a)
	}
	setGate(false)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_channel_profile(tenant_id bigint,account_id bigint,source_id bigint,rule_id bigint,channel_id text,profile_id bigint,deleted_at timestamp,created_at timestamp,updated_at timestamp,UNIQUE(tenant_id,account_id,rule_id,channel_id,profile_id)) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_content_profile(id bigint PRIMARY KEY,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_state(profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_content_profile SELECT n,NULL FROM generate_series(1,100) n`,
			`INSERT INTO hg_youban_publish_profile_state SELECT n,56,561,NULL FROM generate_series(1,100) n`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch(tenant_id bigint,account_id bigint,source_id bigint,rule_id bigint,profile_id bigint,event_id bigint,source_chat_id_snapshot text,status text,error_message text,finished_at timestamp,updated_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_event(id bigint,tenant_id bigint,account_id bigint,source_chat_id text) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_job(tenant_id bigint,account_id bigint,profile_id bigint,channel_id bigint,collect_source_chat_id text,collect_event_id bigint DEFAULT 99,id bigserial PRIMARY KEY,status text DEFAULT 'pending') ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_source(id bigint,tenant_id bigint,account_id bigint,source_chat_id text,deleted_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_source VALUES(348,56,561,'4439615649',NOW()),(355,56,561,'-1004439615649',NULL)`,
			`INSERT INTO hg_youban_publish_collect_channel_profile(tenant_id,account_id,source_id,rule_id,channel_id,profile_id,deleted_at) VALUES(56,561,355,335,'4439615649',1,NULL),(56,562,355,335,'4439615649',91,NULL),(57,561,355,335,'4439615649',92,NULL)`,
			`INSERT INTO hg_youban_publish_collect_event VALUES(20,56,561,'-1004439615649'),(21,57,561,'4439615649')`,
			`INSERT INTO hg_youban_publish_collect_dispatch(tenant_id,account_id,source_id,rule_id,profile_id,event_id,source_chat_id_snapshot) VALUES(56,561,348,335,2,20,''),(56,561,348,335,3,99,'4439615649'),(56,561,348,336,4,99,'4439615649'),(56,561,348,335,5,99,''),(56,561,348,335,6,99,''),(56,561,348,335,7,21,''),(56,562,348,335,93,20,'4439615649'),(57,561,348,335,94,20,'4439615649')`,
			`INSERT INTO hg_youban_publish_tg_job(tenant_id,account_id,profile_id,channel_id,collect_source_chat_id) VALUES(56,561,5,123,'-1004439615649'),(56,561,6,4439615649,'9999'),(56,561,8,123,'4439615649'),(57,561,95,123,'4439615649')`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		assertIDs := func(got []int64, want []int64) {
			t.Helper()
			sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v want %v", got, want)
			}
		}
		// Partial compact index must not hide historical dispatch snapshots.
		ids, err := collectChannelProfileIDs(ctx, 56, 561, 335, "-1004439615649")
		if err != nil {
			return err
		}
		assertIDs(ids, []int64{1, 2, 3, 5})
		counts, countErr := collectChannelProfileCounts(ctx, 56, 561, 335, []string{"4439615649", "-1004439615649", "9999"})
		if countErr != nil {
			return countErr
		}
		if counts["4439615649"] != len(ids) || counts["9999"] != 1 {
			t.Fatalf("page count differs from purge scope: %v", counts)
		}
		// Same source chat across replacement configurations, with no rule scope.
		ids, err = collectChannelProfileIDs(ctx, 56, 561, 0, "4439615649")
		if err != nil {
			return err
		}
		assertIDs(ids, []int64{1, 2, 3, 4, 5, 8})
		ids, err = (&sSysPublish{}).collectSourceDownProfileIds(ctx, 355, 56, 561)
		if err != nil {
			return err
		}
		assertIDs(ids, []int64{1, 2, 3, 4, 5, 8})
		// Retry/rebuild remains strictly source-ID scoped.
		ids, err = (&sSysPublish{}).collectSourceProfileIds(ctx, 355, 56, 561)
		if err != nil {
			return err
		}
		if len(ids) != 0 {
			t.Fatalf("source retry widened: %v", ids)
		}
		if _, err := tx.Exec(`CREATE TEMP TABLE hg_youban_publish_profile_origin(tenant_id bigint,account_id bigint,profile_id bigint,origin_kind text,chat_id text,event_id bigint DEFAULT 99) ON COMMIT DROP`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_profile_origin(tenant_id,account_id,profile_id,origin_kind,chat_id) VALUES(56,561,9,'collect','-1004439615649'),(56,561,6,'collect','-1004439615649'),(56,562,96,'collect','-1004439615649')`); err != nil {
			return err
		}
		setGate(true)
		ids, err = collectChannelProfileIDs(ctx, 56, 561, 335, "4439615649")
		if err != nil {
			return err
		}
		assertIDs(ids, []int64{1, 2, 3, 5, 6})
		ids, err = collectChannelProfileIDs(ctx, 56, 561, 0, "4439615649")
		if err != nil {
			return err
		}
		assertIDs(ids, []int64{1, 2, 3, 4, 5, 6, 8, 9})
		// An origin from another event must not inherit an unrelated rule merely
		// because it shares the profile ID.
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_profile_origin VALUES(56,561,2,'collect','-1007777',9999)`); err != nil {
			return err
		}
		otherIDs, err := collectChannelProfileIDs(ctx, 56, 561, 335, "7777")
		if err != nil {
			return err
		}
		if len(otherIDs) != 0 {
			t.Fatalf("unrelated event inherited a rule: %v", otherIDs)
		}
		setGate(false)
		// Rule ownership is atomic with material persistence and rolls back on
		// failure, even with permanent origins disabled for a staged rollout.
		rollback := errors.New("rollback origin fixture")
		rollbackErr := g.DB().Transaction(ctx, func(nestedCtx context.Context, nested gdb.TX) error {
			if err := saveCollectMaterialOriginTx(nestedCtx, nested, 10, gdb.Record{"tenant_id": g.NewVar(56), "account_id": g.NewVar(561), "source_id": g.NewVar(348), "source_chat_id": g.NewVar("4439615649")}, gdb.Record{"id": g.NewVar(335)}); err != nil {
				return err
			}
			return rollback
		})
		if !errors.Is(rollbackErr, rollback) {
			return rollbackErr
		}
		row, err := tx.GetOne(`SELECT count(*) AS n FROM hg_youban_publish_collect_channel_profile WHERE profile_id=10`)
		if err != nil {
			return err
		}
		if row["n"].Int() != 0 {
			t.Fatal("rolled-back material left a rule mapping")
		}
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_rule(id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_rule VALUES(335,56,561,NULL),(336,56,561,NULL)`,
			`CREATE TEMP TABLE hg_youban_publish_intro_fee_channel_rule(id bigserial PRIMARY KEY,tenant_id bigint,account_id bigint,rule_id bigint,bot_id bigint,channel_id text,suffix text,status int,deleted_at timestamp,created_at timestamp,updated_at timestamp) ON COMMIT DROP`,
			`CREATE UNIQUE INDEX gate_fixture_unique ON hg_youban_publish_intro_fee_channel_rule(tenant_id,account_id,rule_id,bot_id,channel_id) WHERE deleted_at IS NULL`,
			`INSERT INTO hg_youban_publish_intro_fee_channel_rule(tenant_id,account_id,rule_id,bot_id,channel_id,suffix,status) VALUES(56,561,335,0,'4439615649','A',1),(56,561,336,0,'4439615649','B',1)`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		if err := (&sSysPublish{}).finishCollectChannelPurge(ctx, 56, 561, 335, 0, "4439615649"); err != nil {
			return err
		}
		if err := (&sSysPublish{}).finishCollectChannelPurge(ctx, 56, 561, 335, 0, "-1004439615649"); err != nil {
			return err
		}
		for _, tc := range []struct {
			tenant, account, rule int64
			want                  bool
		}{{56, 561, 335, true}, {56, 561, 336, false}, {56, 562, 335, false}, {57, 561, 335, false}} {
			disabled, err := collectRuleChannelDisabled(ctx, tc.tenant, tc.account, tc.rule, 43, "-1004439615649")
			if err != nil {
				return err
			}
			if disabled != tc.want {
				t.Fatalf("gate scope changed: %+v got %v", tc, disabled)
			}
		}
		row, err = tx.GetOne(`SELECT suffix,status FROM hg_youban_publish_intro_fee_channel_rule WHERE rule_id=336`)
		if err != nil {
			return err
		}
		if row["suffix"].String() != "B" || row["status"].Int() != 1 {
			t.Fatal("other rule configuration changed")
		}
		if err := validateCollectRulePurgeProfileScope(ctx, 56, 561, 335, []int64{2, 3}); err != nil {
			return err
		}
		if err := validateCollectRulePurgeProfileScope(ctx, 56, 561, 335, []int64{4}); err == nil {
			t.Fatal("shared/foreign rule profiles accepted")
		}
		row, err = tx.GetOne(`SELECT count(*) AS n FROM hg_youban_publish_collect_channel_profile WHERE profile_id=1`)
		if err != nil {
			return err
		}
		if row["n"].Int() != 1 {
			t.Fatal("configuration cleanup erased permanent rule evidence")
		}
		setGate(true)
		if _, err := tx.Exec(`DELETE FROM hg_youban_publish_collect_channel_profile`); err != nil {
			return err
		}
		ids, err = collectChannelProfileIDs(ctx, 56, 561, 335, "-1004439615649")
		if err != nil {
			return err
		}
		assertIDs(ids, []int64{2, 3, 5, 6})
		// Once a profile is taken offline, historical dispatches must not revive
		// it merely because they use a replaced source configuration ID.
		if err := (&sSysPublish{}).finishCollectSourceDownDispatch(ctx, []int64{2, 3}, 56, 561); err != nil {
			return err
		}
		row, err = tx.GetOne(`SELECT count(*) AS n FROM hg_youban_publish_collect_dispatch WHERE tenant_id=56 AND account_id=561 AND status='skipped'`)
		if err != nil {
			return err
		}
		if row["n"].Int() != 2 {
			t.Fatalf("historical dispatch retries remain: %v", row)
		}
		// Deleted profiles disappear from both counts and selection; their job
		// IDs remain in the operation snapshot for asynchronous message cleanup.
		if _, err := tx.Exec(`UPDATE hg_content_profile SET deleted_at=NOW() WHERE id=3`); err != nil {
			return err
		}
		ids, err = collectChannelProfileIDs(ctx, 56, 561, 335, "4439615649")
		if err != nil {
			return err
		}
		counts, err = collectChannelProfileCounts(ctx, 56, 561, 335, []string{"4439615649"})
		if err != nil {
			return err
		}
		if counts["4439615649"] != len(ids) {
			t.Fatal("deleted profile count diverged")
		}
		if os.Getenv("YOUBAN_SOURCE_DOWN_LOAD_PG") == "1" {
			setGate(false)
			for _, sql := range []string{
				`INSERT INTO hg_content_profile SELECT n,NULL FROM generate_series(100001,200000) n`,
				`INSERT INTO hg_youban_publish_profile_state SELECT n,56,561,NULL FROM generate_series(100001,200000) n`,
				`INSERT INTO hg_youban_publish_collect_dispatch(tenant_id,account_id,source_id,rule_id,profile_id,event_id,source_chat_id_snapshot) SELECT 56,561,348,335,n,n,'987654321' FROM generate_series(100001,200000) n`,
				`CREATE INDEX source_down_fixture_dispatch ON hg_youban_publish_collect_dispatch(tenant_id,account_id,rule_id,profile_id)`,
				`CREATE INDEX source_down_fixture_state ON hg_youban_publish_profile_state(tenant_id,account_id,profile_id)`,
				`ANALYZE hg_youban_publish_collect_dispatch`,
				`ANALYZE hg_content_profile`,
				`ANALYZE hg_youban_publish_profile_state`,
			} {
				if _, err := tx.Exec(sql); err != nil {
					return err
				}
			}
			started := time.Now()
			counts, err = collectChannelProfileCounts(ctx, 56, 561, 335, []string{"987654321", "4439615649"})
			if err != nil {
				return err
			}
			if counts["987654321"] != 100000 {
				t.Fatalf("large page count: %v", counts)
			}
			t.Logf("100k history aggregate: %s", time.Since(started))
			ids, err = collectChannelProfileIDs(ctx, 56, 561, 335, "987654321")
			if err != nil {
				return err
			}
			if len(ids) != 100000 {
				t.Fatalf("large selector: %d", len(ids))
			}
			started = time.Now()
			jobs, err := (&sSysPublish{}).collectChannelPurgeJobs(ctx, 56, 561, ids)
			if err != nil {
				return err
			}
			if len(jobs) != 0 {
				t.Fatal("source and destination IDs mixed")
			}
			t.Logf("100k IDs through 200-ID job batches: %s", time.Since(started))
			if pendingJobs, err := (&sSysPublish{}).collectSourcePendingDownJobs(ctx, ids, 56); err != nil {
				return err
			} else if len(pendingJobs) != 0 {
				t.Fatal("source-down pending scope widened")
			}
			if sourceJobs, err := (&sSysPublish{}).collectSourceDownJobs(ctx, ids, 56); err != nil {
				return err
			} else if len(sourceJobs) != 0 {
				t.Fatal("source-down job scope widened")
			}
		}
		// More than one message-count batch, including a blocked cleanup.
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_youban_publish_tg_message(job_id bigint,status text) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_tg_job(id,tenant_id,account_id,profile_id) SELECT n,56,561,999 FROM generate_series(10001,10600) n`,
			`INSERT INTO hg_youban_publish_tg_message SELECT n,CASE WHEN n<=10300 THEN 'sent' ELSE 'undeletable' END FROM generate_series(10001,10600) n`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		jobIDs := make([]int64, 600)
		for i := range jobIDs {
			jobIDs[i] = 10001 + int64(i)
		}
		pending, blocked, err := (&sSysPublish{}).collectChannelPurgeRemainingMessages(ctx, 56, jobIDs)
		if err != nil {
			return err
		}
		if pending != 300 || blocked != 300 {
			t.Fatalf("batched cleanup totals: %d/%d", pending, blocked)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
