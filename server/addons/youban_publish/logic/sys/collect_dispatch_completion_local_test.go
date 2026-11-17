package sys

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestCollectDispatchCompletionWaitsForEveryTargetAndRuleLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_MULTISOURCE_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	previousDB := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", previousDB)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{
		Type: "pgsql", Host: "127.0.0.1", Port: port, User: "test", Name: "postgres",
	}})
	defer func() { _ = g.DB().GetCore().ClearTableFieldsAll(context.Background()) }()
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, statement := range []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_event(id bigint PRIMARY KEY,status text,updated_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch(
				id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,event_id bigint,status text,updated_at timestamp
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_dispatch_channel(dispatch_id bigint,channel_id bigint) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_tg_job(
				id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,profile_id bigint,collect_event_id bigint,
				channel_id bigint,operation_no text,status text,dispatch_status text
			) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_profile_state(
				profile_id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,deleted_at timestamp
			) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_event VALUES(1,'dispatched',NULL)`,
			`INSERT INTO hg_youban_publish_collect_dispatch VALUES
				(100,87,628,500,1,'pending',NULL),(101,87,628,501,1,'pending',NULL)`,
			`INSERT INTO hg_youban_publish_collect_dispatch_channel VALUES(100,1001),(100,1002),(101,1001)`,
			`INSERT INTO hg_youban_publish_tg_job VALUES
				(1,87,628,500,1,1001,'collect:100','sent','done'),
				(2,87,628,500,1,1002,'collect:100','pending','queued'),
				(3,87,628,501,1,1001,'collect:101','sent','done')`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		settleBranch := func(t *testing.T, id, expected int64) {
			t.Helper()
			result, err := tx.Model(publishCollectDispatchTable).Ctx(ctx).
				Where("id", id).Where(collectDispatchAllJobsSentCondition(publishCollectDispatchTable)).
				Data("status", "sent").Update()
			if err != nil {
				t.Fatal(err)
			}
			affected, err := result.RowsAffected()
			if err != nil || affected != expected {
				t.Fatalf("branch %d settled before all targets: affected=%d want=%d err=%v", id, affected, expected, err)
			}
		}
		settleParent := func(expected int64) {
			t.Helper()
			result, err := tx.Model(publishCollectEventTable).Ctx(ctx).
				Where("id", 1).Where(collectEventAllDispatchesFinishedCondition(publishCollectEventTable)).
				Data("status", "processed").Update()
			if err != nil {
				t.Fatal(err)
			}
			affected, err := result.RowsAffected()
			if err != nil || affected != expected {
				t.Fatalf("event settled before every branch: affected=%d want=%d err=%v", affected, expected, err)
			}
		}
		settleBranch(t, 100, 0) // One of this rule's two target channels is pending.
		settleBranch(t, 101, 1)
		settleParent(0) // First complete rule must not finish its sibling.
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_dispatch SET status='failed' WHERE id=100`); err != nil {
			return err
		}
		settleParent(0)
		if _, err := tx.Exec(`UPDATE hg_youban_publish_tg_job SET status='sent',dispatch_status='done' WHERE id=2`); err != nil {
			return err
		}
		settleBranch(t, 100, 1) // A recovered failed rule can now finish.
		settleParent(1)
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_collect_dispatch_channel VALUES(100,1003)`); err != nil {
			return err
		}
		settleBranch(t, 100, 0) // Missing a Job is not a successful target receipt.
		// Receipt compatibility is separate from transport ownership. All
		// cases use exact event/profile/channel/operation and sent+done jobs.
		for _, tc := range []struct {
			name                                    string
			dispatchAccount, jobAccount             int64
			stateTenant, stateProfile, stateAccount int64
			hasState, deleted                       bool
			want                                    int64
		}{
			{name: "raw0_to_actual_with_owner", jobAccount: 615, hasState: true, stateTenant: 78, stateProfile: 600, stateAccount: 615, want: 1},
			{name: "raw0_to_actual_missing_state", jobAccount: 615},
			{name: "raw0_to_actual_wrong_owner", jobAccount: 615, hasState: true, stateTenant: 78, stateProfile: 600, stateAccount: 999},
			{name: "raw0_state_not_business_owner", jobAccount: 615, hasState: true, stateTenant: 78, stateProfile: 600},
			{name: "raw0_to_actual_wrong_tenant", jobAccount: 615, hasState: true, stateTenant: 79, stateProfile: 600, stateAccount: 615},
			{name: "raw0_to_actual_wrong_profile", jobAccount: 615, hasState: true, stateTenant: 78, stateProfile: 601, stateAccount: 615},
			{name: "raw0_to_actual_deleted_owner", jobAccount: 615, hasState: true, deleted: true, stateTenant: 78, stateProfile: 600, stateAccount: 615},
			{name: "raw0_exact_raw0_preserved", want: 1},
			{name: "positive_exact_preserved", dispatchAccount: 615, jobAccount: 615, want: 1},
			{name: "positive_mismatch_not_alias", dispatchAccount: 999, jobAccount: 615, hasState: true, stateTenant: 78, stateProfile: 600, stateAccount: 615},
			{name: "positive_dispatch_raw0_job_rejected", dispatchAccount: 615},
			{name: "negative_job_not_business_owner", jobAccount: -1, hasState: true, stateTenant: 78, stateProfile: 600, stateAccount: -1},
		} {
			t.Run(tc.name, func(t *testing.T) {
				for _, sql := range []string{
					`DELETE FROM hg_youban_publish_profile_state`,
					`DELETE FROM hg_youban_publish_tg_job WHERE id=10`,
					`DELETE FROM hg_youban_publish_collect_dispatch_channel WHERE dispatch_id=200`,
					`DELETE FROM hg_youban_publish_collect_dispatch WHERE id=200`,
				} {
					if _, err := tx.Exec(sql); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := tx.Exec(`INSERT INTO hg_youban_publish_collect_dispatch VALUES(200,78,?,600,2,'pending',NULL)`, tc.dispatchAccount); err != nil {
					t.Fatal(err)
				}
				if _, err := tx.Exec(`INSERT INTO hg_youban_publish_collect_dispatch_channel VALUES(200,2001)`); err != nil {
					t.Fatal(err)
				}
				if _, err := tx.Exec(`INSERT INTO hg_youban_publish_tg_job VALUES(10,78,?,600,2,2001,'collect:200','sent','done')`, tc.jobAccount); err != nil {
					t.Fatal(err)
				}
				if tc.hasState {
					if _, err := tx.Exec(`INSERT INTO hg_youban_publish_profile_state VALUES(?,?,?,NULL)`, tc.stateProfile, tc.stateTenant, tc.stateAccount); err != nil {
						t.Fatal(err)
					}
					if tc.deleted {
						if _, err := tx.Exec(`UPDATE hg_youban_publish_profile_state SET deleted_at=CURRENT_TIMESTAMP`); err != nil {
							t.Fatal(err)
						}
					}
				}
				settleBranch(t, 200, tc.want)
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
