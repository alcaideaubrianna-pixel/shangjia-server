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

func TestFailEventRejectsStaleExecution(t *testing.T) {
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	defer func() { _ = g.DB().GetCore().ClearTableFieldsAll(context.Background()) }()

	cause := errors.New("old event execution failed")
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, statement := range []string{
			`CREATE TEMP TABLE hg_tg_collector_event(
				id bigint PRIMARY KEY, attempt_count int NOT NULL, status text,
				next_run_at timestamp, lease_owner text, lease_until timestamp,
				error_message text, updated_at timestamp
			) ON COMMIT DROP`,
			`INSERT INTO hg_tg_collector_event(id,attempt_count,status,lease_owner)
			 VALUES (1,5,'processing','new-owner'),(2,1,'ready',''),(3,1,'processing','current-owner')`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		collector := NewCollector()
		for _, id := range []int64{1, 2} {
			got := collector.failEvent(ctx, id, "old-owner", cause)
			if got == nil || errors.Is(got, asynq.SkipRetry) {
				t.Fatalf("stale event execution must not confirm failure/dead: id=%d err=%v", id, got)
			}
			row, err := tx.GetOne(`SELECT status,lease_owner,error_message FROM hg_tg_collector_event WHERE id=?`, id)
			if err != nil {
				return err
			}
			status, owner := sysin.EventStatusProcessing, "new-owner"
			if id == 2 {
				status, owner = sysin.EventStatusReady, ""
			}
			if row["status"].String() != status || row["lease_owner"].String() != owner || !row["error_message"].IsNil() {
				t.Fatalf("stale execution modified event %d", id)
			}
		}
		if got := collector.failEvent(ctx, 3, "current-owner", cause); !errors.Is(got, cause) {
			t.Fatalf("current owner must retain retryable error: %v", got)
		}
		row, err := tx.GetOne(`SELECT status,next_run_at,lease_owner FROM hg_tg_collector_event WHERE id=3`)
		if err != nil {
			return err
		}
		if row["status"].String() != sysin.EventStatusFailedRetry || row["next_run_at"].IsNil() || row["lease_owner"].String() != "" {
			t.Fatal("current event execution did not persist retryable state")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

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
		if _, err := tx.Exec(`INSERT INTO hg_tg_collector_delivery(id, attempt_count, status, lease_owner) VALUES(1, 1, 'processing', 'test-execution-1')`); err != nil {
			return err
		}

		got := NewCollector().failDelivery(ctx, 1, "test-execution-1", errTransient)
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
		if _, err := tx.Exec(`INSERT INTO hg_tg_collector_delivery(id, attempt_count, status, lease_owner) VALUES(2, 5, 'processing', 'test-execution-2')`); err != nil {
			return err
		}

		// An exhausted old execution must not kill the new owner's delivery.
		stale := NewCollector().failDelivery(ctx, 2, "stale-execution", errPermanent)
		if stale == nil || errors.Is(stale, asynq.SkipRetry) {
			t.Fatalf("stale owner must not report a dead transition, got %v", stale)
		}
		unchanged, err := tx.GetOne(`SELECT status, lease_owner FROM hg_tg_collector_delivery WHERE id=2`)
		if err != nil {
			return err
		}
		if unchanged["status"].String() != sysin.DeliveryStatusProcessing || unchanged["lease_owner"].String() != "test-execution-2" {
			t.Fatal("stale execution changed current owner or status")
		}

		got := NewCollector().failDelivery(ctx, 2, "test-execution-2", errPermanent)
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
