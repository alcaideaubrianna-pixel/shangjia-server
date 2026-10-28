package sys

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/errors/gerror"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// Successful collection counts distinct dispatched events, not retained ingress
// events or target channels. Read durable dispatch evidence for the current page
// so legacy zero counters and event-log cleanup cannot hide successful imports.
func fillCollectSourceSuccessTotals(ctx context.Context, list []*sysin.CollectSourceModel, tenantID, accountID int64) error {
	ids := make([]int64, 0, len(list))
	for _, item := range list {
		if item != nil && item.TenantId == tenantID && item.AccountId == accountID {
			ids = append(ids, item.Id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	cols := pdao.YoubanPublishCollectDispatch.Columns()
	var rows []struct {
		SourceID     int64 `orm:"source_id"`
		SuccessTotal int64 `orm:"success_total"`
	}
	if err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields(fmt.Sprintf("%s,COUNT(DISTINCT %s) AS success_total", cols.SourceId, cols.EventId)).
		Where(cols.TenantId, tenantID).Where(cols.AccountId, accountID).
		WhereIn(cols.SourceId, ids).Where(cols.Status, sysin.CollectDispatchStatusSent).
		Group(cols.SourceId).Scan(&rows); err != nil {
		return gerror.Wrap(err, "统计采集源成功采集数量失败")
	}
	totals := make(map[int64]int64, len(rows))
	for _, row := range rows {
		totals[row.SourceID] = row.SuccessTotal
	}
	for _, item := range list {
		if item != nil && item.TenantId == tenantID && item.AccountId == accountID {
			item.SuccessTotal = totals[item.Id]
		}
	}
	return nil
}
