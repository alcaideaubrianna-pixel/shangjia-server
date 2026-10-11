package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/internal/consts"
	"hotgo/internal/library/tasklog"
)

type publishTaskLogRetentionTarget struct {
	table string
	index string
}

// Only task execution detail logs belong here. Task rows and business/audit
// records (including FeiNiu sync run/checkpoint records) are kept.
var publishTaskLogRetentionTables = []publishTaskLogRetentionTarget{
	{table: publishTgJobLogTable, index: "idx_ybp_tg_job_log_created"},
	{table: publishCollectEventLogTable, index: "idx_ybp_collect_event_log_created"},
	{table: publishCollectHistoryLogTable, index: "idx_ybp_collect_history_log_created"},
	{table: publishCycleRunLogTable, index: "idx_ybp_cycle_run_log_created"},
	{table: importRunLogTable, index: "idx_ybp_import_run_log_created"},
}

func (s *sSysPublish) runPublishRecordRetentionCleaner(ctx context.Context) {
	missingIndexWarningShown := false
	cleanup := func() {
		retentionDays := tasklog.RetentionDays()
		deleted, missingIndexes, err := s.cleanupPublishTaskLogsOlderThan(ctx, retentionDays, consts.TaskExecutionLogCleanupBatchSize)
		if err != nil {
			g.Log().Warningf(ctx, "清理过期任务日志失败：%+v", err)
		}
		if len(missingIndexes) > 0 && !missingIndexWarningShown {
			g.Log().Warningf(ctx, "任务日志清理因索引缺失或无效而跳过，请执行大表索引维护：%v", missingIndexes)
			missingIndexWarningShown = true
		} else if len(missingIndexes) == 0 {
			missingIndexWarningShown = false
		}
		if deleted > 0 {
			g.Log().Infof(ctx, "清理过期任务日志完成 deleted:%d retentionDays:%d", deleted, retentionDays)
		}
	}
	cleanup()
	ticker := time.NewTicker(consts.TaskExecutionLogCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}

func (s *sSysPublish) cleanupPublishTaskLogsOlderThan(ctx context.Context, days, batchSize int) (int, []string, error) {
	if days <= 0 {
		days = tasklog.RetentionDays()
	}
	if batchSize <= 0 {
		batchSize = consts.TaskExecutionLogCleanupBatchSize
	}
	cutoff := gtime.Now().Add(-time.Duration(days) * 24 * time.Hour)
	deletedTotal := 0
	missingIndexes := make([]string, 0)
	var firstErr error
	for _, target := range publishTaskLogRetentionTables {
		deleted, ready, err := tasklog.DeleteExpiredBatch(ctx, target.table, target.index, cutoff, batchSize)
		if err != nil {
			if firstErr == nil {
				firstErr = gerror.Wrapf(err, "清理任务日志表失败：%s", target.table)
			}
			continue
		}
		if !ready {
			missingIndexes = append(missingIndexes, target.index)
			continue
		}
		deletedTotal += deleted
	}
	return deletedTotal, missingIndexes, firstErr
}
