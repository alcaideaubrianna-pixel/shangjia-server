package sys

import (
	"context"
	"os"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
)

func TestProfileRankOrderLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires disposable PostgreSQL at 127.0.0.1:55484")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("CREATE TEMP TABLE rank_profile(id bigint); INSERT INTO rank_profile VALUES(7),(9),(42)"); err != nil {
			return err
		}
		for _, ids := range [][]int64{{42, 7}, {0, -1}, nil} {
			rows, err := applyProfileRankOrder(tx.Model("rank_profile p").Ctx(ctx), "p.id", ids).OrderAsc("p.id").Fields("p.id").All()
			if err != nil {
				return err
			}
			want := []int64{7, 9, 42}
			if len(ids) > 0 && ids[0] == 42 {
				want = []int64{42, 7, 9}
			}
			for i, row := range rows {
				if row["id"].Int64() != want[i] {
					t.Fatalf("ranked rows=%v want=%v", rows, want)
				}
			}
			if len(rows) != len(want) {
				t.Fatalf("rows=%v", rows)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
