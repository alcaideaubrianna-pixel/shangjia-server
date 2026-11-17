package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/gtrace"
)

type collectWindowQueryKind string

const (
	collectWindowQueryPrimary collectWindowQueryKind = "primary"
	collectWindowQueryRetry   collectWindowQueryKind = "retry"
)

type collectBatchReuseOutcome string
type collectBatchReuseReason string

const (
	collectBatchReuseHit          collectBatchReuseOutcome = "hit"
	collectBatchReuseMiss         collectBatchReuseOutcome = "miss"
	collectBatchReuseNotEvaluated collectBatchReuseOutcome = "not_evaluated"

	collectBatchReuseValid            collectBatchReuseReason = "completion_valid"
	collectBatchReuseNoCompletion     collectBatchReuseReason = "no_completion"
	collectBatchReuseNoGeneration     collectBatchReuseReason = "generation_missing"
	collectBatchReuseGenerationChange collectBatchReuseReason = "generation_changed"
	collectBatchReuseExpired          collectBatchReuseReason = "merge_expired"
	collectBatchReuseRetryDue         collectBatchReuseReason = "retry_due"
	collectBatchReuseNotCovered       collectBatchReuseReason = "target_not_covered"
	collectBatchReuseGenerationError  collectBatchReuseReason = "generation_read_failed"
	collectBatchReuseCompletionError  collectBatchReuseReason = "completion_read_failed"
	collectBatchReuseInvalidTarget    collectBatchReuseReason = "invalid_target"
	collectBatchReuseCacheError       collectBatchReuseReason = "cache_error"
	collectBatchReuseLeaseBusy        collectBatchReuseReason = "lease_busy"
)

type collectBatchReuseObservation struct {
	Outcome           collectBatchReuseOutcome
	Reason            collectBatchReuseReason
	Generation        string
	CoveredEventCount int
	MergeUntil        time.Time
}

type collectBatchRunObservation struct {
	Started         bool
	RowsLoaded      int
	WindowTruncated bool
}

func collectWindowObservationFields(ctx context.Context, payload collectProcessQueuePayload, target gdb.Record) g.Map {
	return g.Map{
		"log_type": "telegram_collect_pipeline", "traceId": gtrace.GetTraceID(ctx),
		"tenantId": payload.TenantId, "accountId": payload.AccountId, "sourceId": payload.SourceId,
		"sourceChatId": payload.SourceChatId, "canonicalChatId": canonicalCollectSourceChatID(payload.SourceChatId),
		"eventId": target["id"].Int64(), "collectEventId": target["id"].Int64(),
		"eventAccountId": target["account_id"].Int64(), "anchorMessageId": target["source_message_id"].Int64(),
	}
}

// This measures one ORM round trip, including connection waiting and result
// decoding. It is not PostgreSQL server execution time. No cache work, query
// construction, preceding query, or completion-event logging is inside this timer.
func observeCollectWindowQuery(ctx context.Context, payload collectProcessQueuePayload, target gdb.Record, kind collectWindowQueryKind, maxRows int, run func() (gdb.Result, error)) (gdb.Result, error) {
	startedAt := time.Now()
	rows, err := run()
	elapsed := time.Since(startedAt)
	fields := collectWindowObservationFields(ctx, payload, target)
	fields["event"] = "collect_window_query_completed"
	fields["stage"] = "collect.window." + string(kind)
	fields["queryKind"] = kind
	fields["durationMs"], fields["durationUs"] = elapsed.Milliseconds(), elapsed.Microseconds()
	fields["rows"], fields["queryCount"] = len(rows), 1
	fields["rowLimit"], fields["queryLimitReached"] = maxRows+1, len(rows) > maxRows
	fields["emptyQueryCount"], fields["queryErrorCount"] = 0, 0
	fields["queryOutcome"] = "success"
	if err != nil {
		fields["queryOutcome"], fields["queryErrorCount"] = "error", 1
		fields["errorMessage"] = err.Error()
		g.Log().Warning(ctx, "采集窗口查询失败", fields)
	} else {
		if len(rows) == 0 {
			fields["emptyQueryCount"] = 1
		}
		message := "采集实时窗口查询完成"
		if kind == collectWindowQueryRetry {
			message = "采集验证窗口查询完成"
		}
		g.Log().Info(ctx, message, fields)
	}
	return rows, err
}

func logCollectBatchReuse(ctx context.Context, payload collectProcessQueuePayload, target gdb.Record, reuse collectBatchReuseObservation, run collectBatchRunObservation, elapsed time.Duration, err error) {
	fields := collectWindowObservationFields(ctx, payload, target)
	fields["event"], fields["stage"] = "collect_target_batch_reuse", "collect.window.batch_reuse"
	fields["reuseOutcome"], fields["reuseReason"] = reuse.Outcome, reuse.Reason
	fields["reuseLookupCount"], fields["reuseHitCount"], fields["reuseMissCount"] = 0, 0, 0
	switch reuse.Outcome {
	case collectBatchReuseHit:
		fields["reuseLookupCount"], fields["reuseHitCount"] = 1, 1
	case collectBatchReuseMiss:
		fields["reuseLookupCount"], fields["reuseMissCount"] = 1, 1
	}
	fields["batchGeneration"], fields["coveredEventCount"] = reuse.Generation, reuse.CoveredEventCount
	fields["batchRunStarted"], fields["rowsLoaded"], fields["windowTruncated"] = run.Started, run.RowsLoaded, run.WindowTruncated
	fields["durationMs"], fields["durationUs"] = elapsed.Milliseconds(), elapsed.Microseconds()
	fields["batchResult"] = "completed"
	if err != nil {
		fields["batchResult"] = "failed"
		if isCollectProcessRetryError(err) {
			fields["batchResult"] = "deferred"
		}
		fields["errorMessage"] = err.Error()
	}
	if err != nil && !isCollectProcessRetryError(err) {
		g.Log().Warning(ctx, "采集窗口批次复用检查完成", fields)
	} else {
		g.Log().Info(ctx, "采集窗口批次复用检查完成", fields)
	}
}

func observeCollectBatchReuse(ops collectTargetBatchOps, observation collectBatchReuseObservation) {
	if ops.observeReuse != nil {
		ops.observeReuse(observation)
	}
}

func collectBatchReuseDecision(done collectTargetBatchCompletion, generation string, targetID int64, now time.Time, mergeTTL time.Duration) collectBatchReuseObservation {
	observation := collectBatchReuseObservation{
		Outcome: collectBatchReuseMiss, Generation: generation,
		CoveredEventCount: len(uniqueIds(done.CoveredEventIDs)),
	}
	if mergeTTL <= 0 {
		mergeTTL = collectMaterialWindowMergeTTLDefault
	}
	observation.MergeUntil = done.FinishedAt.Add(mergeTTL)
	if done.RetryAt.Before(observation.MergeUntil) {
		observation.MergeUntil = done.RetryAt
	}
	switch {
	case done.FinishedAt.IsZero():
		observation.Reason = collectBatchReuseNoCompletion
	case generation == "":
		observation.Reason = collectBatchReuseNoGeneration
	case done.Generation != generation:
		observation.Reason = collectBatchReuseGenerationChange
	case !now.Before(observation.MergeUntil):
		observation.Reason = collectBatchReuseExpired
		if !now.Before(done.RetryAt) && !done.RetryAt.After(done.FinishedAt.Add(mergeTTL)) {
			observation.Reason = collectBatchReuseRetryDue
		}
	case !collectTargetBatchCovers(done, targetID):
		observation.Reason = collectBatchReuseNotCovered
	default:
		observation.Outcome, observation.Reason = collectBatchReuseHit, collectBatchReuseValid
	}
	return observation
}
