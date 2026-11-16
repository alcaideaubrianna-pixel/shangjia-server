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

	"hotgo/addons/telegram_collector/model/input/sysin"
)

func TestEventRecoveryBranchLimits(t *testing.T) {
	for _, tc := range []struct {
		limit, received, retry int
	}{
		{0, 1, 1}, {1, 1, 1}, {2, 1, 1}, {3, 2, 1},
		{200, 100, 100}, {1000, 500, 500}, {1001, 500, 500},
	} {
		received, retry := eventRecoveryBranchLimits(tc.limit)
		if received != tc.received || retry != tc.retry {
			t.Fatalf("limit=%d received=%d retry=%d", tc.limit, received, retry)
		}
	}
}

func TestEventRecoveryWindowIsFixedUntilWrap(t *testing.T) {
	received := eventRecoveryCursor{UntilTime: "2026-11-17 05:59:45.000000"}
	row := gdb.Record{
		"id": gvar.New(12), "updated_at": gvar.New("2026-10-01 01:02:03.123456"),
		eventRecoveryTimeField: gvar.New("2026-10-01 01:02:03.123456"),
	}
	next := advanceEventRecoveryCursor(received, sysin.EventStatusReceived, row)
	if next.UntilTime != received.UntilTime || next.AfterTime != "2026-10-01 01:02:03.123456" || next.AfterID != 12 {
		t.Fatalf("old received row changed or escaped the fixed window: %+v", next)
	}
}

func TestEventRecoveryDuplicateHandoffAdvancesAndFailureDoesNotSkip(t *testing.T) {
	cursor := eventRecoveryCursor{UntilTime: "2026-11-17 06:00:00.000000"}
	rows := gdb.Result{
		{"id": gvar.New(1), "priority": gvar.New(0), eventRecoveryTimeField: gvar.New("2026-11-16 01:00:00.000000")},
		{"id": gvar.New(2), "priority": gvar.New(50), eventRecoveryTimeField: gvar.New("2026-11-16 01:00:00.000000")},
	}
	var handedOff []int64
	next, err := enqueueEventRecoveryPage(rows, sysin.EventStatusReceived, cursor, 2, func(id int64, _ int) error {
		// A duplicate task has the same nil result as a new enqueue.
		handedOff = append(handedOff, id)
		return nil
	})
	if err != nil || next.AfterID != 2 || next.UntilTime != cursor.UntilTime || !reflect.DeepEqual(handedOff, []int64{1, 2}) {
		t.Fatalf("duplicate handoff stalled cursor: %+v ids=%v err=%v", next, handedOff, err)
	}
	cause := errors.New("queue unavailable")
	partial, err := enqueueEventRecoveryPage(rows, sysin.EventStatusReceived, cursor, 2, func(id int64, _ int) error {
		if id == 2 {
			return cause
		}
		return nil
	})
	if !errors.Is(err, cause) || partial.AfterID != 1 || partial.UntilTime != cursor.UntilTime {
		t.Fatalf("failed enqueue skipped a row or lost progress: %+v err=%v", partial, err)
	}
	wrapped, err := enqueueEventRecoveryPage(nil, sysin.EventStatusReceived, next, 2, func(int64, int) error {
		t.Fatal("empty page must not enqueue")
		return nil
	})
	if err != nil || wrapped != (eventRecoveryCursor{}) {
		t.Fatalf("completed sweep did not wrap: %+v err=%v", wrapped, err)
	}
}

// Opt-in only, on disposable loopback PostgreSQL. Temporary tables are rolled
// back/dropped; this does not start workers, access Telegram or modify production.
func TestEventRecoveryIndexedPagesLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	defer func() { _ = g.DB().GetCore().ClearTableFieldsAll(context.Background()) }()
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		var firstReceived, firstRetry eventRecoveryCursor
		for _, zone := range []string{"UTC", "Asia/Shanghai"} {
			if _, err := tx.Exec("SET LOCAL TIME ZONE '" + zone + "'"); err != nil {
				return err
			}
			received, err := newEventRecoveryCursor(ctx, sysin.EventStatusReceived)
			if err != nil {
				return err
			}
			retry, err := newEventRecoveryCursor(ctx, sysin.EventStatusFailedRetry)
			if err != nil {
				return err
			}
			expected, err := tx.GetValue("SELECT to_char(CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD HH24:MI:SS.US')")
			if err != nil {
				return err
			}
			gap, err := tx.GetValue("SELECT EXTRACT(EPOCH FROM (?::timestamp - ?::timestamp))", retry.UntilTime, received.UntilTime)
			if err != nil {
				return err
			}
			if retry.UntilTime != expected.String() || gap.Int64() != 15 {
				t.Fatalf("cutoff did not follow business wall clock zone=%s received=%+v retry=%+v gap=%v", zone, received, retry, gap)
			}
			if zone == "UTC" {
				firstReceived, firstRetry = received, retry
			} else if received != firstReceived || retry != firstRetry {
				// CURRENT_TIMESTAMP is fixed within this transaction. A session
				// zone change must not alter either branch's fixed upper bound.
				t.Fatalf("session zone changed cutoff: UTC=(%+v,%+v) Shanghai=(%+v,%+v)",
					firstReceived, firstRetry, received, retry)
			}
		}
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_tg_collector_event(id bigint PRIMARY KEY,priority int,status text,attempt_count int,updated_at timestamp,next_run_at timestamp) ON COMMIT DROP`,
			`ALTER TABLE hg_tg_collector_event ADD COLUMN tenant_id bigint DEFAULT 1, ADD COLUMN account_id bigint DEFAULT 0, ADD COLUMN source_id bigint DEFAULT 0`,
			`CREATE INDEX ON hg_tg_collector_event(updated_at,id) WHERE status='received'`,
			`CREATE INDEX ON hg_tg_collector_event(next_run_at,id) WHERE status='failed_retry'`,
			`INSERT INTO hg_tg_collector_event(id,priority,status,attempt_count,updated_at,next_run_at) VALUES
			 (1,0,'received',0,'2020-01-01',NULL),
			 (2,0,'received',5,'2020-01-01',NULL),
			 (3,50,'received',0,'2020-01-01',NULL),
			 (4,50,'received',0,'2026-11-17 07:00:00',NULL),
			 (10,0,'failed_retry',4,'2026-11-17','2026-11-17 01:00:00'),
			 (11,50,'failed_retry',5,'2026-11-17','2026-11-17 01:00:00'),
			 (12,50,'failed_retry',0,'2026-11-17','2026-11-17 07:00:00'),
			 (13,50,'failed_retry',0,'2026-11-17',NULL),
			 (14,0,'failed_retry',0,'2026-11-17','2026-11-17 01:00:00'),
			 (15,50,'processing',0,'2020-01-01','2020-01-01')`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		cutoff := eventRecoveryCursor{UntilTime: "2026-11-17 06:00:00.000000"}
		first, err := eventRecoveryPage(ctx, sysin.EventStatusReceived, cutoff, 2, collectorRecoveryScope{TenantID: 1})
		if err != nil {
			return err
		}
		if len(first) != 2 || first[0]["id"].Int64() != 1 || first[1]["id"].Int64() != 2 {
			t.Fatalf("old received rows lost or priority sorted ahead of indexed order: %v", first)
		}
		// Leave all rows received to simulate Redis duplicates at the head.
		cursor, err := enqueueEventRecoveryPage(first, sysin.EventStatusReceived, cutoff, 2, func(int64, int) error { return nil })
		if err != nil {
			return err
		}
		second, err := eventRecoveryPage(ctx, sysin.EventStatusReceived, cursor, 2, collectorRecoveryScope{TenantID: 1})
		if err != nil {
			return err
		}
		if len(second) != 1 || second[0]["id"].Int64() != 3 {
			t.Fatalf("same-time keyset failed to reach later event: %v", second)
		}
		cursor, err = enqueueEventRecoveryPage(second, sysin.EventStatusReceived, cursor, 2, func(int64, int) error { return nil })
		if err != nil {
			return err
		}
		if cursor != (eventRecoveryCursor{}) {
			t.Fatalf("tail did not wrap: %+v", cursor)
		}
		retried, err := eventRecoveryPage(ctx, sysin.EventStatusFailedRetry, cutoff, 1, collectorRecoveryScope{TenantID: 1})
		if err != nil {
			return err
		}
		if len(retried) != 1 || retried[0]["id"].Int64() != 10 {
			t.Fatalf("retry filter or indexed order wrong: %v", retried)
		}
		retryCursor := advanceEventRecoveryCursor(cutoff, sysin.EventStatusFailedRetry, retried[0])
		next, err := eventRecoveryPage(ctx, sysin.EventStatusFailedRetry, retryCursor, 5, collectorRecoveryScope{TenantID: 1})
		if err != nil {
			return err
		}
		if len(next) != 1 || next[0]["id"].Int64() != 14 {
			t.Fatalf("retry page included exhausted/future/null/processing rows: %v", next)
		}
		if _, err := eventRecoveryPage(ctx, "invalid", cutoff, 1); err == nil {
			t.Fatal("unsupported recovery state must not query")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
