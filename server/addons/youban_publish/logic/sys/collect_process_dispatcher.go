package sys

import (
	"context"
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
		err := s.dispatchDueCollectProcessOutbox(cycleCtx, 200)
		cancel()
		if err != nil && ctx.Err() == nil {
			g.Log().Warningf(ctx, "投递实时采集处理任务失败：%+v", err)
		}
		s.observeCollectProcessBacklog(cycleCtx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *sSysPublish) observeCollectProcessBacklog(ctx context.Context) {
	rows, err := g.DB().Model(collectProcessOutboxTable).Ctx(ctx).
		Fields("source_id,MIN(created_at) AS oldest,COUNT(*) AS pending").
		Where("status", "pending").Group("source_id").OrderAsc("oldest").Limit(100).All()
	if err != nil {
		return
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
	sources, _ := publishObserveMeter.Int64Gauge("xiaohuiji.collect.process_pending_sources")
	sources.Record(ctx, int64(len(rows)))
}
