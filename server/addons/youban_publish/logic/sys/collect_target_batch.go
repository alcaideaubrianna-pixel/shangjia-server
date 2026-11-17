package sys

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/util/guid"
	"hotgo/internal/library/cache"
)

const collectTargetBatchTTL = 15 * time.Minute

func collectTargetBatchKey(p collectProcessQueuePayload) string {
	return fmt.Sprintf("youban_publish:collect:target_batch:%d:%d:%d:%s", p.TenantId, p.AccountId, p.SourceId, canonicalCollectSourceChatID(p.SourceChatId))
}

// Producers only replace dirty; consumers never delete it. A late producer
// cannot have its wakeup erased by an earlier generation's completion.
func dirtyCollectTargetBatch(ctx context.Context, p collectProcessQueuePayload) error {
	if !cache.Initialized() {
		return gerror.New("采集批次缓存未就绪，保留任务重试")
	}
	// Only ingress calls this. A same-event edit must also invalidate generation.
	// Deferred retries never call it; no per-event identity keys are required.
	return cache.Instance().Set(ctx, collectTargetBatchKey(p)+":dirty", guid.S(), collectTargetBatchTTL)
}

type collectTargetBatchVisit struct{ ids []int64 }
type collectTargetBatchVisitKey struct{}

func visitCollectTargetBatch(ctx context.Context, id int64) {
	if visits, ok := ctx.Value(collectTargetBatchVisitKey{}).(*collectTargetBatchVisit); ok && id > 0 {
		visits.ids = append(visits.ids, id)
	}
}

// Run one event at a time against the shared rows/media snapshot. Retry errors
// yield only that event; real errors fail the batch and never complete it.
func runCollectTargetBatchEvents(ctx context.Context, count int, run func(int) error) error {
	var earliest *collectProcessRetryError
	for index := 0; index < count; index++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := run(index)
		if err == nil {
			continue
		}
		var retry *collectProcessRetryError
		if !errors.As(err, &retry) {
			return err
		}
		if earliest == nil || retry.delay < earliest.delay {
			earliest = retry
		}
	}
	if earliest != nil {
		return earliest
	}
	return nil
}

type collectTargetBatchCompletion struct {
	Generation      string
	CoveredEventIDs []int64
	RetryAt         time.Time
	FinishedAt      time.Time
}

type collectTargetBatchOps struct {
	targetID     int64
	mergeTTL     time.Duration
	coverage     func() []int64
	generation   func(context.Context) (string, error)
	completed    func(context.Context) (collectTargetBatchCompletion, error)
	owned        func(context.Context) (bool, error)
	run          func(context.Context) error
	finish       func(context.Context, collectTargetBatchCompletion) error
	observeReuse func(collectBatchReuseObservation)
}

// Errors (including retry errors) never acknowledge the durable event task.
// A completion is valid only for the generation observed before loading rows.
func runCollectTargetBatch(ctx context.Context, ops collectTargetBatchOps, now time.Time) error {
	generation, err := ops.generation(ctx)
	if err != nil {
		observeCollectBatchReuse(ops, collectBatchReuseObservation{
			Outcome: collectBatchReuseNotEvaluated, Reason: collectBatchReuseGenerationError,
		})
		return collectTargetBatchCacheError(err)
	}
	done, err := ops.completed(ctx)
	if err != nil {
		observeCollectBatchReuse(ops, collectBatchReuseObservation{
			Outcome: collectBatchReuseNotEvaluated, Reason: collectBatchReuseCompletionError, Generation: generation,
		})
		return collectTargetBatchCacheError(err)
	}
	mergeTTL := ops.mergeTTL
	if mergeTTL <= 0 {
		mergeTTL = collectMaterialWindowMergeTTLDefault
	}
	reuse := collectBatchReuseDecision(done, generation, ops.targetID, now, mergeTTL)
	observeCollectBatchReuse(ops, reuse)
	if reuse.Outcome == collectBatchReuseHit {
		return newCollectProcessRetryError(reuse.MergeUntil.Sub(now), "来源批次已合并，等待下一次状态推进")
	}
	owned, err := ops.owned(ctx)
	if err != nil {
		return collectTargetBatchCacheError(err)
	}
	if !owned {
		return newCollectProcessRetryError(collectProcessMinimumDelay, "采集批次执行租约已失效")
	}
	runErr := ops.run(ctx)
	if runErr != nil && !isCollectProcessRetryError(runErr) {
		return runErr
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	owned, err = ops.owned(ctx)
	if err != nil {
		return collectTargetBatchCacheError(err)
	}
	if !owned {
		return newCollectProcessRetryError(collectProcessMinimumDelay, "采集批次完成时租约已失效")
	}
	current, err := ops.generation(ctx)
	if err != nil {
		return collectTargetBatchCacheError(err)
	}
	if current != generation {
		return newCollectProcessRetryError(collectProcessMinimumDelay, "采集批次有新消息，保留唤醒")
	}
	delay := collectProcessMinimumDelay
	if retry, ok := runErr.(*collectProcessRetryError); ok {
		delay = retry.delay
	}
	var covered []int64
	if ops.coverage != nil {
		covered = ops.coverage()
	}
	finishedAt := time.Now()
	if err = ops.finish(ctx, collectTargetBatchCompletion{Generation: generation, CoveredEventIDs: covered, FinishedAt: finishedAt, RetryAt: finishedAt.Add(min(delay, mergeTTL))}); err != nil {
		return collectTargetBatchCacheError(err)
	}
	// Never consume another target's durable wakeup on the strength of a batch
	// result: its persisted terminal status is checked by the normal handler.
	if runErr != nil {
		return runErr
	}
	return newCollectProcessRetryError(delay, "采集批次完成，等待事件终态确认")
}

func collectTargetBatchProcessIDs(rows gdb.Result, targets ...int64) map[int64]struct{} {
	targetID := int64(0)
	if len(targets) > 0 {
		targetID = targets[0]
	}
	ids := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		if id := row["id"].Int64(); id > 0 {
			// Unmatched ignored verification is passive pairing context. An explicit
			// durable target may still process itself, but must not poll its peers.
			if id != targetID && collectMaterialEventNeedsPairRepair(row) {
				continue
			}
			ids[id] = struct{}{}
		}
	}
	return ids
}

// Exact membership is necessary: a truncated batch or another time window
// must never suppress a durable task whose target has not been visited.
func collectTargetBatchCovers(done collectTargetBatchCompletion, targetID int64) bool {
	if targetID <= 0 {
		return false
	}
	for _, id := range done.CoveredEventIDs {
		if id == targetID {
			return true
		}
	}
	return false
}

func registerCollectProcessRecoveryScope(ctx context.Context, p collectProcessQueuePayload) error {
	if p.TenantId <= 0 || p.AccountId < 0 || p.SourceId <= 0 {
		return gerror.New("采集恢复来源参数无效")
	}
	if !cache.Initialized() {
		return gerror.New("采集恢复缓存不可用")
	}
	return cache.SetAdd(ctx, collectProcessActiveSourcesKey, fmt.Sprintf("%d:%d:%d", p.TenantId, p.AccountId, p.SourceId))
}

func collectTargetBatchCacheError(err error) error {
	if err == nil {
		return nil
	}
	return newCollectProcessRetryError(collectProcessRedisRetryDelay, "Redis采集批次协调失败，保留持久任务重试: "+err.Error())
}
