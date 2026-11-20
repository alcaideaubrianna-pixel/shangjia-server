package sys

import (
	"context"
	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
	"os"
	"testing"
	"time"
)

func TestPermissionRetentionPaginationLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_PERMISSION_LOCAL_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			"CREATE TEMP TABLE " + publishTgJobTable + " (id bigint PRIMARY KEY,status text,last_dispatch_error text,updated_at timestamp,send_phase text DEFAULT '')",
			"CREATE TEMP TABLE " + publishTgAttemptTable + " (id bigint,job_id bigint)",
			"CREATE TEMP TABLE " + publishTgMessageTable + " (id bigint,job_id bigint)",
			"CREATE TEMP TABLE " + publishTgOutboxTable + " (id bigint,job_id bigint)",
			"CREATE TEMP TABLE " + publishSuccessRecordTable + " (id bigint,job_id bigint)",
		} {
			if _, err = tx.Exec(sql); err != nil {
				return err
			}
		}
		var count int
		// Evidence-heavy pages must not starve later deletable jobs. Preserve
		// microseconds when the shared cursor is serialized between transactions.
		if _, err = tx.Exec("INSERT INTO "+publishTgJobTable+" (id,status,last_dispatch_error,updated_at) SELECT 10000+n,'superseded',?,timestamp '2020-01-01 00:00:00.123456' FROM generate_series(1,201) n", channelPermissionCancelledMessage); err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO " + publishTgAttemptTable + " (id,job_id) SELECT 10000+n,10000+n FROM generate_series(1,200) n"); err != nil {
			return err
		}
		cursor := permissionRetentionCursor{}
		cutoff := gtime.New("2021-01-01")
		if err = prunePermissionCancelledJobsTx(ctx, tx, cutoff, &cursor); err != nil {
			return err
		}
		if cursor.ID != 10200 || cursor.UpdatedAt != "2020-01-01 00:00:00.123456" {
			t.Fatalf("cursor did not advance precisely past evidence page: %+v", cursor)
		}
		if err = prunePermissionCancelledJobsTx(ctx, tx, cutoff, &cursor); err != nil {
			return err
		}
		count, err = tx.Model(publishTgJobTable).Where("id", 10201).Count()
		if err != nil || count != 0 {
			t.Fatalf("eligible next page starved: count=%d err=%v", count, err)
		}
		count, err = tx.Model(publishTgJobTable).WhereBetween("id", 10001, 10200).Count()
		if err != nil || count != 200 {
			t.Fatalf("jobs with evidence deleted: count=%d err=%v", count, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
