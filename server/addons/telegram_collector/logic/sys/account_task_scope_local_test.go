package sys

import (
	"context"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestAccountTaskMaintenanceScopeLocalPostgres(t *testing.T) {
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
			`CREATE TEMP TABLE hg_tg_collector_account_task(
 id bigint PRIMARY KEY,account_id bigint,media_owner_account_id bigint DEFAULT 0,task_type text DEFAULT 'history_page',
 status text DEFAULT 'processing',attempt_count int DEFAULT 0,max_attempts int DEFAULT 5,
 lease_until timestamp DEFAULT '2020-01-01',lease_owner text DEFAULT '',lease_epoch bigint DEFAULT 1,
 next_run_at timestamp,error_message text DEFAULT '',created_at timestamp DEFAULT '2020-01-01',updated_at timestamp DEFAULT '2020-01-01') ON COMMIT DROP`,
			`INSERT INTO hg_tg_collector_account_task(id,account_id) VALUES(1,1),(2,2)`,
			`INSERT INTO hg_tg_collector_account_task(id,account_id,media_owner_account_id,task_type) VALUES(3,2,1,'media_download'),(4,2,1,'history_page')`,
			`INSERT INTO hg_tg_collector_account_task(id,account_id,media_owner_account_id,task_type,status) VALUES(5,1,0,'media_download','pending'),(6,2,0,'media_download','pending'),(7,2,1,'media_download','pending')`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		tasks := &sAccountTasks{}
		recovered, err := tasks.RecoverExpired(ctx, 1, 100)
		if err != nil {
			return err
		}
		if recovered != 2 {
			t.Fatalf("account owner and media owner must recover exactly two tasks: %d", recovered)
		}
		expired, err := tasks.ExpirePending(ctx, 1, sysin.AccountTaskTypeMediaDownload, time.Now().Add(-time.Hour), 100)
		if err != nil {
			return err
		}
		if expired != 2 {
			rows, _ := tx.Ctx(ctx).GetAll(`SELECT id,account_id,media_owner_account_id,task_type,status,updated_at,next_run_at FROM hg_tg_collector_account_task ORDER BY id`)
			t.Fatalf("pending expiry crossed or missed account scope: %d rows=%v", expired, rows)
		}
		finalizing, err := tasks.ListFinalizing(ctx, 1, 100)
		if err != nil {
			return err
		}
		ids := make([]int64, 0, len(finalizing))
		for _, task := range finalizing {
			ids = append(ids, task.ID)
		}
		if !reflect.DeepEqual(ids, []int64{5, 7}) {
			t.Fatalf("finalizing scope leaked: %v", ids)
		}
		for _, id := range []int64{2, 4, 6} {
			row, err := tx.Ctx(ctx).GetOne(`SELECT status FROM hg_tg_collector_account_task WHERE id=?`, id)
			if err != nil {
				return err
			}
			want := sysin.AccountTaskStatusProcessing
			if id == 6 {
				want = sysin.AccountTaskStatusPending
			}
			if row["status"].String() != want {
				t.Fatalf("unrelated task %d changed: %v", id, row)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
