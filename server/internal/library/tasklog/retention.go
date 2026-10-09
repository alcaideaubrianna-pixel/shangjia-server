package tasklog

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/internal/consts"
)

const maxTaskLogRetentionDays = 3650

// RetentionDays returns the configured task-log retention window. Invalid,
// non-positive, or unreasonably large values fall back to the safe default.
func RetentionDays() int {
	value := strings.TrimSpace(os.Getenv(consts.TaskLogRetentionDaysEnv))
	if value == "" {
		return consts.DefaultTaskExecutionLogRetentionDays
	}
	days, err := strconv.Atoi(value)
	if err != nil || days <= 0 || days > maxTaskLogRetentionDays {
		return consts.DefaultTaskExecutionLogRetentionDays
	}
	return days
}

// DeleteExpiredBatch deletes a bounded batch of immutable task-log rows whose
// creation time is older than cutoff. It only runs when the supporting index
// is valid and ready, so deploying the worker before explicit index maintenance
// cannot accidentally trigger a full-table scan. Callers must pass static identifiers.
func DeleteExpiredBatch(ctx context.Context, table, index string, cutoff *gtime.Time, batchSize int) (deleted int, ready bool, err error) {
	if batchSize <= 0 {
		return 0, false, gerror.New("任务日志清理批次大小必须大于 0")
	}
	ready, err = hasIndex(ctx, table, index)
	if err != nil || !ready {
		return 0, ready, err
	}
	rows, err := g.DB().Model(table).Safe().Ctx(ctx).
		Fields("id").
		WhereLT("created_at", cutoff).
		OrderAsc("created_at").
		OrderAsc("id").
		Limit(batchSize).
		All()
	if err != nil {
		return 0, true, gerror.Wrap(err, "读取过期任务日志失败")
	}
	if len(rows) == 0 {
		return 0, true, nil
	}

	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := row["id"].Int64(); id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return 0, true, nil
	}
	result, err := g.DB().Model(table).Safe().Ctx(ctx).
		WhereIn("id", ids).
		WhereLT("created_at", cutoff).
		Delete()
	if err != nil {
		return 0, true, gerror.Wrap(err, "删除过期任务日志失败")
	}
	affected, rowsErr := result.RowsAffected()
	if rowsErr != nil {
		return 0, true, gerror.Wrap(rowsErr, "读取过期任务日志删除数量失败")
	}
	return int(affected), true, nil
}

func hasIndex(ctx context.Context, table, index string) (bool, error) {
	var (
		count int
		err   error
	)
	switch strings.ToLower(g.DB().GetConfig().Type) {
	case consts.DBPgsql, "postgres":
		count, err = g.DB().GetCount(ctx, `
			SELECT COUNT(*)
			FROM pg_index AS i
			JOIN pg_class AS tbl ON tbl.oid = i.indrelid
			JOIN pg_class AS idx ON idx.oid = i.indexrelid
			JOIN pg_namespace AS ns ON ns.oid = tbl.relnamespace
			WHERE ns.nspname = current_schema()
			  AND tbl.relname = ?
			  AND idx.relname = ?
			  AND i.indisvalid
			  AND i.indisready
		`, table, index)
	case consts.DBMysql, "":
		count, err = g.DB().GetCount(ctx, `
			SELECT COUNT(*) FROM information_schema.statistics
			WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?
		`, table, index)
	default:
		return false, gerror.Newf("任务日志保留不支持当前数据库类型：%s", g.DB().GetConfig().Type)
	}
	if err != nil {
		return false, gerror.Wrap(err, "检查任务日志保留索引失败")
	}
	return count > 0, nil
}
