package channelprogress

import (
	"context"
	"os"
	"testing"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestCollectChannelProgressLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_COLLECTOR_BATCH_TEST_PORT") != "55489" {
		t.Skip("requires dedicated loopback PostgreSQL 55489")
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55489", User: "postgres", Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		_, err := tx.Exec(`CREATE TEMP TABLE hg_youban_publish_collect_channel (
 id bigint PRIMARY KEY,tenant_id bigint,last_message_id bigint,last_message_at timestamp,last_seen_at timestamp,updated_at timestamp)`)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_collect_channel(id,tenant_id,last_message_id) VALUES(1,78,100),(2,99,200)`); err != nil {
			return err
		}
		for _, input := range [][]Progress{
			{{78, 1, 110}, {78, 1, 108}, {78, 2, 999}},
			{{78, 1, 105}},
		} {
			for _, result := range Write(ctx, input) {
				if result.Err != nil {
					return result.Err
				}
			}
		}
		rows, err := tx.GetAll(`SELECT id,last_message_id,last_message_at FROM hg_youban_publish_collect_channel ORDER BY id`)
		if err != nil {
			return err
		}
		if rows[0]["last_message_id"].Int64() != 110 || rows[1]["last_message_id"].Int64() != 200 || rows[0]["last_message_at"].IsNil() {
			t.Fatalf("progress regressed or crossed tenant: %v", rows)
		}
		_, err = tx.Exec(`DROP TABLE hg_youban_publish_collect_channel`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
