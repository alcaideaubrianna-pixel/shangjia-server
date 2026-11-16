package sys

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestFailDeliveryKeepsTransientFailureRetryable(t *testing.T) {
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})

	ctx := context.Background()
	errTransient := errors.New("temporary downstream failure")
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec(`CREATE TEMP TABLE hg_tg_collector_delivery(
			id bigint primary key,
			attempt_count int not null,
			status text,
			next_run_at timestamp,
			lease_owner text,
			lease_until timestamp,
			error_message text,
			updated_at timestamp
		) ON COMMIT DROP`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_tg_collector_delivery(id, attempt_count, status) VALUES(1, 1, 'processing')`); err != nil {
			return err
		}

		got := NewCollector().failDelivery(ctx, 1, errTransient)
		if got == nil || errors.Is(got, asynq.SkipRetry) {
			t.Fatalf("transient delivery failure must remain retryable, got %v", got)
		}
		row, err := tx.GetOne(`SELECT status, next_run_at, error_message FROM hg_tg_collector_delivery WHERE id=1`)
		if err != nil {
			return err
		}
		if row["status"].String() != sysin.DeliveryStatusFailedRetry {
			t.Fatalf("status=%q, want %q", row["status"].String(), sysin.DeliveryStatusFailedRetry)
		}
		if row["next_run_at"].IsNil() {
			t.Fatal("retryable delivery must have next_run_at")
		}
		if row["error_message"].String() != errTransient.Error() {
			t.Fatalf("error_message=%q, want %q", row["error_message"].String(), errTransient.Error())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFailDeliveryMakesExhaustedFailureObservableAsSkipRetry(t *testing.T) {
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})

	ctx := context.Background()
	errPermanent := errors.New("permanent downstream failure")
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec(`CREATE TEMP TABLE hg_tg_collector_delivery(
			id bigint primary key,
			attempt_count int not null,
			status text,
			next_run_at timestamp,
			lease_owner text,
			lease_until timestamp,
			error_message text,
			updated_at timestamp
		) ON COMMIT DROP`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_tg_collector_delivery(id, attempt_count, status) VALUES(2, 5, 'processing')`); err != nil {
			return err
		}

		got := NewCollector().failDelivery(ctx, 2, errPermanent)
		if got == nil || !errors.Is(got, asynq.SkipRetry) {
			t.Fatalf("exhausted delivery failure must be permanent/skip-retry, got %v", got)
		}
		row, err := tx.GetOne(`SELECT status, next_run_at, error_message FROM hg_tg_collector_delivery WHERE id=2`)
		if err != nil {
			return err
		}
		if row["status"].String() != sysin.DeliveryStatusDead {
			t.Fatalf("status=%q, want %q", row["status"].String(), sysin.DeliveryStatusDead)
		}
		if !row["next_run_at"].IsNil() {
			t.Fatal("dead delivery must not have next_run_at")
		}
		if row["error_message"].String() != errPermanent.Error() {
			t.Fatalf("error_message=%q, want %q", row["error_message"].String(), errPermanent.Error())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
