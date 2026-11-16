package sys

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func deliveryRecoveryFixtureRow(id int64, priority int, created string) gdb.Record {
	return gdb.Record{
		"id": gvar.New(id), "priority": gvar.New(priority),
		deliveryRecoveryCreatedField: gvar.New(created),
	}
}

func TestDeliveryRecoveryCursorKeepsWindowAndMixedPriorityPosition(t *testing.T) {
	cursor := deliveryRecoveryCursor{
		UntilTime: "2026-11-17 06:00:00.000000", StaleBefore: "2026-11-17 05:59:45.000000",
	}
	rows := gdb.Result{
		deliveryRecoveryFixtureRow(3, 200, "2026-11-17 02:00:00.000000"),
		deliveryRecoveryFixtureRow(4, 100, "2026-11-17 00:00:00.000000"),
	}
	var submitted []int64
	next, err := walkDeliveryRecoveryPage(rows, cursor, 2, func(row gdb.Record) error {
		submitted = append(submitted, row["id"].Int64())
		return nil
	})
	if err != nil || !reflect.DeepEqual(submitted, []int64{3, 4}) {
		t.Fatalf("submitted=%v err=%v", submitted, err)
	}
	if next.UntilTime != cursor.UntilTime || next.StaleBefore != cursor.StaleBefore ||
		next.AfterPriority != 100 || next.AfterID != 4 || next.AfterCreatedAt != "2026-11-17 00:00:00.000000" {
		t.Fatalf("lost the fixed window or mixed-priority position: %+v", next)
	}
}

func TestDeliveryRecoveryCursorPreservesProgressOnInterruptedPage(t *testing.T) {
	for _, cause := range []error{
		context.Canceled, context.DeadlineExceeded,
		errors.Join(errCollectorRecoveryRedisUnavailable, errors.New("transport unavailable")),
	} {
		t.Run(cause.Error(), func(t *testing.T) {
			cursor := deliveryRecoveryCursor{UntilTime: "2026-11-17 06:00:00.000000"}
			rows := gdb.Result{
				deliveryRecoveryFixtureRow(1, 100, "2026-11-16 01:00:00.000000"),
				deliveryRecoveryFixtureRow(2, 100, "2026-11-16 01:00:00.000000"),
				deliveryRecoveryFixtureRow(3, 50, "2026-11-16 00:00:00.000000"),
			}
			var attempted []int64
			next, err := walkDeliveryRecoveryPage(rows, cursor, 3, func(row gdb.Record) error {
				id := row["id"].Int64()
				attempted = append(attempted, id)
				if id == 2 {
					return cause
				}
				return nil
			})
			if !errors.Is(err, cause) || next.AfterID != 1 || next.UntilTime != cursor.UntilTime {
				t.Fatalf("interrupted page skipped an unconfirmed row or lost progress: %+v err=%v", next, err)
			}
			if !reflect.DeepEqual(attempted, []int64{1, 2}) {
				t.Fatalf("continued after interruption: %v", attempted)
			}
		})
	}
}

func TestDeliveryRecoveryCursorOrdinaryFailureDoesNotPinLaterRows(t *testing.T) {
	cursor := deliveryRecoveryCursor{UntilTime: "2026-11-17 06:00:00.000000"}
	rows := gdb.Result{
		deliveryRecoveryFixtureRow(1, 100, "2026-11-16 01:00:00.000000"),
		deliveryRecoveryFixtureRow(2, 50, "2026-11-16 00:00:00.000000"),
	}
	cause := errors.New("one delivery could not be read")
	var submitted []int64
	next, err := walkDeliveryRecoveryPage(rows, cursor, 2, func(row gdb.Record) error {
		id := row["id"].Int64()
		submitted = append(submitted, id)
		if id == 1 {
			return cause
		}
		return nil
	})
	if !errors.Is(err, cause) || next.AfterID != 2 || !reflect.DeepEqual(submitted, []int64{1, 2}) {
		t.Fatalf("one row pinned the whole page: cursor=%+v submitted=%v err=%v", next, submitted, err)
	}
	wrapped, err := walkDeliveryRecoveryPage(nil, next, 2, func(gdb.Record) error {
		t.Fatal("empty page submitted work")
		return nil
	})
	if err != nil || wrapped != (deliveryRecoveryCursor{}) {
		t.Fatalf("failed durable rows would not be revisited in a new sweep: %+v err=%v", wrapped, err)
	}
}

func TestDeliveryRecoveryCursorRejectsMissingKeyBeforeSubmission(t *testing.T) {
	cursor := deliveryRecoveryCursor{UntilTime: "2026-11-17 06:00:00.000000"}
	next, err := walkDeliveryRecoveryPage(
		gdb.Result{deliveryRecoveryFixtureRow(1, 100, "")}, cursor, 1,
		func(gdb.Record) error { t.Fatal("submitted a row without a cursor key"); return nil },
	)
	if err == nil || next != cursor {
		t.Fatalf("invalid pagination key advanced the cursor: %+v err=%v", next, err)
	}
}

// Opt-in CI/local fixture only. It uses a disposable loopback database and
// temporary tables, never the configured application or production database.
func TestDeliveryRecoveryCursorPagesLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{
		Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres",
	}})
	defer func() { _ = g.DB().GetCore().ClearTableFieldsAll(context.Background()) }()
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, statement := range []string{
			`CREATE TEMP TABLE hg_tg_collector_delivery(
				id bigint PRIMARY KEY,tenant_id bigint,account_id bigint,source_id bigint,
				scheduler_pool text,status text,priority int,attempt_count int,
				created_at timestamp,updated_at timestamp,next_run_at timestamp
			) ON COMMIT DROP`,
			`INSERT INTO hg_tg_collector_delivery VALUES
			 (1,78,0,351,'realtime','pending',200,0,'2026-11-17 01:00','2026-11-17 01:00',NULL),
			 (2,78,0,351,'realtime','pending',200,0,'2026-11-17 01:00','2026-11-17 01:00',NULL),
			 (3,78,0,351,'realtime','failed_retry',200,1,'2026-11-17 02:00','2026-11-17 02:00','2026-11-17 02:00'),
			 (4,78,0,351,'realtime','pending',100,0,'2026-11-17 00:00','2026-11-17 00:00',NULL),
			 (5,78,0,351,'realtime','failed_retry',50,1,'2026-11-17 00:00','2026-11-17 00:00','2026-11-17 01:00'),
			 (6,78,0,351,'realtime','pending',0,0,'2026-11-17 00:00','2026-11-17 00:00',NULL),
			 (7,78,0,352,'realtime','pending',200,0,'2026-11-17 00:00','2026-11-17 00:00',NULL),
			 (8,79,0,351,'realtime','pending',200,0,'2026-11-17 00:00','2026-11-17 00:00',NULL),
			 (9,78,0,351,'realtime','pending',200,0,'2026-11-17 07:00','2026-11-17 01:00',NULL),
			 (10,78,0,351,'realtime','pending',200,0,'2026-11-17 03:00','2026-11-17 07:00',NULL),
			 (11,78,0,351,'realtime','failed_retry',200,1,'2026-11-17 03:00','2026-11-17 03:00','2026-11-17 07:00'),
			 (12,78,0,351,'realtime','failed_retry',200,5,'2026-11-17 03:00','2026-11-17 03:00','2026-11-17 03:00'),
			 (13,78,0,351,'realtime','failed_retry',200,1,'2026-11-17 03:00','2026-11-17 03:00',NULL),
			 (14,78,0,351,'realtime','processing',200,1,'2026-11-17 03:00','2026-11-17 03:00',NULL),
			 (15,78,0,351,'history','pending',200,0,'2026-11-17 00:00','2026-11-17 00:00',NULL),
			 (16,78,9,351,'realtime','pending',200,0,'2026-11-17 00:00','2026-11-17 00:00',NULL)`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		cursor := deliveryRecoveryCursor{
			UntilTime: "2026-11-17 06:00:00.000000", StaleBefore: "2026-11-17 05:59:45.000000",
		}
		scope := collectorRecoveryScope{TenantID: 78, SourceID: 351}
		var actual []int64
		for page := 0; page < 4; page++ {
			rows, err := deliveryRecoveryPage(ctx, scope, "realtime", cursor, 2)
			if err != nil {
				return err
			}
			cursor, err = walkDeliveryRecoveryPage(rows, cursor, 2, func(row gdb.Record) error {
				actual = append(actual, row["id"].Int64())
				return nil
			})
			if err != nil {
				return err
			}
		}
		if !reflect.DeepEqual(actual, []int64{1, 2, 3, 4, 5, 6}) || cursor != (deliveryRecoveryCursor{}) {
			t.Fatalf("pagination lost ordering/scope/due filtering: ids=%v cursor=%+v", actual, cursor)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
