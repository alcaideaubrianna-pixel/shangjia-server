package sys

import (
	"context"
	"os"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestHistoryDeliveryBatchLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_COLLECTOR_BATCH_TEST_PORT") != "55489" {
		t.Skip("requires dedicated loopback PostgreSQL 55489")
	}
	if !collectorBatchPostgresChild(t) {
		return
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55489", User: "postgres", Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		_, err := tx.Exec(`CREATE TEMP TABLE hg_tg_collector_delivery (
 id bigserial PRIMARY KEY,tenant_id bigint,event_id bigint,delivery_key text,status text,priority integer,
 source_id bigint,account_id bigint,scheduler_pool text,lease_owner text DEFAULT '',created_at timestamp,updated_at timestamp,UNIQUE(tenant_id,delivery_key))`)
		if err != nil {
			return err
		}
		d := sysin.CollectorDelivery{TenantID: 78, SourceID: 351, EventID: 1, DeliveryKey: "test-stable", History: true}
		first := writeHistoryDeliveryBatch(ctx, []sysin.CollectorDelivery{d, d})
		if first[0].Err != nil || first[1].Err != nil || first[0].Value.ID <= 0 || first[0].Value != first[1].Value {
			t.Fatalf("duplicate identities: %+v", first)
		}
		if _, err := tx.Exec(`UPDATE hg_tg_collector_delivery SET priority=100,scheduler_pool='realtime'`); err != nil {
			return err
		}
		if got := writeHistoryDeliveryBatch(ctx, []sysin.CollectorDelivery{d}); got[0].Err != nil {
			t.Fatal(got[0].Err)
		}
		row, err := tx.GetOne(`SELECT priority,scheduler_pool FROM hg_tg_collector_delivery`)
		if err != nil {
			return err
		}
		if row["priority"].Int() != sysin.EventPriorityNormal || row["scheduler_pool"].String() != "history" {
			t.Fatalf("history promoted to live: %v", row)
		}
		if _, err := tx.Exec(`UPDATE hg_tg_collector_delivery SET status='completed'`); err != nil {
			return err
		}
		d.EventID = 2
		collision := writeHistoryDeliveryBatch(ctx, []sysin.CollectorDelivery{d})
		if collision[0].Err != nil || collision[0].Value.EventID != 1 {
			t.Fatalf("collision overwrote original event: %+v", collision)
		}
		row, err = tx.GetOne(`SELECT event_id,status FROM hg_tg_collector_delivery`)
		if err != nil {
			return err
		}
		if row["event_id"].Int64() != 1 || row["status"].String() != "completed" {
			t.Fatalf("completed delivery changed: %v", row)
		}
		_, err = tx.Exec(`DROP TABLE hg_tg_collector_delivery`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
