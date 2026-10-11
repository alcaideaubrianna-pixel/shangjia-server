package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/internal/consts"
	"hotgo/internal/library/tasklog"
)

const (
	pushLogRetentionTable         = "hg_addon_lazysheep_tggo_push_log"
	pushLogRetentionIndex         = "idx_lazysheep_tggo_push_log_created"
	webhookLogRetentionTable      = "hg_addon_lazysheep_tggo_webhook_log"
	webhookLogRetentionIndex      = "idx_lazysheep_tggo_webhook_log_created"
	webhookLogRetentionMysqlIndex = "created_at"
)

type tggoTaskLogRetentionTarget struct {
	table string
	index string
}

func (s *sLazySheepTGGo) runPushLogRetentionCleaner(ctx context.Context) {
	missingIndexWarningShown := false
	cleanup := func() {
		retentionDays := tasklog.RetentionDays()
		cutoff := gtime.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
		deletedTotal := 0
		missingIndexes := make([]string, 0, 2)
		for _, target := range tggoTaskLogRetentionTargets(g.DB().GetConfig().Type) {
			var ensureErr error
			switch target.table {
			case pushLogRetentionTable:
				ensureErr = s.ensurePushLogTable(ctx)
			case webhookLogRetentionTable:
				ensureErr = s.ensureWebhookLogTable(ctx)
			}
			if ensureErr != nil {
				g.Log().Warningf(ctx, "初始化任务日志保留目标失败 table:%s err:%+v", target.table, ensureErr)
				continue
			}
			deleted, ready, err := tasklog.DeleteExpiredBatch(ctx, target.table, target.index, cutoff, consts.TaskExecutionLogCleanupBatchSize)
			if err != nil {
				g.Log().Warningf(ctx, "清理任务日志失败 table:%s err:%+v", target.table, err)
				continue
			}
			if !ready {
				missingIndexes = append(missingIndexes, target.index)
				continue
			}
			deletedTotal += deleted
		}
		if len(missingIndexes) > 0 && !missingIndexWarningShown {
			g.Log().Warningf(ctx, "TGGo任务日志清理因索引缺失或无效而跳过，请执行大表索引维护 indexes:%v", missingIndexes)
			missingIndexWarningShown = true
		} else if len(missingIndexes) == 0 {
			missingIndexWarningShown = false
		}
		if deletedTotal > 0 {
			g.Log().Infof(ctx, "清理TGGo过期任务日志完成 deleted:%d retentionDays:%d", deletedTotal, retentionDays)
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

func tggoTaskLogRetentionTargets(databaseType string) []tggoTaskLogRetentionTarget {
	webhookIndex := webhookLogRetentionIndex
	if databaseType = strings.ToLower(strings.TrimSpace(databaseType)); databaseType == consts.DBMysql || databaseType == "" {
		webhookIndex = webhookLogRetentionMysqlIndex
	}
	return []tggoTaskLogRetentionTarget{
		{table: pushLogRetentionTable, index: pushLogRetentionIndex},
		{table: webhookLogRetentionTable, index: webhookIndex},
	}
}
