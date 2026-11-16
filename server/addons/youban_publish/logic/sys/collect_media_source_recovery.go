package sys

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

type collectMediaSourceRecoveryStep struct {
	name string
	run  func(context.Context, int, ...publishAccountScope) error
}

// Only scheduling position is shared; no source data or task ownership is stored.
var collectMediaSourceRecoveryTurn atomic.Uint64

// recoverCollectMediaSourceScopes shares one fixed budget across the bounded
// source batch. Each phase keeps its own query timeout and source predicates.
func (s *sSysPublish) recoverCollectMediaSourceScopes(ctx context.Context, scopes []publishAccountScope) {
	if len(scopes) == 0 {
		return
	}
	ctx, cancelPass := context.WithTimeout(ctx, 30*time.Second)
	defer cancelPass()
	budget := max(1, min(20, 100/len(scopes)))
	steps := []collectMediaSourceRecoveryStep{
		{"stale_download", s.recoverStaleCollectMediaRows},
		{"pending_media", s.recoverPendingCollectMedia},
		{"bot_media", s.recoverBotCollectMedia},
		{"ready_media", s.recoverReadyMediaPendingEvents},
		{"missing_dispatch", s.recoverMediaReadyWithoutDispatch},
	}
	turn := collectMediaSourceRecoveryTurn.Add(1) - 1
	runCollectMediaSourceRecovery(ctx, scopes, steps, budget, turn, 10*time.Second,
		func(scope publishAccountScope, phase string, err error) {
			g.Log().Warning(ctx, "来源级采集媒体恢复失败，保留下一轮重试", g.Map{
				"event": "collect.recovery.media_retry", "phase": phase,
				"tenantId": scope.TenantID, "accountId": scope.AccountID,
				"sourceId": scope.SourceID, "error": err.Error(),
			})
		})
}

// Rotate both starting positions on every pass, including passes exhausted by
// one slow phase. A fixed phase order could otherwise never reach dispatch.
// Lane deadlines inherit the whole-pass deadline; errors do not stop peers.
func runCollectMediaSourceRecovery(ctx context.Context, scopes []publishAccountScope, steps []collectMediaSourceRecoveryStep, budget int, turn uint64, laneTimeout time.Duration, report func(publishAccountScope, string, error)) {
	if len(scopes) == 0 || len(steps) == 0 || ctx.Err() != nil {
		return
	}
	// Advance lanes after a phase rotation, avoiding correlated modulo cycles
	// that would pin each source to the same first phase.
	laneStart := int((turn / uint64(len(steps))) % uint64(len(scopes)))
	phaseStart := int(turn % uint64(len(steps)))
	for lane := 0; lane < len(scopes); lane++ {
		if ctx.Err() != nil {
			return
		}
		scope := scopes[(laneStart+lane)%len(scopes)]
		laneCtx, cancel := context.WithTimeout(ctx, laneTimeout)
		for phase := 0; phase < len(steps); phase++ {
			if laneCtx.Err() != nil {
				break
			}
			step := steps[(phaseStart+phase)%len(steps)]
			if err := step.run(laneCtx, budget, scope); err != nil && report != nil {
				report(scope, step.name, err)
			}
		}
		cancel()
	}
}
