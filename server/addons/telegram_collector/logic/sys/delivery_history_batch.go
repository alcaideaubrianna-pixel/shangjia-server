package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/internal/library/microbatch"
)

type historyDeliverySaved struct{ ID, EventID int64 }

func (s *sCollector) historyDeliveryWriter(ctx context.Context) *microbatch.Batcher[sysin.CollectorDelivery, historyDeliverySaved] {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	if s.historyDeliveryBatch == nil {
		size, wait := historyEventBatchConfig(ctx)
		parent := s.runtimeCtx
		if parent == nil {
			parent = context.Background()
		}
		s.historyDeliveryBatch = microbatch.New(parent, size, wait, func(ctx context.Context, rows []sysin.CollectorDelivery) []microbatch.Result[historyDeliverySaved] {
			s.historyBatchSQLMu.Lock()
			defer s.historyBatchSQLMu.Unlock()
			return writeHistoryDeliveryBatch(ctx, rows)
		})
	}
	return s.historyDeliveryBatch
}

func historyDeliveryLookupKey(tenantID int64, key string) string {
	return fmt.Sprintf("%d:%s", tenantID, key)
}

func writeHistoryDeliveryBatch(ctx context.Context, input []sysin.CollectorDelivery) []microbatch.Result[historyDeliverySaved] {
	out := make([]microbatch.Result[historyDeliverySaved], len(input))
	if len(input) == 0 {
		return out
	}
	if len(input) > 50 {
		for i := range out {
			out[i].Err = gerror.New("历史交付批次不能超过50条")
		}
		return out
	}
	var values []string
	var args []any
	seen := make(map[string]bool)
	for i, row := range input {
		if row.TenantID <= 0 || row.EventID <= 0 || row.SourceID <= 0 || row.DeliveryKey == "" || !row.History {
			out[i].Err = gerror.New("历史交付批次归属无效")
			continue
		}
		key := historyDeliveryLookupKey(row.TenantID, row.DeliveryKey)
		if seen[key] {
			continue
		}
		seen[key] = true
		values = append(values, "(CAST(? AS bigint),CAST(? AS bigint),CAST(? AS text),CAST(? AS text),CAST(? AS integer),CAST(? AS bigint),CAST(? AS bigint),'history',CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai',CURRENT_TIMESTAMP AT TIME ZONE 'Asia/Shanghai')")
		args = append(args, row.TenantID, row.EventID, row.DeliveryKey, sysin.DeliveryStatusPending, sysin.EventPriorityNormal, row.SourceID, row.AccountID)
	}
	if len(values) == 0 {
		return out
	}
	c := dao.TgCollectorDelivery.Columns()
	eligible := fmt.Sprintf("existing.%s=EXCLUDED.%s AND existing.%s IN ('%s','%s') AND COALESCE(existing.%s,'')=''", c.EventId, c.EventId, c.Status, sysin.DeliveryStatusPending, sysin.DeliveryStatusFailedRetry, c.LeaseOwner)
	query := fmt.Sprintf(`INSERT INTO %s AS existing (%s,%s,%s,%s,%s,source_id,account_id,scheduler_pool,%s,%s) VALUES %s
 ON CONFLICT (%s,%s) DO UPDATE SET %s=EXCLUDED.%s,
 %s=CASE WHEN %s THEN EXCLUDED.%s ELSE existing.%s END,
 scheduler_pool=CASE WHEN %s THEN EXCLUDED.scheduler_pool ELSE existing.scheduler_pool END
 RETURNING %s,%s,%s,%s`,
		dao.TgCollectorDelivery.Table(), c.TenantId, c.EventId, c.DeliveryKey, c.Status, c.Priority, c.CreatedAt, c.UpdatedAt, strings.Join(values, ","),
		c.TenantId, c.DeliveryKey, c.DeliveryKey, c.DeliveryKey,
		c.Priority, eligible, c.Priority, c.Priority, eligible, c.Id, c.EventId, c.TenantId, c.DeliveryKey)
	started := time.Now()
	rows, err := g.DB().GetAll(ctx, query, args...)
	saved := make(map[string]historyDeliverySaved, len(rows))
	for _, row := range rows {
		saved[historyDeliveryLookupKey(row[c.TenantId].Int64(), row[c.DeliveryKey].String())] = historyDeliverySaved{row[c.Id].Int64(), row[c.EventId].Int64()}
	}
	for i, row := range input {
		if out[i].Err != nil {
			continue
		}
		if err != nil {
			out[i].Err = gerror.Wrap(err, "批量保存历史交付失败")
			continue
		}
		out[i].Value = saved[historyDeliveryLookupKey(row.TenantID, row.DeliveryKey)]
		if out[i].Value.ID <= 0 {
			out[i].Err = gerror.New("历史交付批次未返回持久化身份")
		}
	}
	if err == nil {
		g.Log().Info(ctx, "历史交付批次写入", g.Map{"event": "collector.history.delivery_batch", "requested": len(input), "affected": len(rows), "durationMs": time.Since(started).Milliseconds()})
	}
	return out
}
