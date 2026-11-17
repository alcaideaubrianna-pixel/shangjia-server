package sys

import (
	"context"
	"os"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/utility/sourcequeue"
)

// Explicit opt-in to the same disposable loopback fixture as the batch tests.
// Temporary tables and the isolated child process never modify a shared schema.
func TestSourceEventClaimLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_COLLECTOR_BATCH_TEST_PORT") == "" {
		t.Skip("requires disposable local PostgreSQL")
	}
	if !collectorBatchPostgresChild(t) {
		return
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55489", User: "postgres", Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec(`CREATE TEMP TABLE hg_tg_collector_event (
 id bigint PRIMARY KEY,tenant_id bigint,source_id bigint,status text,
 lease_owner text,lease_until timestamp,attempt_count integer,updated_at timestamp,
 next_run_at timestamp)`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_tg_collector_event
 (id,tenant_id,source_id,status,lease_owner,attempt_count,next_run_at) VALUES
 (1,78,351,'received','',0,NULL),
 (2,78,1370,'received','',0,NULL),
 (3,79,351,'received','',0,NULL),
 (4,78,351,'failed_retry','',0,CURRENT_TIMESTAMP + INTERVAL '1 day'),
 (5,78,351,'ready','',1,NULL)`); err != nil {
			return err
		}
		scope := sourcequeue.Scope{TenantID: 78, SourceID: 351}
		for _, id := range []int64{2, 3} {
			if _, err := claimSourceCollectorEvent(ctx, id, "wrong", scope); err == nil {
				t.Fatalf("claimed event %d belonging to another source or tenant", id)
			}
		}
		row, err := claimSourceCollectorEvent(ctx, 1, "first", scope)
		if err != nil || row.Id != 1 || row.LeaseOwner != "first" || row.AttemptCount != 1 {
			t.Fatalf("first claim: row=%+v err=%v", row, err)
		}
		if _, err := claimSourceCollectorEvent(ctx, 1, "second", scope); err == nil {
			t.Fatal("busy durable lease was acknowledged as completed work")
		}
		if _, err := claimSourceCollectorEvent(ctx, 4, "early", scope); err == nil {
			t.Fatal("future retry was consumed before it was due")
		}
		if row, err := claimSourceCollectorEvent(ctx, 5, "terminal", scope); err != nil || row.Id != 0 {
			t.Fatalf("terminal receipt must settle without reprocessing: %+v %v", row, err)
		}
		rows, err := tx.GetAll(`SELECT id,status,attempt_count,lease_owner FROM hg_tg_collector_event ORDER BY id`)
		if err != nil {
			return err
		}
		for _, index := range []int{1, 2, 3} {
			if rows[index]["attempt_count"].Int() != 0 {
				t.Fatalf("unowned or delayed row was mutated: %v", rows[index])
			}
		}
		if rows[0]["lease_owner"].String() != "first" {
			t.Fatal("second claim replaced the durable owner")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
