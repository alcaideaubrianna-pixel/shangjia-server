package sys

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestAccountTaskAdmissionLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_ACCOUNT_PHASE_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL at 127.0.0.1:35432")
	}
	if os.Getenv("YOUBAN_ADMISSION_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAccountTaskAdmissionLocalPostgres$", "-test.count=1")
		cmd.Env = append(os.Environ(), "YOUBAN_ADMISSION_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("admission retry persistence: %v\n%s", err, out)
		}
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_tg_collector_account_task(id bigint PRIMARY KEY,status text,attempt_count int,max_attempts int,next_run_at timestamp,lease_owner text,lease_epoch bigint,lease_until timestamp,error_message text,updated_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_tg_collector_account_task VALUES(1,'processing',5,5,NULL,'test-owner',7,NULL,'',NOW())`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		s := new(sAccountTasks)
		in := &sysin.AccountTaskFailure{TaskID: 1, Lease: &sysin.AccountLease{InstanceID: "test-owner", Epoch: 7}, Cause: context.DeadlineExceeded, RetryDelay: time.Second, NotStarted: true}
		if err := s.Fail(ctx, in); err != nil {
			return err
		}
		row, err := tx.GetOne(`SELECT status,attempt_count FROM hg_tg_collector_account_task WHERE id=1`)
		if err != nil || row["status"].String() != sysin.AccountTaskStatusFailedRetry || row["attempt_count"].Int() != 4 {
			t.Fatalf("admission consumed retry: row=%v err=%v", row, err)
		}
		if err := s.Fail(ctx, in); err == nil {
			t.Fatal("stale admission result changed a released lease")
		}
		if _, err := tx.Exec(`UPDATE hg_tg_collector_account_task SET status='processing',attempt_count=5,lease_owner='test-owner',lease_epoch=7 WHERE id=1`); err != nil {
			return err
		}
		in.NotStarted = false
		if err := s.Fail(ctx, in); err != nil {
			return err
		}
		row, err = tx.GetOne(`SELECT status,attempt_count FROM hg_tg_collector_account_task WHERE id=1`)
		if err != nil || row["status"].String() != sysin.AccountTaskStatusFinalizing || row["attempt_count"].Int() != 5 {
			t.Fatalf("execution failure escaped retry cap: row=%v err=%v", row, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
