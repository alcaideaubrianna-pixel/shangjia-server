package sys

import (
	"context"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestDeliveryScopeKeysetLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if number, err := strconv.Atoi(port); err != nil || number < 1024 || number > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	before := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", before)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	ctx := context.Background()
	defer func() { _ = g.DB().GetCore().ClearTableFieldsAll(ctx) }()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, statement := range []string{
			`CREATE TEMP TABLE hg_tg_collector_delivery(
 id bigint PRIMARY KEY, tenant_id bigint DEFAULT 1,account_id bigint DEFAULT 2,source_id bigint DEFAULT 3,
 scheduler_pool text DEFAULT 'realtime',status text DEFAULT 'pending',priority int,
 created_at timestamp DEFAULT '2026-11-16 12:00:00.123456',updated_at timestamp DEFAULT '2026-11-16',
 next_run_at timestamp,attempt_count int DEFAULT 0) ON COMMIT DROP`,
			`CREATE INDEX ON hg_tg_collector_delivery(tenant_id,account_id,source_id,scheduler_pool,priority DESC,created_at,id) WHERE status IN ('pending','failed_retry')`,
			`INSERT INTO hg_tg_collector_delivery(id,priority) VALUES(1,100),(2,100),(3,50),(4,1),(10,17)`,
			`INSERT INTO hg_tg_collector_delivery(id,priority,account_id) VALUES(5,100,99)`,
			`INSERT INTO hg_tg_collector_delivery(id,priority,status,next_run_at) VALUES(6,100,'failed_retry','2026-11-18')`,
			`INSERT INTO hg_tg_collector_delivery(id,priority,scheduler_pool) VALUES(7,100,'history')`,
			`INSERT INTO hg_tg_collector_delivery(id,priority,attempt_count) VALUES(8,100,100)`,
			`INSERT INTO hg_tg_collector_delivery(id,priority,created_at) VALUES(9,100,'2026-11-18')`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		scope := collectorRecoveryScope{TenantID: 1, AccountID: 2, SourceID: 3}
		cursor := deliveryRecoveryCursor{UntilTime: "2026-11-17 00:00:00.000001", StaleBefore: "2026-11-17 00:00:00.000001"}
		for _, expected := range [][]int64{{1, 2}, {3, 10}, {4}} {
			rows, err := deliveryRecoveryPage(ctx, scope, "realtime", cursor, 2)
			if err != nil {
				return err
			}
			ids := make([]int64, 0, len(rows))
			for _, row := range rows {
				ids = append(ids, row["id"].Int64())
			}
			if !reflect.DeepEqual(ids, expected) {
				t.Fatalf("keyset skipped or leaked scope: got %v want %v", ids, expected)
			}
			cursor, err = walkDeliveryRecoveryPage(rows, cursor, 2, func(gdb.Record) error { return nil })
			if err != nil {
				return err
			}
		}
		if cursor.UntilTime != "" || cursor.AfterID != 0 {
			t.Fatalf("completed window must wrap to retry still-pending rows: %+v", cursor)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
