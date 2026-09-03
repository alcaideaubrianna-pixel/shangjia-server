package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
)

var collectEventCleanupStatuses = []string{"failed", "ignored"}

func (s *sSysPublish) cleanupCollectEventsOlderThan(ctx context.Context, days, limit int) error {
	if days <= 0 {
		days = collectEventRetentionDays
	}
	if limit <= 0 {
		limit = 1000
	}
	cutoff := gtime.Now().Add(-time.Duration(days) * 24 * time.Hour)
	eventCols := pdao.YoubanPublishCollectEvent.Columns()
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields(eventCols.Id).
		WhereIn(eventCols.Status, collectEventCleanupStatuses).
		WhereLTE(eventCols.CreatedAt, cutoff).
		OrderAsc(eventCols.Id).
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取过期采集事件失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := row[eventCols.Id].Int64(); id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err = tx.Model(pdao.YoubanPublishCollectEventMedia.Table()).
			Ctx(ctx).WhereIn("event_id", ids).Delete(); err != nil {
			return gerror.Wrap(err, "清理过期采集事件媒体失败")
		}
		if _, err = tx.Model(pdao.YoubanPublishCollectEventLog.Table()).
			Ctx(ctx).WhereIn("event_id", ids).Delete(); err != nil {
			return gerror.Wrap(err, "清理过期采集事件日志失败")
		}
		if _, err = tx.Model(pdao.YoubanPublishCollectEvent.Table()).
			Ctx(ctx).
			WhereIn(eventCols.Id, ids).
			WhereIn(eventCols.Status, collectEventCleanupStatuses).
			WhereLTE(eventCols.CreatedAt, cutoff).
			Delete(); err != nil {
			return gerror.Wrap(err, "清理过期采集事件失败")
		}
		return nil
	})
}
