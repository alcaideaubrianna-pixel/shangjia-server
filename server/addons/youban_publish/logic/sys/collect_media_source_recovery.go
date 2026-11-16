package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

// recoverCollectMediaSourceScopes shares one fixed budget across the bounded
// source batch. Each phase keeps its own query timeout and source predicates.
func (s *sSysPublish) recoverCollectMediaSourceScopes(ctx context.Context, scopes []publishAccountScope) {
	if len(scopes) == 0 {
		return
	}
	ctx, cancelPass := context.WithTimeout(ctx, 30*time.Second)
	defer cancelPass()
	budget := max(1, min(20, 100/len(scopes)))
	steps := []struct {
		name string
		run  func(context.Context, int, ...publishAccountScope) error
	}{
		{"stale_download", s.recoverStaleCollectMediaRows},
		{"pending_media", s.recoverPendingCollectMedia},
		{"bot_media", s.recoverBotCollectMedia},
		{"ready_media", s.recoverReadyMediaPendingEvents},
		{"missing_dispatch", s.recoverMediaReadyWithoutDispatch},
	}
	for _, scope := range scopes {
		if ctx.Err() != nil {
			return
		}
		laneCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		for _, step := range steps {
			if laneCtx.Err() != nil {
				break
			}
			if err := step.run(laneCtx, budget, scope); err != nil {
				g.Log().Warning(ctx, "来源级采集媒体恢复失败，保留下一轮重试", g.Map{
					"event": "collect.recovery.media_retry", "phase": step.name,
					"tenantId": scope.TenantID, "accountId": scope.AccountID,
					"sourceId": scope.SourceID, "error": err.Error(),
				})
			}
		}
		cancel()
	}
}
