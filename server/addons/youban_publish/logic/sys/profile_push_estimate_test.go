package sys

import (
	"context"
	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/os/gtime"
)

func TestProfilePushBatchDayOffset(t *testing.T) {
	for _, tc := range []struct {
		size int
		rank profilePushBatchRank
		want int64
		ok   bool
	}{
		{70, profilePushBatchRank{Eligible: 1}, 0, true},
		{70, profilePushBatchRank{Eligible: 1, BeforeTarget: 69}, 0, true},
		{70, profilePushBatchRank{Eligible: 1, BeforeTarget: 70}, 1, true},
		{70, profilePushBatchRank{Eligible: 1, BeforeTarget: 262}, 3, true},
		{70, profilePushBatchRank{Eligible: 1, InCurrentRun: true}, -1, true},
		{70, profilePushBatchRank{}, 0, false},
		{0, profilePushBatchRank{Eligible: 1}, 0, false},
	} {
		if got, ok := profilePushBatchDayOffset(tc.size, tc.rank, 0); got != tc.want || ok != tc.ok {
			t.Fatalf("rank=%+v got=%d,%v want=%d,%v", tc.rank, got, ok, tc.want, tc.ok)
		}
	}
}

func TestProfilePushBatchSendingDayBeijing(t *testing.T) {
	// The same instant is Nov 19 in Beijing, but still Nov 18 in UTC.
	first := gtime.New(time.Date(2026, 11, 18, 18, 30, 0, 0, time.UTC))
	if got := profilePushBatchSendingDay("13:00", first, 1); got.Format("Y-m-d H:i") != "2026-11-20 13:00" {
		t.Fatalf("Beijing next day: %s", got)
	}
	// An overdue first run after today's configured clock still advances one
	// full calendar sending day, rather than counting today's clock twice.
	first = gtime.New(time.Date(2026, 11, 18, 15, 0, 0, 0, time.FixedZone("UTC+8", 28800)))
	if got := profilePushBatchSendingDay("13:00", first, 3); got.Format("Y-m-d H:i") != "2026-11-21 13:00" {
		t.Fatalf("three sending days: %s", got)
	}
}

func TestProfilePushNextBatchAt(t *testing.T) {
	zone := time.FixedZone("UTC+8", 8*60*60)
	now := gtime.New(time.Date(2026, 11, 18, 12, 0, 0, 0, zone))
	future := gtime.New(time.Date(2026, 11, 19, 13, 0, 0, 0, zone))
	past := gtime.New(time.Date(2026, 11, 18, 11, 0, 0, 0, zone))
	for _, tc := range []struct {
		name    string
		planned *gtime.Time
		want    int64
	}{
		{"stored plan", future, future.Unix()},
		{"overdue is immediately due", past, now.Unix()},
		{"missing plan is immediately due", nil, now.Unix()},
		{"database clock represented as UTC", gtime.New(time.Date(2026, 11, 18, 13, 0, 0, 0, time.UTC)), now.Add(time.Hour).Unix()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := profilePushNextBatchAt(tc.planned, now); got.Unix() != tc.want {
				t.Fatalf("got=%s want unix=%d", got, tc.want)
			}
		})
	}
}

func TestProfilePushBatchActiveRunDates(t *testing.T) {
	zone := time.FixedZone("UTC+8", 28800)
	now := gtime.New(time.Date(2026, 11, 18, 15, 0, 0, 0, zone))
	planned := gtime.New(time.Date(2026, 11, 19, 13, 0, 0, 0, zone))
	for _, tc := range []struct {
		name   string
		active bool
		offset int64
		want   string
	}{
		{"selected still pending today", true, -1, "2026-11-18 15:00"},
		{"unselected rank zero starts tomorrow", true, 0, "2026-11-19 13:00"},
		{"sent tail next selection day plus rank", true, 3, "2026-11-22 13:00"},
		{"no active run overdue starts now", false, 0, "2026-11-18 15:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := profilePushBatchEstimateAt("13:00", nil, now, now, tc.active, tc.offset).Format("Y-m-d H:i"); got != tc.want {
				t.Fatalf("got=%s want=%s", got, tc.want)
			}
		})
	}
	if got := profilePushBatchEstimateAt("13:00", planned, now, now, true, 0); got.Unix() != planned.Unix() {
		t.Fatalf("advanced valid next plan twice: %s", got)
	}
}

func TestProfilePushEstimateRotationPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_BATCH_ROTATION_TEST_PG_PORT")
	if port == "" {
		t.Skip("set YOUBAN_BATCH_ROTATION_TEST_PG_PORT=55491 for isolated loopback PG")
	}
	if port != "55491" {
		t.Fatal("only the disposable loopback port 55491 is accepted")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: port, User: "postgres", Name: "postgres", MaxOpenConnCount: 1, MaxIdleConnCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		exec := func(query string) {
			t.Helper()
			if _, err := tx.Exec(query); err != nil {
				t.Fatal(err)
			}
		}
		// Connection-local TEMP tables deliberately omit every legacy round,
		// created_at, media-ready and task-status field.
		exec(`CREATE TEMP TABLE hg_youban_publish_channel_profile (tenant_id bigint,account_id bigint,channel_id bigint,profile_id bigint,status text,batch_order bigint,cycle_override_enabled boolean NOT NULL DEFAULT false) ON COMMIT DROP`)
		exec(`CREATE TEMP TABLE hg_youban_publish_cycle_batch_item (run_id bigint,ordinal bigint,tenant_id bigint,account_id bigint,channel_id bigint,profile_id bigint,batch_order bigint,job_id bigint,item_state text,PRIMARY KEY(run_id,ordinal)) ON COMMIT DROP`)
		exec(`CREATE TEMP TABLE hg_content_profile (id bigint PRIMARY KEY,status int,visibility text,deleted_at timestamp) ON COMMIT DROP`)
		exec(`CREATE TEMP TABLE hg_youban_publish_profile_state (profile_id bigint,tenant_id bigint,account_id bigint,deleted_at timestamp,cycle_override_enabled boolean NOT NULL DEFAULT false) ON COMMIT DROP`)
		exec(`CREATE TEMP TABLE hg_youban_publish_account (id bigint PRIMARY KEY,tenant_id bigint,status int,deleted_at timestamp) ON COMMIT DROP`)
		exec(`CREATE TEMP TABLE hg_youban_publish_profile_channel (tenant_id bigint,account_id bigint,profile_id bigint,channel_id bigint,is_manual int,deleted_at timestamp) ON COMMIT DROP`)
		exec(`CREATE TEMP TABLE hg_youban_publish_channel (id bigint PRIMARY KEY,tenant_id bigint,status int,publish_direction text,is_default_selected int,deleted_at timestamp) ON COMMIT DROP`)
		exec(`CREATE TEMP TABLE hg_youban_publish_tg_job (id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,channel_id bigint,profile_id bigint,status text) ON COMMIT DROP`)
		check := func(want bool) {
			t.Helper()
			r, e := tx.GetOne(profilePushBatchSchemaSQL(), publishChannelProfileTable, profilePushBatchItemTable, profilePushBatchItemTable, publishChannelProfileTable)
			if e != nil {
				t.Fatal(e)
			}
			if r["queue_index"].Bool() != want || !r["snapshot_columns"].Bool() || !r["snapshot_index"].Bool() || (want && !r["unmigrated_index"].Bool()) {
				t.Fatalf("schema readiness: %+v", r)
			}
		}
		check(false)
		exec(`CREATE INDEX idx_ybp_batch_inherited_rotation ON hg_youban_publish_channel_profile(tenant_id,channel_id,status,batch_order,profile_id) WHERE status='inactive' AND batch_order>0`)
		check(false) // Same columns with a wrong partial predicate is not ready.
		exec(`DROP INDEX pg_temp.idx_ybp_batch_inherited_rotation`)
		exec(`CREATE INDEX idx_ybp_batch_inherited_rotation ON hg_youban_publish_channel_profile(tenant_id,channel_id,status,batch_order,profile_id) WHERE status='active' AND batch_order>0 AND cycle_override_enabled=false`)
		exec(`CREATE INDEX idx_ybp_batch_unmigrated ON hg_youban_publish_channel_profile(tenant_id,channel_id) WHERE status='active' AND batch_order<=0`)
		check(true)
		exec(`INSERT INTO hg_youban_publish_channel VALUES (10,1,1,'up',1,NULL)`)
		exec(`INSERT INTO hg_content_profile SELECT id,1,'public',NULL FROM generate_series(1,9) id`)
		exec(`UPDATE hg_content_profile SET status=2 WHERE id=7`)
		exec(`UPDATE hg_content_profile SET visibility='private' WHERE id=8`)
		exec(`UPDATE hg_content_profile SET deleted_at=now() WHERE id=9`)
		exec(`INSERT INTO hg_youban_publish_profile_state(profile_id,tenant_id,account_id,deleted_at) SELECT id,1,0,NULL FROM generate_series(1,9) id`)
		// Selection moved 1 and 2 to the tail; 3 later sent and moved again.
		exec(`INSERT INTO hg_youban_publish_channel_profile VALUES (1,0,10,4,'active',4),(1,0,10,5,'active',5),(1,0,10,6,'active',6),(1,0,10,1,'active',101),(1,0,10,2,'active',102),(1,0,10,3,'active',200),(1,0,10,7,'active',1),(1,0,10,8,'active',2),(1,0,10,9,'active',3),(2,0,10,900,'active',1),(1,0,99,901,'active',1)`)
		exec(`INSERT INTO hg_youban_publish_cycle_batch_item VALUES (20,1,1,0,10,1,1,0,'selected'),(20,2,1,0,10,2,2,22,'admitted'),(20,3,1,0,10,3,3,23,'admitted')`)
		exec(`INSERT INTO hg_youban_publish_tg_job VALUES (22,1,0,10,2,'pending'),(23,1,0,10,3,'sent')`)
		load := func(limit int) []profilePushQueueItem {
			t.Helper()
			rows, e := tx.GetAll(profilePushQueueSnapshotSQL(), 1, 10, limit, 20, 1, 10, limit, JobStatusSent, JobStatusFailed, JobStatusSuperseded)
			if e != nil {
				t.Fatal(e)
			}
			var items []profilePushQueueItem
			if e = rows.Structs(&items); e != nil {
				t.Fatal(e)
			}
			return items
		}
		items := load(100)
		if len(items) != 9 {
			t.Fatalf("cross-tenant/channel leaked: %d", len(items))
		}
		for _, tc := range []struct {
			id, ahead int64
			member    bool
		}{{1, 0, true}, {2, 1, true}, {3, 3, false}, {4, 0, false}, {6, 2, false}} {
			position := profilePushBatchPosition(profilePushQueueRank(items, tc.id, 0), 70)
			if position.AheadCount != tc.ahead || position.InCurrentRun != tc.member {
				t.Fatalf("id=%d position=%+v", tc.id, position)
			}
		}
		for _, id := range []int64{7, 8, 9} {
			if rank := profilePushQueueRank(items, id, 0); rank.Eligible != 0 {
				t.Fatalf("ineligible profile counted: %d", id)
			}
		}
		if len(load(2)) != 2 {
			t.Fatal("queue snapshot did not honor bound")
		}
		exec(`UPDATE hg_youban_publish_tg_job SET status='failed' WHERE id=22`)
		items = load(100)
		if rank := profilePushQueueRank(items, 2, 0); rank.InCurrentRun {
			t.Fatal("terminal failed job still in today's processing")
		}
		if pos := profilePushBatchPosition(profilePushQueueRank(items, 3, 0), 70); pos.AheadCount != 4 {
			t.Fatalf("failed predecessor incorrectly deducted: %+v", pos)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
