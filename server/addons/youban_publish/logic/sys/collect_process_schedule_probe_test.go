package sys

import (
	"errors"
	"testing"

	"hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/os/gtime"
)

func collectScheduleFixture(id int64, status, role string) gdb.Record {
	return gdb.Record{
		"id": gvar.New(id), "status": gvar.New(status),
		"material_role": gvar.New(role),
		"created_at":    gvar.New(gtime.Now().Add(-2 * collectMaterialGroupingDelay)),
		"updated_at":    gvar.New(gtime.Now().Add(-2 * collectMaterialWaitingVerifyRetryDelay)),
	}
}

func TestCollectScheduleProbeStaleCandidatesCannotHideActiveSource(t *testing.T) {
	calls := 0
	active := collectScheduleFixture(200, sysin.CollectEventStatusGroupCollect, collectMaterialRoleDisplay)
	rows, err := collectProcessScheduleProbe([]int64{100}, func(ids []int64) (gdb.Result, error) {
		calls++
		if calls == 1 {
			if len(ids) != 1 || ids[0] != 100 {
				t.Fatalf("first probe lost candidate scope: %v", ids)
			}
			// Candidate 100 is an old ignored row, filtered by the DB predicate.
			return nil, nil
		}
		if len(ids) != 0 {
			t.Fatal("fallback is still restricted to the stale candidate subset")
		}
		return gdb.Result{active}, nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("bounded fallback failed: calls=%d err=%v", calls, err)
	}
	delay, pending, err := collectProcessScheduleRowsDelay(rows)
	if err != nil || !pending || delay != 0 {
		t.Fatalf("active source wakeup was lost: delay=%s pending=%t err=%v", delay, pending, err)
	}
}

func TestCollectScheduleProbeDoesNotFallbackOnErrorOrNonemptyPage(t *testing.T) {
	sentinel := errors.New("database unavailable")
	for _, withError := range []bool{true, false} {
		calls := 0
		_, err := collectProcessScheduleProbe([]int64{100}, func([]int64) (gdb.Result, error) {
			calls++
			if withError {
				return nil, sentinel
			}
			return gdb.Result{collectScheduleFixture(100, sysin.CollectEventStatusPending, collectMaterialRolePending)}, nil
		})
		if calls != 1 || (withError && !errors.Is(err, sentinel)) || (!withError && err != nil) {
			t.Fatalf("unexpected probe behavior: error=%t calls=%d err=%v", withError, calls, err)
		}
	}
	calls := 0
	_, err := collectProcessScheduleProbe(nil, func([]int64) (gdb.Result, error) {
		calls++
		return nil, nil
	})
	if err != nil || calls != 1 {
		t.Fatal("cold source probe executed more than one fallback page")
	}
}

func TestCollectScheduleTerminalRowsCannotKeepSourceAlive(t *testing.T) {
	rows := gdb.Result{
		collectScheduleFixture(1, sysin.CollectEventStatusDispatched, collectMaterialRoleDisplay),
		collectScheduleFixture(2, sysin.CollectEventStatusProcessed, collectMaterialRoleDisplay),
		collectScheduleFixture(3, sysin.CollectEventStatusIgnored, collectMaterialRoleVerify),
		collectScheduleFixture(4, sysin.CollectEventStatusFailed, collectMaterialRoleDisplay),
	}
	if _, pending, err := collectProcessScheduleRowsDelay(rows); err != nil || pending {
		t.Fatalf("terminal source was kept alive: pending=%t err=%v", pending, err)
	}
}

func TestCollectScheduleWaitingDisplayIsNotDropped(t *testing.T) {
	row := collectScheduleFixture(10, sysin.CollectEventStatusGroupCollect, collectMaterialRoleDisplay)
	row["material_group_status"] = gvar.New(collectMaterialGroupWaitingVerify)
	row["updated_at"] = gvar.New(gtime.Now())
	delay, pending, err := collectProcessScheduleRowsDelay(gdb.Result{row})
	if err != nil || !pending || delay <= 0 {
		t.Fatalf("waiting display lost wakeup: delay=%s pending=%t err=%v", delay, pending, err)
	}
	row["updated_at"] = gvar.New(nil)
	if _, _, err = collectProcessScheduleRowsDelay(gdb.Result{row}); err == nil {
		t.Fatal("invalid active timestamps were silently acknowledged")
	}
}
