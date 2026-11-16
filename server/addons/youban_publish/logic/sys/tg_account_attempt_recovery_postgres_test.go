package sys

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestTelegramAccountAttemptExcludedFromOrdinaryRecoveryLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_ACCOUNT_PHASE_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL at 127.0.0.1:35432")
	}
	// Isolate default DB configuration from the rest of the publisher tests.
	if os.Getenv("YOUBAN_ACCOUNT_RECOVERY_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTelegramAccountAttemptExcludedFromOrdinaryRecoveryLocalPostgres$", "-test.count=1")
		cmd.Env = append(os.Environ(), "YOUBAN_ACCOUNT_RECOVERY_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("Account attempt recovery isolation: %v\n%s", err, out)
		}
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{
		Type: "pgsql", Host: "127.0.0.1", Port: "35432",
		User: os.Getenv("USER"), Name: "postgres",
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, ddl := range []string{
			"CREATE TEMP TABLE " + publishTgJobTable + "(id bigint PRIMARY KEY,status text,last_dispatch_error text) ON COMMIT DROP",
			"CREATE TEMP TABLE " + publishTgAttemptTable + "(id bigint PRIMARY KEY,job_id bigint,phase text,status text,updated_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE " + publishTgAttemptMessageTable + "(attempt_id bigint,source text) ON COMMIT DROP",
		} {
			if _, err := tx.Exec(ddl); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("INSERT INTO "+publishTgJobTable+" VALUES(42,'sending',?),(43,'sending','')", telegramMediaFallbackReason("verify")); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO " + publishTgAttemptTable + " VALUES(101,42,'verify','sending',LOCALTIMESTAMP-INTERVAL '1 hour'),(102,43,'display','sending',LOCALTIMESTAMP+INTERVAL '1 hour')"); err != nil {
			return err
		}
		// Account owns this stale Attempt. Ordinary recovery must not change it
		// to awaiting_webhook and prematurely fail the Account Job after 15s.
		if err := new(sSysPublish).recoverStaleSendingTelegramAttempts(ctx, 1); err != nil {
			return err
		}
		status, err := tx.GetValue("SELECT status FROM " + publishTgAttemptTable + " WHERE id=101")
		if err != nil {
			return err
		}
		if status.String() != telegramAttemptStatusSending {
			t.Fatalf("ordinary recovery stole Account Attempt ownership: %s", status.String())
		}
		status, err = tx.GetValue("SELECT status FROM " + publishTgJobTable + " WHERE id=42")
		if err == nil && status.String() != telegramJobStatusSending {
			t.Fatalf("ordinary recovery changed Account Job status: %s", status.String())
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}
