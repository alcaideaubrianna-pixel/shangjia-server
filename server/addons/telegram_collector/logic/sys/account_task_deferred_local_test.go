package sys

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"hotgo/addons/telegram_collector/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestAccountTaskChannelDeferredLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_ACCOUNT_PHASE_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL at 127.0.0.1:35432")
	}
	if os.Getenv("YOUBAN_CHANNEL_DEFERRED_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAccountTaskChannelDeferredLocalPostgres$", "-test.count=1")
		cmd.Env = append(os.Environ(), "YOUBAN_CHANNEL_DEFERRED_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("channel admission persistence: %v\n%s", err, out)
		}
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, query := range []string{
			`CREATE TEMP TABLE hg_tg_collector_account_task(id bigint PRIMARY KEY,status text,attempt_count int,max_attempts int,next_run_at timestamp,lease_owner text,lease_epoch bigint,lease_until timestamp,error_message text,updated_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_tg_collector_account_task VALUES(1,'pending',0,5,NULL,'',0,NULL,'',NOW())`,
		} {
			if _, err := tx.Exec(query); err != nil {
				return err
			}
		}
		service := new(sAccountTasks)
		in := &sysin.AccountTaskFailure{
			TaskID: 1, Lease: &sysin.AccountLease{InstanceID: "owner", Epoch: 7},
			Cause: context.DeadlineExceeded, RetryDelay: 17 * time.Second,
			NotStarted: true,
		}
		// More than MaxAttempts admissions must leave all five actual send
		// attempts available; each failed admission refunds exactly its claim.
		for i := 0; i < 8; i++ {
			if _, err := tx.Exec(`UPDATE hg_tg_collector_account_task SET status='processing',attempt_count=attempt_count+1,lease_owner='owner',lease_epoch=7,lease_until=LOCALTIMESTAMP+INTERVAL '1 minute' WHERE id=1`); err != nil {
				return err
			}
			if err := service.Fail(ctx, in); err != nil {
				return err
			}
			row, err := tx.GetOne(`SELECT status,attempt_count,next_run_at FROM hg_tg_collector_account_task WHERE id=1`)
			if err != nil {
				return err
			}
			if row["status"].String() != sysin.AccountTaskStatusFailedRetry || row["attempt_count"].Int() != 0 || row["next_run_at"].IsNil() {
				t.Fatalf("admission %d consumed the send retry budget: %v", i+1, row)
			}
			if err := service.Fail(ctx, in); err == nil {
				t.Fatal("duplicate released-lease result refunded an attempt twice")
			}
		}
		if _, err := tx.Exec(`UPDATE hg_tg_collector_account_task SET status='processing',attempt_count=2,lease_owner='new-owner',lease_epoch=8 WHERE id=1`); err != nil {
			return err
		}
		if err := service.Fail(ctx, in); err == nil {
			t.Fatal("old account lease modified a later owner")
		}
		if _, err := tx.Exec(`UPDATE hg_tg_collector_account_task SET status='processing',attempt_count=5,lease_owner='owner',lease_epoch=7 WHERE id=1`); err != nil {
			return err
		}
		in.NotStarted = false
		if err := service.Fail(ctx, in); err != nil {
			return err
		}
		row, err := tx.GetOne(`SELECT status,attempt_count FROM hg_tg_collector_account_task WHERE id=1`)
		if err != nil {
			return err
		}
		if row["status"].String() != sysin.AccountTaskStatusFinalizing || row["attempt_count"].Int() != 5 {
			t.Fatalf("real send failure escaped the original retry cap: %v", row)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
