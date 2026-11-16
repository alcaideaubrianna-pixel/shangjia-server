package sys

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/database/gredis"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/internal/library/cache"
)

func TestAccountCycleIsolationLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_CYCLE_ISOLATION_LOCAL") != "1" {
		t.Skip("requires disposable localhost PostgreSQL port 55579")
	}
	before, _ := gdb.GetConfigGroup("default")
	old := g.Cfg().GetAdapter()
	redisBefore, _ := gredis.GetConfig()
	t.Cleanup(func() { gdb.SetConfigGroup("default", before); g.Cfg().SetAdapter(old); gredis.SetConfig(redisBefore) })
	node := gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "55579", User: "cycle_test", Name: "postgres"}
	bootstrap, err := gdb.New(node)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("cycle_isolation_%d", time.Now().UnixNano())
	if _, err = bootstrap.Exec(context.Background(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = bootstrap.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
	node.Namespace = schema
	gdb.SetConfigGroup("default", gdb.ConfigGroup{node})
	redis := miniredis.RunT(t)
	a, err := gcfg.NewAdapterContent(fmt.Sprintf(`{"cache":{"adapter":"redis"},"redis":{"default":{"address":%q}}}`, redis.Addr()))
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(a)
	gredis.SetConfig(&gredis.Config{Address: redis.Addr()})
	cache.SetAdapter(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = func(ctx context.Context, tx gdb.DB) error {
		ddls := []string{
			"CREATE TABLE " + publishChannelProfileTable + "(id bigint,tenant_id bigint,account_id bigint,channel_id bigint,profile_id bigint,last_job_id bigint,status text,next_cycle_at timestamp,cycle_generation int default 0,applied_cycle_config_version bigint default 0)",
			"CREATE TABLE " + publishTgJobTable + "(id bigint,tenant_id bigint,account_id bigint,channel_id bigint,profile_id bigint,status text,sent_at timestamp,cycle_days int default 3,cycle_publish_time text default '',cycle_failure_count int default 0,operation_no text default '')",
			"CREATE TABLE " + publishChannelTable + "(id bigint,tenant_id bigint,status int,cycle_publish_enabled int,cycle_publish_mode text,publish_direction text,deleted_at timestamp,cycle_config_version bigint)",
			"CREATE TABLE hg_content_profile(id bigint,status int,visibility text,deleted_at timestamp)",
			"CREATE TABLE " + channelPermissionCircuitTable + "(tenant_id bigint,channel_id bigint,paused int)",
			"CREATE TABLE " + publishProfileCycleRunTable + "(id bigint,tenant_id bigint,account_id bigint,profile_id bigint,status text)",
			"CREATE TABLE " + publishProfileCycleRunChannelTable + "(run_id bigint,channel_id bigint,status text)",
			"CREATE INDEX ON " + publishChannelProfileTable + "(tenant_id,account_id,next_cycle_at,id) WHERE status='active' AND next_cycle_at IS NOT NULL",
			"INSERT INTO " + publishChannelTable + " VALUES(231,78,1,1,'time','up',NULL,1)",
			"INSERT INTO hg_content_profile SELECT i,CASE WHEN i<=80 THEN 2 ELSE 1 END,'public',NULL FROM generate_series(1,85)i",
			"INSERT INTO " + publishTgJobTable + "(id,tenant_id,account_id,channel_id,profile_id,status,sent_at) SELECT i,78,615,231,i,'sent',now()-interval '5 days' FROM generate_series(1,85)i",
			"INSERT INTO " + publishChannelProfileTable + "(id,tenant_id,account_id,channel_id,profile_id,last_job_id,status,next_cycle_at) SELECT i,78,615,231,i,i,'active',now()-interval '1 day'+i*interval '1 second' FROM generate_series(1,85)i",
		}
		for _, ddl := range ddls {
			if _, e := tx.Exec(ctx, ddl); e != nil {
				return e
			}
		}
		s := new(sSysPublish)
		scope := publishAccountScope{TenantID: 78, AccountID: 615}
		for i := 1000; i < 1200; i++ {
			indexProfileCycleDue(ctx, int64(i), gtime.Now().Add(-time.Hour))
		}
		rows, e := s.profileCycleDueRowsShard(ctx, 20, -1, scope)
		if e != nil {
			return e
		}
		if len(rows) != 0 || !accountProfileCycleCandidatesMore(ctx, scope) {
			t.Fatalf("bounded invalid scan must request continuation: rows=%d", len(rows))
		}
		rows, e = s.profileCycleDueRowsShard(ctx, 20, -1, scope)
		if e != nil {
			return e
		}
		if len(rows) != 5 {
			t.Fatalf("global index or invalid first 80 starved account: got %d", len(rows))
		}
		if !redis.Exists(accountProfileCycleDueKey(scope)) {
			t.Fatal("account index not populated")
		}
		if _, e = tx.Exec(ctx, "UPDATE "+publishProfileCycleRunTable+" SET status='pending'"); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO "+publishProfileCycleRunTable+" VALUES(1,78,615,81,'pending')"); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO "+publishProfileCycleRunChannelTable+" VALUES(1,231,'pending')"); e != nil {
			return e
		}
		rows, e = s.profileCycleDueRowsShard(ctx, 20, -1, scope)
		if e != nil {
			return e
		}
		for _, r := range rows {
			if r.ProfileId == 81 {
				t.Fatal("run barrier bypassed")
			}
		}
		// Repair existing mapping, then verify a new plan and inactive mapping survive repeated repair.
		if _, e = tx.Exec(ctx, "UPDATE "+publishChannelProfileTable+" SET status='inactive' WHERE id<>82"); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "UPDATE "+publishChannelProfileTable+" SET next_cycle_at=NULL WHERE id=82"); e != nil {
			return e
		}
		config := profileCycleChannelConfig{Id: 231, TenantId: 78, Enabled: 1, Mode: "time", Days: 3, Status: 1, Direction: "up", Version: 1}
		if e = cache.Instance().Set(ctx, profileCycleChannelConfigCacheKey(231), config, time.Minute); e != nil {
			return e
		}
		if e = markAccountCycleRepairChannel(ctx, scope, 231); e != nil {
			return e
		}
		if e = s.repairAccountCyclePlans(ctx, scope); e != nil {
			return e
		}
		v, e := tx.GetValue(ctx, "SELECT next_cycle_at FROM "+publishChannelProfileTable+" WHERE id=82")
		if e != nil {
			return e
		}
		if v.IsNil() {
			t.Fatal("existing empty plan was not repaired")
		}
		if _, e = tx.Exec(ctx, "UPDATE "+publishChannelProfileTable+" SET next_cycle_at='2030-01-01' WHERE id=82"); e != nil {
			return e
		}
		_, _ = cache.Instance().Remove(ctx, "youban_publish:cycle:repair:78:615")
		if e = s.repairAccountCyclePlans(ctx, scope); e != nil {
			return e
		}
		v, e = tx.GetValue(ctx, "SELECT next_cycle_at::text FROM "+publishChannelProfileTable+" WHERE id=82")
		if e != nil {
			return e
		}
		if v.String() != "2030-01-01 00:00:00" {
			t.Fatalf("new plan overwritten: %s", v.String())
		}
		// Pause a real PG update so a mark lands after the repair's version snapshot
		// and before its completion CAS. No timing-only sleep decides the ordering.
		dirtyKey := accountProfileCycleDueKey(scope) + ":repair_channels"
		repairKey := "youban_publish:cycle:repair:78:615"
		if _, e = tx.Exec(ctx, "UPDATE "+publishChannelProfileTable+" SET next_cycle_at=NULL WHERE id=82"); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "CREATE FUNCTION repair_pause() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(1); RETURN NEW; END $$"); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "CREATE TRIGGER repair_pause BEFORE UPDATE ON "+publishChannelProfileTable+" FOR EACH ROW EXECUTE FUNCTION repair_pause()"); e != nil {
			return e
		}
		if e = markAccountCycleRepairChannel(ctx, scope, 231); e != nil {
			return e
		}
		_, _ = cache.Instance().Remove(ctx, repairKey)
		finished := make(chan error, 1)
		go func() { finished <- s.repairAccountCyclePlans(ctx, scope) }()
		observed := false
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			waiting, queryErr := bootstrap.GetValue(ctx, "SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE wait_event='PgSleep' AND query LIKE ?)", "%"+publishChannelProfileTable+"%")
			if queryErr != nil {
				return queryErr
			}
			if waiting.Bool() {
				observed = true
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if !observed {
			<-finished
			t.Fatal("did not observe repair waiting inside PG update")
		}
		if e = markAccountCycleRepairChannel(ctx, scope, 231); e != nil {
			return e
		}
		if e = <-finished; e != nil {
			return e
		}
		if !redis.Exists(dirtyKey) || redis.HGet(dirtyKey+":versions", "231") != "2" {
			t.Fatal("completion swallowed the concurrent mark/version")
		}
		if _, e = tx.Exec(ctx, "DROP TRIGGER repair_pause ON "+publishChannelProfileTable); e != nil {
			return e
		}
		// A new version must ignore a stale high mapping cursor and revisit id 82.
		if _, e = tx.Exec(ctx, "UPDATE "+publishChannelProfileTable+" SET next_cycle_at=NULL WHERE id=82"); e != nil {
			return e
		}
		if e = cache.Instance().Set(ctx, repairKey, g.Map{"Cursor": 99999, "Channel": 231, "Version": 1, "NextAt": 0}, time.Minute); e != nil {
			return e
		}
		if e = s.repairAccountCyclePlans(ctx, scope); e != nil {
			return e
		}
		v, e = tx.GetValue(ctx, "SELECT next_cycle_at FROM "+publishChannelProfileTable+" WHERE id=82")
		if e != nil {
			return e
		}
		if v.IsNil() {
			t.Fatal("new dirty version did not reset the stale mapping cursor")
		}
		if redis.Exists(dirtyKey) || redis.HGet(dirtyKey+":versions", "231") != "" {
			t.Fatal("same-version successful completion must clear dirty member/version")
		}
		return nil
	}(ctx, g.DB())
	if err != nil {
		t.Fatal(err)
	}
}
