package sys

import (
	"context"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"os"
	"testing"
)

func TestCollectReviewRecoveryLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires local PostgreSQL")
	}
	previous := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", previous)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("CREATE TEMP TABLE hg_youban_publish_collect_dispatch(id bigint PRIMARY KEY,review_id bigint,status text,error_message text,finished_at timestamp,updated_at timestamp) ON COMMIT DROP"); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO hg_youban_publish_collect_dispatch VALUES(1,3371,'failed','old error',now(),now()),(2,0,'failed','old error',now(),now())"); err != nil {
			return err
		}
		for _, id := range []int64{1, 2} {
			if err := new(sSysPublish).markCollectDispatchReviewing(ctx, id, 3371); err != nil {
				return err
			}
			row, err := tx.GetOne("SELECT * FROM hg_youban_publish_collect_dispatch WHERE id=?", id)
			if err != nil {
				return err
			}
			if row["status"].String() != "reviewing" || row["error_message"].String() != "" || !row["finished_at"].IsNil() || row["review_id"].Int64() != 3371 {
				t.Fatalf("recovery failed: %+v", row)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
