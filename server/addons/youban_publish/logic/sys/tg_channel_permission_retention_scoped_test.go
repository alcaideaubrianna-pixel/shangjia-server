package sys

import (
	"context"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
)

func TestPermissionBacklogScopedCandidatesLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_PERMISSION_LOCAL_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:35432")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, statement := range []string{
			"CREATE TEMP TABLE " + publishTgJobTable + " (id bigint PRIMARY KEY,tenant_id bigint,channel_id bigint,status text,operation_no text DEFAULT '',send_phase text DEFAULT '',created_at timestamp DEFAULT '2026-11-01') ON COMMIT DROP",
			"CREATE INDEX ON " + publishTgJobTable + " (tenant_id,channel_id,status,id)",
			"CREATE TEMP TABLE " + publishTgAttemptTable + " (job_id bigint) ON COMMIT DROP",
			"CREATE TEMP TABLE " + publishTgMessageTable + " (job_id bigint) ON COMMIT DROP",
			"CREATE TEMP TABLE " + channelPermissionCircuitTable + " (tenant_id bigint,channel_id bigint,paused int,resumed_at timestamp,PRIMARY KEY(tenant_id,channel_id)) ON COMMIT DROP",
			"INSERT INTO " + channelPermissionCircuitTable + " VALUES (1,20,1,NULL)",
			// A full excluded first page must not hide eligible later jobs.
			"INSERT INTO " + publishTgJobTable + " (id,tenant_id,channel_id,status) SELECT n,1,20,'pending' FROM generate_series(1,10) n",
			"INSERT INTO " + publishTgAttemptTable + " SELECT n FROM generate_series(1,10) n",
			"INSERT INTO " + publishTgJobTable + " (id,tenant_id,channel_id,status,operation_no,send_phase) VALUES " +
				"(11,1,20,'pending','profile:ok','')," +
				"(12,1,20,'pending','profile:message','')," +
				"(13,1,20,'pending','  DOWN:1','')," +
				"(14,1,20,'pending','message_push:1','')," +
				"(15,1,20,'pending','message_push_plan:1','')," +
				"(16,1,20,'pending','profile:confirmed','display_confirmed')," +
				"(17,1,20,'pending','profile:cleanup','cleanup_confirmed')," +
				"(18,1,20,'pending','profile:new','')," +
				"(19,1,20,'pending','profile:boundary','')," +
				"(20,1,20,'failed_retry','profile:retry','')," +
				"(21,2,20,'pending','profile:other-tenant','')," +
				"(22,1,21,'pending','profile:other-channel','')," +
				"(23,1,20,'sent','profile:sent','')",
			"INSERT INTO " + publishTgMessageTable + " VALUES (12)",
			"UPDATE " + publishTgJobTable + " SET created_at='2026-11-18' WHERE id=18",
			"UPDATE " + publishTgJobTable + " SET created_at='2026-11-10' WHERE id=19",
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		scope := permissionBacklogScope{TenantId: 1, ChannelId: 20}
		page := func(status JobStatus, after int64) ([]int64, []int64, error) {
			query, args := permissionBacklogCandidatesSQL(scope, status, after)
			rows, err := tx.GetAll(query, args...)
			if err != nil {
				return nil, nil, err
			}
			ids := make([]int64, 0, len(rows))
			for _, row := range rows {
				ids = append(ids, row["id"].Int64())
			}
			if len(ids) == 0 {
				return ids, nil, nil
			}
			var jobs []telegramJobRecord
			err = permissionBacklogEligibleJobsModel(tx.Model(publishTgJobTable).Safe().Ctx(ctx), scope, status, ids).
				OrderAsc("id").Scan(&jobs)
			eligible := make([]int64, 0, len(jobs))
			for _, job := range jobs {
				eligible = append(eligible, job.Id)
				if job.TenantId != 1 || job.ChannelId != 20 || job.Status != status {
					t.Fatalf("candidate lost scope or status: %+v", job)
				}
			}
			return ids, eligible, err
		}
		first, eligible, err := page(telegramJobStatusPending, 0)
		if err != nil {
			return err
		}
		if len(first) != permissionBacklogJobPageSize || first[0] != 1 || first[len(first)-1] != 10 || len(eligible) != 0 {
			t.Fatalf("evidence-heavy page must remain bounded: candidates=%v eligible=%v", first, eligible)
		}
		second, eligible, err := page(telegramJobStatusPending, first[len(first)-1])
		if err != nil {
			return err
		}
		if len(second) != 9 || !reflect.DeepEqual(eligible, []int64{11, 17, 18, 19}) {
			t.Fatalf("next page lost safety checks: candidates=%v eligible=%v", second, eligible)
		}
		_, retry, err := page(telegramJobStatusRetryWait, 0)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(retry, []int64{20}) {
			t.Fatalf("status equality lane missed retry job: %v", retry)
		}
		if _, err := tx.Exec("UPDATE " + channelPermissionCircuitTable + " SET paused=0,resumed_at='2026-11-10'"); err != nil {
			return err
		}
		_, eligible, err = page(telegramJobStatusPending, 10)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(eligible, []int64{11, 17, 19}) {
			t.Fatalf("resume must retain exact <= boundary and exclude newer jobs: %v", eligible)
		}
		// Cursor wrap repeats safe checks after circuit state changes.
		if _, err := tx.Exec("UPDATE " + channelPermissionCircuitTable + " SET resumed_at=NULL"); err != nil {
			return err
		}
		_, eligible, err = page(telegramJobStatusPending, 10)
		if err != nil {
			return err
		}
		if len(eligible) != 0 {
			t.Fatalf("inactive circuit admitted candidates: %v", eligible)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
