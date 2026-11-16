package sys

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestTelegramFallbackAdmissionDelayIsBoundedByJobDeadline(t *testing.T) {
	for _, remaining := range []time.Duration{time.Hour, 10 * time.Second} {
		ctx, cancel := context.WithTimeout(context.Background(), remaining)
		err := telegramFallbackAdmissionWait(ctx)
		var deferred collectorin.AccountTaskAdmissionDeferred
		if !errors.As(err, &deferred) || !deferred.AccountTaskNotStarted() {
			t.Fatal("channel wait must report no external send was started")
		}
		var retry interface{ AccountTaskRetryDelay() time.Duration }
		if !errors.As(err, &retry) {
			t.Fatal("channel wait lost its retry delay")
		}
		delay := retry.AccountTaskRetryDelay()
		if delay <= 0 || delay > telegramStreamExecutionTTL || delay > remaining {
			t.Fatalf("admission delay escapes the persisted send budget: %s remaining=%s", delay, remaining)
		}
		cancel()
	}
}

func TestTelegramFallbackAdmissionCannotRestartExpiredOrMissingBudget(t *testing.T) {
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	for _, ctx := range []context.Context{expired, context.Background()} {
		err := telegramFallbackAdmissionWait(ctx)
		var deferred collectorin.AccountTaskAdmissionDeferred
		if err == nil || errors.As(err, &deferred) {
			t.Fatal("expired/missing budget must not authorize unlimited admission retries")
		}
	}
}

func TestTelegramFallbackAdmissionBudgetLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_ACCOUNT_PHASE_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL at 127.0.0.1:35432")
	}
	if os.Getenv("YOUBAN_FALLBACK_ADMISSION_CHILD") != "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTelegramFallbackAdmissionBudgetLocalPostgres$", "-test.count=1")
		cmd.Env = append(os.Environ(), "YOUBAN_FALLBACK_ADMISSION_CHILD=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fallback admission deadline: %v\n%s", err, out)
		}
		return
	}
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, query := range []string{
			"CREATE TEMP TABLE " + publishTgJobTable + "(id bigint PRIMARY KEY,tenant_id bigint,collect_event_id bigint,status text,send_deadline_at timestamp,updated_at timestamp) ON COMMIT DROP",
			"CREATE TEMP TABLE " + publishCollectEventTable + "(id bigint,tenant_id bigint,received_at timestamp,source_type text) ON COMMIT DROP",
			"INSERT INTO " + publishTgJobTable + " VALUES(42,7,0,'failed_retry',(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai')+INTERVAL '1 minute',NULL)",
		} {
			if _, err := tx.Exec(query); err != nil {
				return err
			}
		}
		before, err := tx.GetValue("SELECT send_deadline_at FROM " + publishTgJobTable + " WHERE id=42")
		if err != nil {
			return err
		}
		service := &sSysPublish{}
		for i := 0; i < 2; i++ {
			bounded, stop, err := service.telegramSendBudgetContext(ctx, telegramJobRecord{Id: 42})
			if err != nil {
				return err
			}
			if _, ok := bounded.Deadline(); !ok || bounded.Err() != nil {
				t.Fatal("queued failed_retry must read its existing live deadline even when UPDATE affects zero rows")
			}
			stop()
		}
		after, err := tx.GetValue("SELECT send_deadline_at FROM " + publishTgJobTable + " WHERE id=42")
		if err != nil {
			return err
		}
		if before.String() != after.String() {
			t.Fatal("admission retry extended the persisted Job budget")
		}
		if _, err := tx.Exec("UPDATE " + publishTgJobTable + " SET send_deadline_at=(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai')-INTERVAL '1 minute' WHERE id=42"); err != nil {
			return err
		}
		if _, stop, err := service.telegramSendBudgetContext(ctx, telegramJobRecord{Id: 42}); err == nil {
			stop()
			t.Fatal("expired queued Job received a fresh admission budget")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
