package sys

import (
	"context"
	"os"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestDeliveryLeaseRecoveryIsScopedAndBoundedLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_COLLECTOR_BATCH_TEST_PORT") != "55489" {
		t.Skip("requires disposable localhost PostgreSQL on port 55489")
	}
	if !collectorBatchPostgresChild(t) {
		return
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55489", User: "postgres", Name: "postgres"}})

	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec(`CREATE TEMP TABLE hg_tg_collector_delivery (
id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,attempt_count int,
status text,next_run_at timestamp,lease_owner text,lease_until timestamp,error_message text,updated_at timestamp) ON COMMIT DROP`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_tg_collector_delivery
(id,tenant_id,account_id,source_id,attempt_count,status,lease_owner,lease_until) VALUES
(1,78,0,351,1,'processing','old','2000-01-01'),
(2,78,0,351,5,'processing','old','2000-01-01'),
(3,78,0,352,1,'processing','other','1999-01-01'),
(4,78,0,351,1,'processing','renewed','2099-01-01')`); err != nil {
			return err
		}
		collector := &sCollector{}
		scope := collectorRecoveryScope{TenantID: 78, SourceID: 351}
		for _, first := range []bool{true, false} {
			if err := collector.resetExpiredDeliveryLeasesForScope(ctx, 1, scope); err != nil {
				return err
			}
			rows, err := tx.GetAll(`SELECT id,status,lease_owner,lease_until,next_run_at FROM hg_tg_collector_delivery ORDER BY id`)
			if err != nil {
				return err
			}
			if rows[0]["status"].String() != "failed_retry" || !rows[0]["lease_until"].IsNil() || rows[0]["next_run_at"].IsNil() {
				t.Fatalf("first lease was not reset for retry: %v", rows[0])
			}
			if first && rows[1]["status"].String() != "processing" {
				t.Fatalf("recovery exceeded its quota: %v", rows[1])
			}
			if !first && (rows[1]["status"].String() != "dead" || !rows[1]["next_run_at"].IsNil()) {
				t.Fatalf("attempt limit did not produce a terminal row: %v", rows[1])
			}
			if rows[2]["lease_owner"].String() != "other" || rows[3]["lease_owner"].String() != "renewed" {
				t.Fatalf("recovery touched an unrelated or live lease: %v", rows)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
