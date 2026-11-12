package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"time"
)

// The foreground handoff must not wait behind the broad minute-long recovery
// sweep. Database claims arbitrate concurrent dispatchers across replicas.
func (s *sSysPublish) runCollectProcessDispatcher(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		cycleCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		// 账户级轮询，避免一个历史积压账户占满全局 Outbox 批次。
		var scopes []publishAccountScope
		_ = g.DB().Model(collectProcessOutboxTable).Ctx(cycleCtx).
			Fields("tenant_id,account_id").Where("status", "pending").Where("account_id > 0").
			Group("tenant_id,account_id").OrderAsc("tenant_id").OrderAsc("account_id").Limit(16).Scan(&scopes)
		err := error(nil)
		// 没有账户分片时不再回退到全局扫描；无归属历史任务由独立恢复器处理。
		for _, scope := range scopes {
			if scopeErr := s.dispatchDueCollectProcessOutbox(cycleCtx, 200, scope); scopeErr != nil && err == nil {
				err = scopeErr
			}
		}
		s.observeCollectProcessBacklog(cycleCtx, scopes)
		cancel()
		if err != nil && ctx.Err() == nil {
			g.Log().Warningf(ctx, "投递实时采集处理任务失败：%+v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *sSysPublish) observeCollectProcessBacklog(ctx context.Context, scopes []publishAccountScope) {
	// 统计必须与投递使用同样的账户分片，禁止每两秒扫描全局 Outbox。
	rows := make([]gdb.Record, 0)
	for _, scope := range scopes {
		part, err := g.DB().Model(collectProcessOutboxTable).Ctx(ctx).
			Fields("COALESCE(history,FALSE) AS history,MIN(created_at) AS oldest,COUNT(*) AS pending").
			Where("status", "pending").Where("tenant_id", scope.TenantID).Where("account_id", scope.AccountID).
			Group("COALESCE(history,FALSE)").OrderAsc("oldest").All()
		if err != nil {
			continue
		}
		rows = append(rows, part...)
	}
	oldest := int64(0)
	for _, row := range rows {
		if value := row["oldest"].GTime(); value != nil && !value.IsZero() {
			age := time.Since(value.Time).Milliseconds()
			if age > oldest {
				oldest = age
			}
		}
	}
	gauge, _ := publishObserveMeter.Int64Gauge("xiaohuiji.collect.process_oldest_pending_age_ms")
	gauge.Record(ctx, oldest)
	var realtimePending, historyPending int64
	var realtimeOldest, historyOldest int64
	for _, row := range rows {
		age := int64(0)
		if value := row["oldest"].GTime(); value != nil && !value.IsZero() {
			age = max(0, time.Since(value.Time).Milliseconds())
		}
		if row["history"].Bool() {
			historyPending += row["pending"].Int64()
			if age > historyOldest {
				historyOldest = age
			}
		} else {
			realtimePending += row["pending"].Int64()
			if age > realtimeOldest {
				realtimeOldest = age
			}
		}
	}
	pending, _ := publishObserveMeter.Int64Gauge("xiaohuiji.collect.process_pending")
	pending.Record(ctx, realtimePending+historyPending)
	realtime, _ := publishObserveMeter.Int64Gauge("xiaohuiji.collect.realtime_process_pending")
	realtime.Record(ctx, realtimePending)
	history, _ := publishObserveMeter.Int64Gauge("xiaohuiji.collect.history_process_pending")
	history.Record(ctx, historyPending)
	realtimeAge, _ := publishObserveMeter.Int64Gauge("xiaohuiji.collect.realtime_process_oldest_age_ms")
	realtimeAge.Record(ctx, realtimeOldest)
	historyAge, _ := publishObserveMeter.Int64Gauge("xiaohuiji.collect.history_process_oldest_age_ms")
	historyAge.Record(ctx, historyOldest)
}
