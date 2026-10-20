package sys

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
)

func TestTelegramChannelLockTimeoutLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires disposable PostgreSQL on 127.0.0.1:55484")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	probeCtx := ctx
	baseline, err := db.GetValue(ctx, "SELECT setting FROM pg_settings WHERE name='idle_in_transaction_session_timeout'")
	if err != nil {
		t.Fatal(err)
	}

	sentinel := errors.New("rollback lock holder")
	for _, original := range []string{"100ms", "0", "20min"} {
		t.Run(original, func(t *testing.T) {
			err := db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
				if _, err := tx.Exec("SELECT set_config('idle_in_transaction_session_timeout', ?, true)", original); err != nil {
					return err
				}
				if err := configureTelegramChannelLockTimeout(tx); err != nil {
					return err
				}
				val, err := tx.GetValue("SELECT setting FROM pg_settings WHERE name='idle_in_transaction_session_timeout'")
				if err != nil {
					return err
				}
				switch original {
				case "100ms":
					if val.Int64() != telegramChannelLeaseTTL.Milliseconds() {
						t.Fatalf("guard timeout=%s", val.String())
					}
				case "0":
					if val.Int64() != 0 {
						t.Fatal("disabled timeout changed")
					}
				case "20min":
					if val.Int64() != 1200000 {
						t.Fatal("longer timeout shortened")
					}
				}
				if _, err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext('telegram_channel_timeout_regression'))"); err != nil {
					return err
				}
				// Simulated Telegram I/O exceeds the original idle timeout. Another
				// worker must still be unable to acquire this channel's lock.
				time.Sleep(250 * time.Millisecond)
				contender, err := db.GetValue(probeCtx, "SELECT pg_try_advisory_xact_lock(hashtext('telegram_channel_timeout_regression'))")
				if err != nil {
					return err
				}
				if contender.Bool() {
					t.Fatal("channel lock released while callback still running")
				}
				if _, err := tx.GetValue("SELECT 1"); err != nil {
					return err
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) {
				t.Fatalf("guard transaction failed: %v", err)
			}
			released, err := db.GetValue(ctx, "SELECT pg_try_advisory_xact_lock(hashtext('telegram_channel_timeout_regression'))")
			if err != nil {
				t.Fatal(err)
			}
			if !released.Bool() {
				t.Fatal("rollback did not release lock")
			}
			restored, err := db.GetValue(ctx, "SELECT setting FROM pg_settings WHERE name='idle_in_transaction_session_timeout'")
			if err != nil {
				t.Fatal(err)
			}
			if restored.String() != baseline.String() {
				t.Fatal("channel timeout leaked into pooled connection")
			}

		})
	}
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec("SET LOCAL idle_in_transaction_session_timeout='100ms'"); err != nil {
			return err
		}
		val, err := tx.GetValue("SELECT setting FROM pg_settings WHERE name='idle_in_transaction_session_timeout'")
		if err != nil {
			return err
		}
		if val.Int64() != 100 {
			t.Fatal("ordinary timeout changed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
