package install

import (
	"context"
	"os"
	"testing"

	_ "hotgo/addons/youban_publish/crons"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/cron"
	"hotgo/internal/model/entity"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcron"
)

func TestDailyReportCronInstallationLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_COLLECT_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec(`CREATE TEMP TABLE hg_sys_cron(id bigserial PRIMARY KEY,group_id bigint,title text,name text,params text,pattern text,policy bigint,count bigint,sort int,remark text,status int,created_at timestamp,updated_at timestamp) ON COMMIT DROP`); err != nil {
			return err
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		defer g.DB().GetCore().ClearTableFieldsAll(ctx)
		var firstID int64
		for pass := 0; pass < 2; pass++ {
			if err := ensureDailyReportCron(ctx); err != nil {
				return err
			}
			var rows []*entity.SysCron
			if err := dao.SysCron.Ctx(ctx).Scan(&rows); err != nil {
				return err
			}
			if len(rows) != 1 {
				t.Fatalf("installation created %d cron rows", len(rows))
			}
			row := rows[0]
			defer cron.Delete(row)
			if pass == 0 {
				firstID = row.Id
			} else if row.Id != firstID {
				t.Fatal("upgrade replaced cron identity")
			}
			if row.Name != "youbanPublishDailyReport" || row.Pattern != "@every 1m" || row.Policy != consts.CronPolicySingle || row.Status != consts.StatusEnabled {
				t.Fatalf("invalid report schedule: %+v", row)
			}
			if gcron.Search(cron.GenCronSn(row)) == nil {
				t.Fatal("database cron was not registered in scheduler")
			}
			if row.CreatedAt == nil || row.UpdatedAt == nil {
				t.Fatal("cron timestamps were not maintained")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
