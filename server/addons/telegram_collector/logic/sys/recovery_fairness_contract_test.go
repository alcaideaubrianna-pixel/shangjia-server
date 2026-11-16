package sys

import (
	"context"
	"errors"
	"testing"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/hibiken/asynq"
)

func TestRecoverySpoolRejectsMissingScopeBeforeSQL(t *testing.T) {
	if _, err := claimIngestSpool(context.Background()); err == nil {
		t.Fatal("unscoped spool claim accepted")
	}
	if err := (&sCollector{}).recoverIngestSpool(context.Background(), 10); err == nil {
		t.Fatal("unscoped spool recovery accepted")
	}
}

func TestDeliveryRecoveryRedisFailureAndCancellationStopPage(t *testing.T) {
	rows := gdb.Result{
		gdb.Record{"id": gvar.New(1), "priority": gvar.New(10), deliveryRecoveryCreatedField: gvar.New("2026-11-17 01:02:03.000001")},
		gdb.Record{"id": gvar.New(2), "priority": gvar.New(10), deliveryRecoveryCreatedField: gvar.New("2026-11-17 01:02:03.000002")},
	}
	for _, cause := range []error{errCollectorRecoveryRedisUnavailable, context.Canceled, context.DeadlineExceeded} {
		cursor, err := walkDeliveryRecoveryPage(rows, deliveryRecoveryCursor{UntilTime: "2026-11-17 02:00:00.000001"}, 2, func(row gdb.Record) error {
			if row["id"].Int64() == 2 {
				return cause
			}
			return nil
		})
		if !errors.Is(err, cause) || cursor.AfterID != 1 || cursor.AfterCreatedAt != "2026-11-17 01:02:03.000001" {
			t.Fatalf("interrupted handoff advanced cursor: %+v %v", cursor, err)
		}
	}
}

func TestRecoveryBranchRotationDoesNotMutateInput(t *testing.T) {
	branches := []string{"realtime", "history"}
	first := collectorRecoveryBranchOrder(branches)
	second := collectorRecoveryBranchOrder(branches)
	if first[0] == second[0] || branches[0] != "realtime" {
		t.Fatalf("rotation missing or mutated input: %v %v %v", first, second, branches)
	}
}

func TestDeliveryRecoveryClassifiesOnlyKnownRedisErrors(t *testing.T) {
	for _, cause := range []error{errors.New("source route missing"), errors.New("database connection refused"), context.DeadlineExceeded, asynq.ErrDuplicateTask, asynq.ErrTaskIDConflict} {
		got := classifyDeliveryRecoveryEnqueueError(cause)
		if !errors.Is(got, cause) || errors.Is(got, errCollectorRecoveryRedisUnavailable) {
			t.Fatalf("lane error incorrectly stops all scopes: %v", got)
		}
	}
	cause := errors.New("redis: connection refused")
	if !errors.Is(classifyDeliveryRecoveryEnqueueError(cause), errCollectorRecoveryRedisUnavailable) {
		t.Fatal("identified Redis failure not propagated")
	}
}

func TestDeliveryRecoveryOrdinaryErrorsAdvanceScanWithoutAcknowledgingDelivery(t *testing.T) {
	rows := gdb.Result{
		gdb.Record{"id": gvar.New(2), "priority": gvar.New(10), "status": gvar.New("pending"), deliveryRecoveryCreatedField: gvar.New("2026-11-17 01:02:03.000002")},
		gdb.Record{"id": gvar.New(3), "priority": gvar.New(10), "status": gvar.New("pending"), deliveryRecoveryCreatedField: gvar.New("2026-11-17 01:02:03.000003")},
	}
	for _, cause := range []error{errors.New("database overloaded"), errors.New("source route missing"), asynq.ErrDuplicateTask, asynq.ErrTaskIDConflict} {
		cursor := deliveryRecoveryCursor{UntilTime: "2026-11-17 02:00:00.000001", AfterID: 1}
		visited := 0
		next, err := walkDeliveryRecoveryPage(rows, cursor, 2, func(row gdb.Record) error {
			visited++
			if row["id"].Int64() == 2 {
				return cause
			}
			return nil
		})
		if !errors.Is(err, cause) || next.AfterID != 3 || visited != 2 {
			t.Fatalf("failed row starved tail or error hidden: %+v %v", next, err)
		}
		if rows[0]["status"].String() != "pending" {
			t.Fatal("scan position changed business status")
		}
		reset, err := walkDeliveryRecoveryPage(nil, next, 2, func(gdb.Record) error { t.Fatal("unexpected submit"); return nil })
		if err != nil || reset.AfterID != 0 || reset.UntilTime != "" {
			t.Fatal("completed window did not reset for retry")
		}
	}
}
