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
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
