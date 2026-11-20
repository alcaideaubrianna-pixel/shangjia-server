package sys

import (
	"context"
	"os"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestDeliveryCompletionBatchLocalPostgres(t *testing.T) {
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
id bigint PRIMARY KEY,status text,next_run_at timestamp,lease_owner text,lease_until timestamp,error_message text,updated_at timestamp) ON COMMIT DROP`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_tg_collector_delivery(id,status,lease_owner) VALUES (1,'processing','current'), (2,'completed',''), (3,'processing','another')`); err != nil {
			return err
		}
		results := writeDeliveryCompletions(ctx, []deliveryCompletion{
			{ID: 1, Owner: "stale"}, {ID: 1, Owner: "current"}, {ID: 2, Owner: "current"}, {ID: 3, Owner: "current"},
		})
		for i, result := range results {
			if result.Err != nil || result.Value != (i == 1) {
				t.Errorf("request %d: got %+v", i, result)
			}
		}
		value, err := tx.GetValue(`SELECT count(*) FROM hg_tg_collector_delivery WHERE status='completed' AND lease_owner=''`)
		if err == nil && value.Int() != 2 {
			t.Errorf("completed rows=%d, want 2", value.Int())
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
