package sys

import (
	"context"
	"time"

	"hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
)

const collectProcessScheduleProbeLimit = 100

// Failed events have their own bounded recovery path. Neither terminal events
// nor passive unmatched verification may keep an active source wakeup alive.
func collectProcessActiveWindowStatuses() []string {
	return []string{
		sysin.CollectEventStatusPending, sysin.CollectEventStatusGroupCollect,
		sysin.CollectEventStatusWaitingOrder, sysin.CollectEventStatusPrechecked,
		sysin.CollectEventStatusMediaPending, sysin.CollectEventStatusMediaReady,
	}
}

func collectMaterialActiveWindowModel(ctx context.Context, payload collectProcessQueuePayload) *gdb.Model {
	return collectMaterialWindowModel(ctx, payload, false).
		WhereIn("e.status", collectProcessActiveWindowStatuses()).
		Where("e.processed_at IS NULL")
}

// A candidate cache can be incomplete or stale. An empty candidate subset is
// not evidence that the source is empty. The caller supplies the same scoped,
// bounded query for both the indexed probe and the one permitted fallback.
func collectProcessScheduleProbe(candidateIDs []int64, load func([]int64) (gdb.Result, error)) (gdb.Result, error) {
	rows, err := load(candidateIDs)
	if err != nil || len(rows) > 0 || len(candidateIDs) == 0 {
		return rows, err
	}
	return load(nil)
}

func collectProcessScheduleRowsDelay(rows gdb.Result) (time.Duration, bool, error) {
	var nextDelay time.Duration
	found := false
	for _, row := range rows {
		active := false
		for _, status := range collectProcessActiveWindowStatuses() {
			if row["status"].String() == status {
				active = true
				break
			}
		}
		role := row["material_role"].String()
		if !active || row["processed_at"].GTime() != nil ||
			(role != "" && role != collectMaterialRolePending && role != collectMaterialRoleDisplay) {
			continue
		}
		from, wait := row["created_at"].GTime(), time.Duration(collectMaterialGroupingDelay)
		if row["material_group_status"].String() == collectMaterialGroupWaitingVerify {
			from, wait = row["updated_at"].GTime(), collectMaterialWaitingVerifyRetryDelay
		}
		if from == nil || from.IsZero() {
			return 0, false, gerror.New("采集调度候选缺少有效时间，保留任务等待修复")
		}
		delay := collectLocalWindowRemaining(from, wait)
		if delay <= 0 {
			return 0, true, nil
		}
		if !found || delay < nextDelay {
			nextDelay, found = delay, true
		}
	}
	return nextDelay, found, nil
}
