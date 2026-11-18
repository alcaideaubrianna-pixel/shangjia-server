package sys

import (
	"context"
	"os"
	"testing"
	"time"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcache"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gtime"
)

// No production connection is accepted: every table is transaction-local.
func TestProfileCycleOverrideBoundariesPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_BATCH_ROTATION_TEST_PG_PORT")
	if port == "" {
		t.Skip("set YOUBAN_BATCH_ROTATION_TEST_PG_PORT for disposable localhost PG")
	}
	if port != "55491" {
		t.Fatal("isolated test requires explicit localhost port 55491")
	}
	previousDB, _ := gdb.GetConfigGroup("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previousDB) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{
		Type: "pgsql", Host: "127.0.0.1", Port: port, User: "postgres", Name: "postgres",
	}})
	previousConfig := g.Cfg().GetAdapter()
	testConfig, err := gcfg.NewAdapterContent(`{"cache":{"adapter":"memory"},"system":{"mode":"production"},"youbanPublish":{"cycle":{"freeIntervalDays":15}}}`)
	if err != nil {
		t.Fatal(err)
	}
	g.Cfg().SetAdapter(testConfig)
	t.Cleanup(func() { g.Cfg().SetAdapter(previousConfig) })
	if !cache.Initialized() {
		cache.SetAdapter(context.Background())
	}
	previousCache := cache.Instance().GetAdapter()
	cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	t.Cleanup(func() {
		_, _ = cache.Instance().Remove(context.Background(), tenantVipCacheKey(1))
		cache.Instance().SetAdapter(previousCache)
	})
	setVip := func(expiredAt *gtime.Time) {
		t.Helper()
		if err := cache.Instance().Set(context.Background(), tenantVipCacheKey(1), &sysin.TenantVipStatusModel{
			TenantId: 1, IsVip: true, Status: 1, ExpiredAt: expiredAt,
		}, time.Hour); err != nil {
			t.Fatal(err)
		}
	}
	setVip(gtime.Now().Add(24 * time.Hour))
	err = g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		exec := func(query string) {
			t.Helper()
			if _, err := tx.Exec(query); err != nil {
				t.Fatal(err)
			}
		}
		for _, query := range []string{
			// Legacy timestamp columns store Shanghai wall time, including fixtures.
			`SET LOCAL TIME ZONE 'Asia/Shanghai'`,
			`CREATE TEMP TABLE hg_content_profile
				(id bigint PRIMARY KEY,status int,visibility text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_state
				(profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp,
				cycle_override_enabled boolean NOT NULL DEFAULT false,cycle_override_days int NOT NULL DEFAULT 0) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_channel_profile
				(tenant_id bigint,account_id bigint,channel_id bigint,profile_id bigint,status text,
				batch_order bigint,last_job_id bigint) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_account
				(id bigint PRIMARY KEY,tenant_id bigint,status int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_channel
				(tenant_id bigint,account_id bigint,profile_id bigint,channel_id bigint,is_manual int,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_channel
				(id bigint PRIMARY KEY,tenant_id bigint,status int,publish_direction text,is_default_selected int,
				cycle_publish_enabled int,cycle_publish_mode text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_cycle_run
				(id bigint PRIMARY KEY,tenant_id bigint,channel_id bigint,scheduled_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_job
				(id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,channel_id bigint,sent_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_content_profile VALUES (1,1,'public',NULL),(2,1,'public',NULL)`,
			`INSERT INTO hg_youban_publish_profile_state VALUES
				(1,1,0,NULL,true,2),(2,1,0,NULL,false,0),(2,2,0,NULL,true,1)`,
			`INSERT INTO hg_youban_publish_channel VALUES (10,1,1,'up',1,1,'batch',NULL)`,
			`INSERT INTO hg_youban_publish_channel_profile VALUES
				(1,0,10,1,'active',1,100),(1,0,10,2,'active',2,101)`,
			`INSERT INTO hg_youban_publish_tg_job VALUES
				(100,1,0,1,10,now()-interval '10 days'),(101,1,0,2,10,now()-interval '10 days')`,
			`INSERT INTO hg_youban_publish_cycle_run VALUES (20,1,10,now())`,
		} {
			exec(query)
		}
		rows, err := tx.GetAll(`SELECT cp.profile_id FROM hg_youban_publish_channel_profile cp
			WHERE cp.tenant_id=1 AND cp.channel_id=10 AND (` + batchRotationEligibilitySQL() + `)
			ORDER BY cp.batch_order`)
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0]["profile_id"].Int64() != 2 {
			t.Fatalf("batch must exclude independent profile without leaking another tenant's override: %+v", rows)
		}
		independent := cycleProfileScope{ProfileID: 1, AccountID: 0}
		inherited := cycleProfileScope{ProfileID: 2, AccountID: 0}
		assertAllowed := func(operation string, scope cycleProfileScope, want bool) {
			t.Helper()
			got, err := cycleChannelAllowed(ctx, 1, 10, operation, scope)
			if err != nil || got != want {
				t.Fatalf("operation=%s profile=%d allowed=%v want=%v err=%v", operation, scope.ProfileID, got, want, err)
			}
		}
		assertAllowed("cycle_batch:20:1:10", independent, false)
		assertAllowed("cycle_batch:20:2:10", inherited, true)
		assertAllowed("profile_cycle:100:1:1:10", independent, true)
		assertAllowed("profile_cycle:101:1:2:10", inherited, false)
		assertAllowed("profile:manual", independent, true)

		// Expiry does not rewrite the saved two-day override. The same queued
		// child must nevertheless observe the current free fifteen-day interval.
		setVip(gtime.Now().Add(-time.Hour))
		assertAllowed("profile_cycle:100:1:1:10", independent, false)
		exec(`UPDATE hg_youban_publish_tg_job SET sent_at=now()-interval '16 days' WHERE id=100`)
		assertAllowed("profile_cycle:100:1:1:10", independent, true)
		setVip(gtime.Now().Add(24 * time.Hour))
		exec(`UPDATE hg_youban_publish_tg_job SET sent_at=now()-interval '10 days' WHERE id=100`)

		// A task already queued under the old interval cannot bypass a new,
		// not-yet-due plan while the asynchronous projection is catching up.
		exec(`UPDATE hg_youban_publish_tg_job SET sent_at=now() WHERE id=100`)
		assertAllowed("profile_cycle:100:1:1:10", independent, false)
		exec(`UPDATE hg_youban_publish_channel SET cycle_publish_enabled=0 WHERE id=10`)
		assertAllowed("profile_cycle:100:1:1:10", independent, false)
		assertAllowed("profile:manual", independent, true)
		exec(`UPDATE hg_youban_publish_channel SET cycle_publish_enabled=1 WHERE id=10`)
		exec(`UPDATE hg_youban_publish_profile_state SET cycle_override_enabled=false
			WHERE tenant_id=1 AND profile_id=1`)
		assertAllowed("profile_cycle:100:1:1:10", independent, false)
		assertAllowed("cycle_batch:20:1:10", independent, true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Exercise the generated PostgreSQL statements, not an approximation of their
// predicates. All fixtures live on one transaction-local localhost connection.
func TestProfileCycleProjectionStatementsPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_BATCH_ROTATION_TEST_PG_PORT")
	if port == "" {
		t.Skip("set YOUBAN_BATCH_ROTATION_TEST_PG_PORT for disposable localhost PG")
	}
	if port != "55491" {
		t.Fatal("isolated test requires explicit localhost port 55491")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: port, User: "postgres", Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	err = db.Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		exec := func(query string, args ...any) int64 {
			t.Helper()
			result, err := tx.Exec(query, args...)
			if err != nil {
				t.Fatal(err)
			}
			count, err := result.RowsAffected()
			if err != nil {
				t.Fatal(err)
			}
			return count
		}
		for _, query := range []string{
			`SET LOCAL TIME ZONE 'Asia/Shanghai'`,
			`CREATE TEMP TABLE hg_youban_publish_profile_state (
				tenant_id bigint,account_id bigint,profile_id bigint,deleted_at timestamp,
				cycle_override_enabled boolean,cycle_override_version bigint,
				PRIMARY KEY(tenant_id,account_id,profile_id)) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_job (
				id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,channel_id bigint,status text,
				cycle_enabled int NOT NULL DEFAULT 0,cycle_days int NOT NULL DEFAULT 9,
				cycle_publish_time text NOT NULL DEFAULT '',next_cycle_at timestamp,
				profile_cycle_version bigint NOT NULL DEFAULT 0) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_channel_profile (
				id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,channel_id bigint,
				last_job_id bigint,status text,next_cycle_at timestamp,
				applied_cycle_config_version bigint NOT NULL DEFAULT 0,
				applied_profile_cycle_version bigint NOT NULL DEFAULT 0,
				cycle_override_enabled boolean NOT NULL DEFAULT false) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_profile_state VALUES (1,11,1,NULL,true,7),(2,11,1,NULL,true,7)`,
			`INSERT INTO hg_youban_publish_tg_job(id,tenant_id,account_id,profile_id,channel_id,status)
				VALUES (101,1,11,1,10,'sent'),(202,2,11,1,10,'sent')`,
			`INSERT INTO hg_youban_publish_channel_profile(id,tenant_id,account_id,profile_id,channel_id,last_job_id,status)
				VALUES (1,1,11,1,10,101,'active'),(2,2,11,1,10,202,'active')`,
		} {
			exec(query)
		}
		assertProjection := func(label string, relationID int64, days int, jobVersion, appliedVersion, channelVersion int64, enabled bool, jobAt, mappingAt *gtime.Time) {
			t.Helper()
			row, err := tx.GetOne(`SELECT j.cycle_days,j.profile_cycle_version,cp.applied_profile_cycle_version,
				cp.applied_cycle_config_version,cp.cycle_override_enabled,
				(j.next_cycle_at IS NOT DISTINCT FROM ?::timestamp) AS job_time_ok,
				(cp.next_cycle_at IS NOT DISTINCT FROM ?::timestamp) AS mapping_time_ok
				FROM hg_youban_publish_channel_profile cp JOIN hg_youban_publish_tg_job j ON j.id=cp.last_job_id
				WHERE cp.id=?`, jobAt, mappingAt, relationID)
			if err != nil {
				t.Fatal(err)
			}
			if row.IsEmpty() || row["cycle_days"].Int() != days || row["profile_cycle_version"].Int64() != jobVersion ||
				row["applied_profile_cycle_version"].Int64() != appliedVersion || row["applied_cycle_config_version"].Int64() != channelVersion ||
				row["cycle_override_enabled"].Bool() != enabled || !row["job_time_ok"].Bool() || !row["mapping_time_ok"].Bool() {
				t.Fatalf("%s: unexpected projection %+v", label, row)
			}
		}
		applyMapping := func(query string, args []any, want int) {
			t.Helper()
			rows, err := tx.GetAll(query, args...)
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != want {
				t.Fatalf("mapping RETURNING count=%d want=%d", len(rows), want)
			}
		}
		assertAffected := func(query string, args []any, want int64) {
			t.Helper()
			if got := exec(query, args...); got != want {
				t.Fatalf("affected=%d want=%d", got, want)
			}
		}
		nextAt := gtime.New("2026-11-20 08:30:00")
		plan := profileCycleProjection{
			Row:     profileCycleRescheduleRow{RelationId: 1, JobId: 101, TenantId: 1, AccountId: 11, ProfileId: 1, ChannelId: 10},
			Config:  profileCycleChannelConfig{Days: 2, Version: 3, OverrideVersion: 7, OverrideEnabled: true},
			Enabled: 1, NextAt: nextAt,
		}
		// Even matching account/profile/channel IDs and an existing state in tenant
		// 2 cannot authorize its payload to update tenant 1's mapping or Job.
		foreign := plan
		foreign.Row.TenantId = 2
		jobSQL, mappingSQL, ackSQL, args := profileCycleProjectionStatements([]profileCycleProjection{foreign})
		assertAffected(jobSQL, args, 0)
		applyMapping(mappingSQL, args, 0)
		assertAffected(ackSQL, args, 0)
		assertProjection("foreign payload left owner intact", 1, 9, 0, 0, 0, false, nil, nil)
		assertProjection("foreign payload left other tenant intact", 2, 9, 0, 0, 0, false, nil, nil)

		jobSQL, mappingSQL, ackSQL, args = profileCycleProjectionStatements([]profileCycleProjection{plan})
		assertAffected(jobSQL, args, 1)
		assertProjection("Job write alone does not acknowledge", 1, 2, 7, 0, 0, false, nextAt, nil)
		applyMapping(mappingSQL, args, 1)
		assertProjection("mapping write awaits index acknowledgement", 1, 2, 7, 0, 3, true, nextAt, nextAt)
		assertAffected(ackSQL, args, 1)
		assertProjection("explicit acknowledgement advances version", 1, 2, 7, 7, 3, true, nextAt, nextAt)
		assertProjection("owner writes did not update tenant 2", 2, 9, 0, 0, 0, false, nil, nil)

		// A newer projection can be written while its indexing/ack is pending.
		// A delayed old ack must fail against the latest state and schedule.
		exec(`UPDATE hg_youban_publish_profile_state SET cycle_override_version=8 WHERE tenant_id=1 AND account_id=11 AND profile_id=1`)
		newer := plan
		newer.Config.Days = 15
		newer.Config.Version = 4
		newer.Config.OverrideVersion = 8
		newer.NextAt = gtime.New("2026-12-03 08:30:00")
		newJobSQL, newMappingSQL, newAckSQL, newArgs := profileCycleProjectionStatements([]profileCycleProjection{newer})
		assertAffected(newJobSQL, newArgs, 1)
		applyMapping(newMappingSQL, newArgs, 1)
		assertAffected(ackSQL, args, 0)
		assertProjection("old ack cannot complete newer pending projection", 1, 15, 8, 7, 4, true, newer.NextAt, newer.NextAt)
		assertAffected(newAckSQL, newArgs, 1)
		assertProjection("newer acknowledgement", 1, 15, 8, 8, 4, true, newer.NextAt, newer.NextAt)

		// The old callback attempts to restore false and NULL as well as an
		// earlier interval. None of its three statements may overwrite version 8.
		late := plan
		late.Config.OverrideEnabled = false
		late.Enabled = 0
		late.NextAt = nil
		oldJobSQL, oldMappingSQL, oldAckSQL, oldArgs := profileCycleProjectionStatements([]profileCycleProjection{late})
		assertAffected(oldJobSQL, oldArgs, 0)
		applyMapping(oldMappingSQL, oldArgs, 0)
		assertAffected(oldAckSQL, oldArgs, 0)
		assertProjection("late old version cannot restore false or earlier date", 1, 15, 8, 8, 4, true, newer.NextAt, newer.NextAt)
		assertProjection("late callback leaves tenant 2 intact", 2, 9, 0, 0, 0, false, nil, nil)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
