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

func TestDeliveryRecoveryBoundedCandidateEligibility(t *testing.T) {
	cursor := deliveryRecoveryCursor{UntilTime: "2026-11-18 05:54:00.123456", StaleBefore: "2026-11-18 05:53:45.123456"}
	for _, tc := range []struct {
		name, status, created, updated, next string
		attempts                             int
		want                                 bool
	}{
		{"pending boundary", "pending", cursor.UntilTime, cursor.StaleBefore, "", 0, true},
		{"fresh pending", "pending", cursor.UntilTime, "2026-11-18 05:53:45.123457", "", 0, false},
		{"pending null update", "pending", cursor.UntilTime, "", "", 0, false},
		{"retry boundary", "failed_retry", cursor.UntilTime, "", cursor.UntilTime, 4, true},
		{"future retry", "failed_retry", cursor.UntilTime, "", "2026-11-18 05:54:00.123457", 0, false},
		{"retry null due", "failed_retry", cursor.UntilTime, "", "", 0, false},
		{"attempt ceiling", "pending", cursor.UntilTime, cursor.StaleBefore, "", 5, false},
		{"future creation", "pending", "2026-11-18 05:54:00.123457", cursor.StaleBefore, "", 0, false},
		{"completed", "completed", cursor.UntilTime, cursor.StaleBefore, "", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := gdb.Record{
				"status": gvar.New(tc.status), "attempt_count": gvar.New(tc.attempts),
				deliveryRecoveryCreatedField: gvar.New(tc.created),
				"recovery_updated_at":        gvar.New(tc.updated), "recovery_next_run_at": gvar.New(tc.next),
			}
			if got := deliveryRecoveryCandidateEligible(row, cursor); got != tc.want {
				t.Fatalf("eligible=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestDeliveryRecoveryBoundedCandidatesAdvanceBeforeCancellation(t *testing.T) {
	cursor := deliveryRecoveryCursor{UntilTime: "2026-11-18 05:54:00.000000", StaleBefore: "2026-11-18 05:53:45.000000"}
	rows := gdb.Result{}
	for id := int64(1); id <= 3; id++ {
		rows = append(rows, gdb.Record{
			"id": gvar.New(id), "priority": gvar.New(100), "status": gvar.New("failed_retry"), "attempt_count": gvar.New(0),
			deliveryRecoveryCreatedField: gvar.New("2026-11-18 05:00:00.000001"),
			"recovery_next_run_at":       gvar.New("2026-11-19 00:00:00.000000"),
		})
	}
	rows[2]["recovery_next_run_at"] = gvar.New(cursor.UntilTime)
	next, err := walkDeliveryRecoveryPage(rows, cursor, 3, func(row gdb.Record) error {
		if !deliveryRecoveryCandidateEligible(row, cursor) {
			return nil
		}
		return context.DeadlineExceeded
	})
	if !errors.Is(err, context.DeadlineExceeded) || next.AfterID != 2 || next.UntilTime != cursor.UntilTime {
		t.Fatalf("ineligible candidates must advance, interrupted candidate must remain: cursor=%+v err=%v", next, err)
	}
}

// Opt-in disposable loopback PostgreSQL only; never uses a production connection.
func TestDeliveryRecoveryBoundedCandidatesLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	before := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", before)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	ctx := context.Background()
	defer func() { _ = g.DB().GetCore().ClearTableFieldsAll(ctx) }()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, statement := range []string{
			`CREATE TEMP TABLE hg_tg_collector_delivery(
 id bigint PRIMARY KEY,tenant_id bigint DEFAULT 25,account_id bigint DEFAULT 0,source_id bigint DEFAULT 78,
 scheduler_pool text DEFAULT 'realtime',status text DEFAULT 'failed_retry',priority int DEFAULT 100,
 created_at timestamp DEFAULT '2026-11-18 05:00:00.123456',updated_at timestamp DEFAULT '2026-11-18 05:00:00',
 next_run_at timestamp DEFAULT '2026-11-19',attempt_count int DEFAULT 0) ON COMMIT DROP`,
			`CREATE INDEX ON hg_tg_collector_delivery(tenant_id,account_id,source_id,scheduler_pool,priority DESC,created_at,id) WHERE status IN ('pending','failed_retry')`,
			`INSERT INTO hg_tg_collector_delivery(id) SELECT generate_series(1,24)`,
			`INSERT INTO hg_tg_collector_delivery(id,priority,status,next_run_at) VALUES(25,17,'failed_retry','2026-11-18 05:54:00.123456'),(26,1,'pending',NULL)`,
			`INSERT INTO hg_tg_collector_delivery(id,source_id) VALUES(27,79)`,
			`INSERT INTO hg_tg_collector_delivery(id,scheduler_pool) VALUES(28,'history')`,
			`INSERT INTO hg_tg_collector_delivery(id,account_id) VALUES(29,1)`,
			`INSERT INTO hg_tg_collector_delivery(id,tenant_id) VALUES(30,26)`,
		} {
			if _, err := tx.Exec(statement); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		cursor := deliveryRecoveryCursor{UntilTime: "2026-11-18 05:54:00.123456", StaleBefore: "2026-11-18 05:53:45.123456"}
		scope := collectorRecoveryScope{TenantID: 25, AccountID: 0, SourceID: 78}
		var scanned, submitted []int64
		for page := 0; page < 4; page++ {
			// Append faster than one candidate page per invocation. These rows
			// must not extend the old snapshot even at its lowest priority.
			if _, err := tx.Exec(`INSERT INTO hg_tg_collector_delivery
				(id,priority,status,created_at,updated_at,next_run_at)
				SELECT n,1,'pending','2026-11-18 06:00:00','2026-11-18 06:00:00',NULL
				FROM generate_series(?::integer,?::integer) n`, 100+page*9, 108+page*9); err != nil {
				return err
			}
			rows, err := deliveryRecoveryBoundedCandidatePage(ctx, scope, "realtime", cursor, 100)
			if err != nil {
				return err
			}
			if len(rows) > deliveryRecoveryPageSize {
				t.Fatalf("unbounded candidate page: %d", len(rows))
			}
			if page < 3 && len(rows) != 8 {
				t.Fatalf("due filtering happened before LIMIT: page=%d rows=%d", page, len(rows))
			}
			for _, row := range rows {
				scanned = append(scanned, row["id"].Int64())
			}
			cursor, err = walkDeliveryRecoveryPage(rows, cursor, 8, func(row gdb.Record) error {
				if deliveryRecoveryCandidateEligible(row, cursor) {
					submitted = append(submitted, row["id"].Int64())
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		wantScanned := make([]int64, 26)
		for i := range wantScanned {
			wantScanned[i] = int64(i + 1)
		}
		if !reflect.DeepEqual(scanned, wantScanned) || !reflect.DeepEqual(submitted, []int64{25, 26}) || cursor != (deliveryRecoveryCursor{}) {
			t.Fatalf("scan=%v submit=%v cursor=%+v", scanned, submitted, cursor)
		}

		// A cursor saved by the earlier unfenced query may already be beyond
		// its own upper bound. It must drain/reset, not require manual repair.
		old := deliveryRecoveryCursor{UntilTime: "2026-11-18 05:54:00.123456", StaleBefore: "2026-11-18 05:53:45.123456",
			AfterPriority: 1, AfterID: 100, AfterCreatedAt: "2026-11-18 06:00:00.000000"}
		rows, err := deliveryRecoveryBoundedCandidatePage(ctx, scope, "realtime", old, 8)
		if err != nil {
			return err
		}
		if len(rows) != 0 {
			t.Fatalf("old cursor admitted future-of-fence rows: %v", rows)
		}
		reset, err := walkDeliveryRecoveryPage(rows, old, 8, func(gdb.Record) error {
			t.Fatal("old out-of-fence cursor must not submit a candidate")
			return nil
		})
		if err != nil || reset != (deliveryRecoveryCursor{}) {
			t.Fatalf("old cursor did not reset: %+v err=%v", reset, err)
		}

		// The next sweep gets a new fence and admits every newly due row.
		cursor = deliveryRecoveryCursor{UntilTime: "2026-11-18 06:01:00.000000", StaleBefore: "2026-11-18 06:00:45.000000"}
		var admitted []int64
		for page := 0; page < 10; page++ {
			rows, err := deliveryRecoveryBoundedCandidatePage(ctx, scope, "realtime", cursor, 100)
			if err != nil {
				return err
			}
			if len(rows) > deliveryRecoveryPageSize {
				t.Fatalf("new fence candidate page is unbounded: %d", len(rows))
			}
			cursor, err = walkDeliveryRecoveryPage(rows, cursor, 8, func(row gdb.Record) error {
				if row["id"].Int64() >= 100 && deliveryRecoveryCandidateEligible(row, cursor) {
					admitted = append(admitted, row["id"].Int64())
				}
				return nil
			})
			if err != nil {
				return err
			}
			if cursor == (deliveryRecoveryCursor{}) {
				break
			}
		}
		wantAdmitted := make([]int64, 36)
		for i := range wantAdmitted {
			wantAdmitted[i] = int64(100 + i)
		}
		if !reflect.DeepEqual(admitted, wantAdmitted) || cursor != (deliveryRecoveryCursor{}) {
			t.Fatalf("new fence lost work or did not finish: admitted=%v cursor=%+v", admitted, cursor)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
