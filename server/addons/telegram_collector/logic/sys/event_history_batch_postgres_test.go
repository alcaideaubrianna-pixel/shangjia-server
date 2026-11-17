package sys

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// Only the dedicated, disposable loopback container is accepted. No production DSN.
func TestHistoryEventBatchLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_COLLECTOR_BATCH_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable local PostgreSQL")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n != 55489 {
		t.Fatal("requires dedicated loopback port 55489")
	}
	if !collectorBatchPostgresChild(t) {
		return
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: "postgres", Name: "postgres"}})
	ctx := context.Background()
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		_, err := tx.Exec(`CREATE TEMP TABLE hg_tg_collector_event (
 id bigint PRIMARY KEY,tenant_id bigint,source_id bigint,source_type text,account_id bigint,
 status text,lease_owner text,lease_until timestamp,attempt_count integer,
 updated_at timestamp,processed_at timestamp,next_run_at timestamp,error_message text,raw_update jsonb)`)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO hg_tg_collector_event(id,tenant_id,status,lease_owner,attempt_count,raw_update)
 VALUES (1,78,'received','',0,'{}'),(2,78,'received','',0,'{}'),(3,99,'received','',0,'{}')`)
		if err != nil {
			return err
		}
		claims := writeHistoryEventMutations(ctx, []historyEventMutation{
			{Kind: historyEventClaim, ID: 1, Owner: "a"}, {Kind: historyEventClaim, ID: 2, Owner: "b"}, {Kind: historyEventClaim, ID: 1, Owner: "duplicate"},
		})
		if claims[0].Err != nil || claims[1].Err != nil || claims[0].Value.LeaseOwner != "a" || claims[1].Value.AttemptCount != 1 || claims[2].Value.Id != 0 {
			t.Fatalf("claims=%+v", claims)
		}
		completed := writeHistoryEventMutations(ctx, []historyEventMutation{
			{Kind: historyEventReady, ID: 1, TenantID: 78, Owner: "a", Attempt: 1},
			{Kind: historyEventReady, ID: 2, TenantID: 99, Owner: "b", Attempt: 1},
		})
		if completed[0].Err != nil || completed[0].Value.Id != 1 || completed[1].Value.Id != 0 {
			t.Fatalf("completion=%+v", completed)
		}
		if _, err := tx.Exec(`UPDATE hg_tg_collector_event SET lease_owner='new',attempt_count=2 WHERE id=2`); err != nil {
			return err
		}
		stale := writeHistoryEventMutations(ctx, []historyEventMutation{{Kind: historyEventReady, ID: 2, TenantID: 78, Owner: "b", Attempt: 1}})
		if stale[0].Err != nil || stale[0].Value.Id != 0 {
			t.Fatalf("stale owner settled newer claim: %+v", stale)
		}
		rows, err := tx.GetAll(`SELECT id,status,lease_owner FROM hg_tg_collector_event ORDER BY id`)
		if err != nil {
			return err
		}
		if rows[0]["status"].String() != "ready" || rows[1]["lease_owner"].String() != "new" || rows[2]["status"].String() != "received" {
			t.Fatalf("rows=%v", rows)
		}
		_, err = tx.Exec(`DROP TABLE hg_tg_collector_event`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
