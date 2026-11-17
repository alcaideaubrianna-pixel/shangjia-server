package sys

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"

	publishconsts "hotgo/addons/youban_publish/consts"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

const (
	// Grouping starts immediately. The only waiting window is the twenty-minute
	// verification window; a display message without a match is published when
	// that window expires.
	collectMaterialGroupingDelay           = 0
	collectMaterialVerifyWindowDefault     = 20 * time.Minute
	collectMaterialVerifySettleDelay       = time.Minute
	collectMaterialVerifyRetryDelay        = time.Minute
	collectMaterialWaitingVerifyRetryDelay = 10 * time.Second
	collectMaterialWindowExecutionTimeout  = 90 * time.Second
	collectMaterialWindowMergeTTLDefault   = 3 * time.Second
	collectMaterialCandidateRebuildTTL     = 5 * time.Second
	collectMaterialRolePending             = "pending"
	collectMaterialRoleDisplay             = "display"
	collectMaterialRoleVerify              = "verify"
	collectMaterialGroupCollecting         = "group_collecting"
	collectMaterialGroupWaitingVerify      = "waiting_verify"
)

func collectMaterialWindowMergeTTL(ctx context.Context) time.Duration {
	value := strings.TrimSpace(os.Getenv("YOUBAN_PUBLISH_COLLECT_WINDOW_MERGE_TTL"))
	if value == "" {
		return collectMaterialWindowMergeTTLDefault
	}
	if duration, err := time.ParseDuration(value); err == nil && duration > 0 && duration <= time.Minute {
		return duration
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 && seconds <= 60 {
		return time.Duration(seconds) * time.Second
	}
	g.Log().Warning(ctx, "采集窗口合并 TTL 配置无效，使用默认值", g.Map{"value": value, "default": collectMaterialWindowMergeTTLDefault.String()})
	return collectMaterialWindowMergeTTLDefault
}

func reserveCollectMaterialCandidateRebuild(ctx context.Context, payload collectProcessQueuePayload) (bool, error) {
	if !cache.Initialized() {
		return true, nil
	}
	key := collectProcessCandidateIndexKey(payload) + ":rebuild"
	ok, err := cache.Instance().SetIfNotExist(ctx, key, 1, collectMaterialCandidateRebuildTTL)
	if err != nil {
		// Redis is an accelerator; PostgreSQL fallback remains the correctness
		// path when Redis is unavailable.
		return true, nil
	}
	return ok, nil
}

// collectMaterialVerifyWindow is the single source of truth for display/verify pairing.
func collectMaterialVerifyWindow() time.Duration {
	value := strings.TrimSpace(os.Getenv("YOUBAN_COLLECT_VERIFY_WINDOW"))
	if value == "" {
		return collectMaterialVerifyWindowDefault
	}
	if duration, err := time.ParseDuration(value); err == nil && duration > 0 {
		return duration
	}
	if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return collectMaterialVerifyWindowDefault
}

func collectMaterialWindowMaxRows(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "youbanPublish.collect.materialWindowMaxRows", 100).Int()
	if value < 20 {
		return 20
	}
	if value > 1000 {
		return 1000
	}
	return value
}

func (s *sSysPublish) processCollectSourceWindow(ctx context.Context, payload collectProcessQueuePayload) error {
	// BOT 采集没有协议号账户，account_id=0 是合法的租户来源分片。
	if !validCollectSourceWindowPayload(payload) {
		return gerror.New("采集源窗口参数不完整")
	}
	batchSize := collectMaterialWindowBatchSize(ctx)
	if maxRows := collectMaterialWindowMaxRows(ctx); batchSize > maxRows {
		batchSize = maxRows
	}
	// Redis 只提供候选 ID，PG 仍通过完整窗口条件、租户/账户/来源和状态
	// 校验事实。索引为空、过期或 Redis 不可用时回退原始 PG 扫描，并用结果回填。
	candidateIDs := collectProcessCandidateIDs(ctx, payload, batchSize*collectProcessCandidateIndexReadFactor)
	candidateMode := len(candidateIDs) > 0
	candidateModel := func(model *gdb.Model) *gdb.Model {
		if candidateMode {
			return model.WhereIn("e.id", candidateIDs)
		}
		return model
	}

	// 只选择仍需推进的展示/待分类事件。已忽略的验证不是主动恢复任务。
	eventModel := candidateModel(collectMaterialActiveWindowModel(ctx, payload)).
		Where(collectMaterialGroupReadyCondition(), collectMaterialGroupWaitingVerify)
	if payload.EventId > 0 {
		eventModel = eventModel.Where("e.id", payload.EventId)
	}
	rows, err := eventModel.
		Fields("e.*").
		OrderAsc("e.updated_at").
		OrderAsc("e.id").
		Limit(batchSize).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取采集源消息窗口失败")
	}
	// Ignored verification remains available to an arriving display's bounded
	// target/lookback queries. It is not active work for a source sweep: polling
	// the same unmatched row cannot manufacture a missing display group.

	// A stale/incomplete index must not hide work. One bounded PG fallback rebuilds
	// the index for this shard; normal indexed cycles avoid the source-range scan.
	if len(rows) == 0 && candidateMode && payload.EventId == 0 {
		rebuildReserved, rebuildErr := reserveCollectMaterialCandidateRebuild(ctx, payload)
		if rebuildErr != nil {
			return rebuildErr
		}
		if !rebuildReserved {
			return newCollectProcessRetryError(collectMaterialWindowMergeTTL(ctx), "采集候选批次正在重建")
		}
		fallbackModel := collectMaterialActiveWindowModel(ctx, payload).
			Where(collectMaterialGroupReadyCondition(), collectMaterialGroupWaitingVerify)
		rows, err = fallbackModel.Fields("e.*").OrderAsc("e.updated_at").OrderAsc("e.id").Limit(batchSize).All()
		if err != nil {
			return gerror.Wrap(err, "回退读取采集源消息窗口失败")
		}
	}
	if len(rows) == 0 {
		return nil
	}
	for _, event := range rows {
		targetPayload := payload
		targetPayload.EventId = event["id"].Int64()
		targetPayload.SourceChatId = event["source_chat_id"].String()
		indexCollectProcessCandidate(ctx, targetPayload, targetPayload.EventId)
		if err = s.processTargetCollectEventWindow(ctx, targetPayload, event); err != nil {
			if isCollectProcessRetryError(err) {
				continue
			}
			return err
		}
		removeCollectProcessCandidate(ctx, targetPayload, targetPayload.EventId)
	}
	return nil
}

// validCollectSourceWindowPayload 允许 BOT 采集使用 account_id=0，同时拒绝无租户或无来源的任务。
func validCollectSourceWindowPayload(payload collectProcessQueuePayload) bool {
	return payload.SourceId > 0 && payload.TenantId > 0 && payload.AccountId >= 0
}

// collectMaterialWindowModel 构造账户和来源分片查询，隔离正常事件与验证重试事件。
func collectMaterialWindowModel(ctx context.Context, payload collectProcessQueuePayload, verifyOnly bool) *gdb.Model {
	model := collectProcessEventModel(ctx, payload)
	if verifyOnly {
		return model.Where("e.status", sysin.CollectEventStatusIgnored).
			Where("e.material_role", collectMaterialRoleVerify).
			Where("e.error_message", collectMaterialVerifyUnmatchedMessage)
	}
	return model.Where("e.status <> ?", sysin.CollectEventStatusIgnored).
		Where("(e.material_role IS NULL OR e.material_role = '' OR e.material_role IN (?,?))", collectMaterialRolePending, collectMaterialRoleDisplay)
}

// collectMaterialGroupReadyCondition 在 PostgreSQL 中使用 IS DISTINCT FROM，避免 NULL OR
// 造成 BitmapOr；其他数据库保留兼容条件。
func collectMaterialGroupReadyCondition() string {
	if strings.EqualFold(strings.TrimSpace(g.DB().GetConfig().Type), "pgsql") {
		return "e.material_group_status IS DISTINCT FROM ?"
	}
	return "(e.material_group_status IS NULL OR e.material_group_status <> ?)"
}

// mergeCollectMaterialWindowRows 合并两个有界查询结果并按消息顺序截断，保证恢复与实时任务幂等。
func mergeCollectMaterialWindowRows(primary, retry gdb.Result, limit int) gdb.Result {
	seen := make(map[int64]struct{}, len(primary)+len(retry))
	rows := make(gdb.Result, 0, len(primary)+len(retry))
	for _, row := range append(primary, retry...) {
		id := row["id"].Int64()
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		left, right := rows[i]["source_message_id"].Int64(), rows[j]["source_message_id"].Int64()
		if left == right {
			return rows[i]["id"].Int64() < rows[j]["id"].Int64()
		}
		return left < right
	})
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

// processTargetCollectEventWindow processes one realtime event with only the
// forward context required to pair a verification message. It deliberately
// avoids the source backlog so old events cannot delay realtime ingestion.
func (s *sSysPublish) processTargetCollectEventWindow(ctx context.Context, payload collectProcessQueuePayload, target gdb.Record) (resultErr error) {
	ctx = collectBusinessEventContext(ctx, target, nil, 0)
	startedAt := time.Now()
	reuse := collectBatchReuseObservation{Outcome: collectBatchReuseNotEvaluated, Reason: collectBatchReuseInvalidTarget}
	var batchRun collectBatchRunObservation
	defer func() {
		g.Log().Info(ctx, "采集实时事件窗口处理完成", g.Map{"stage": "collect.window.process", "tenantId": payload.TenantId, "accountId": payload.AccountId, "sourceId": payload.SourceId, "eventId": target["id"].Int64(), "durationMs": time.Since(startedAt).Milliseconds()})
		logCollectBatchReuse(ctx, payload, target, reuse, batchRun, time.Since(startedAt), resultErr)
	}()
	if target.IsEmpty() || target["id"].Int64() <= 0 {
		return gerror.New("目标采集事件不存在")
	}
	if collectProcessEventFinished(target) && !collectMaterialLateVerificationRepairCandidate(target) {
		reuse.Reason = collectBatchReuseTerminalTarget
		return nil
	}
	messageID := target["source_message_id"].Int64()
	if messageID <= 0 {
		return gerror.Newf("目标采集事件消息ID无效 eventId:%d", target["id"].Int64())
	}
	eventAt := target["received_at"].GTime()
	if eventAt == nil || eventAt.IsZero() {
		eventAt = target["created_at"].GTime()
	}
	if eventAt == nil || eventAt.IsZero() {
		return gerror.Newf("目标采集事件时间无效 eventId:%d", target["id"].Int64())
	}
	payload.SourceChatId = target["source_chat_id"].String()
	reuse.Reason = collectBatchReuseCacheError
	key := collectTargetBatchKey(payload)
	lease, err := cache.NewQueueLease(key+":lease", "collect_target_batch", collectMaterialWindowExecutionTimeout+10*time.Second)
	if err != nil {
		return collectTargetBatchCacheError(err)
	}
	acquired, err := lease.TryAcquire(ctx)
	if err != nil {
		return collectTargetBatchCacheError(err)
	}
	if !acquired {
		reuse.Reason = collectBatchReuseLeaseBusy
		return newCollectProcessRetryError(collectProcessMinimumDelay, "来源批次正在处理")
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = lease.Unlock(releaseCtx)
	}()
	readGeneration := func(ctx context.Context) (string, error) {
		value, err := cache.Instance().Get(ctx, key+":dirty")
		if err != nil {
			return "", err
		}
		if value.IsNil() {
			return "", gerror.New("采集批次generation缺失，保留任务重试")
		}
		return value.String(), nil
	}
	// Legacy durable tasks have no producer generation yet. Registration is safe
	// under the per-chat lease and does not remove an arriving producer's marker.
	if value, readErr := cache.Instance().Get(ctx, key+":dirty"); readErr != nil {
		return collectTargetBatchCacheError(readErr)
	} else if value.IsNil() {
		if _, err = cache.Instance().SetIfNotExist(ctx, key+":dirty", target["id"].String(), collectTargetBatchTTL); err != nil {
			return collectTargetBatchCacheError(err)
		}
	}
	var coveredIDs []int64
	return runCollectTargetBatch(ctx, collectTargetBatchOps{
		targetID: target["id"].Int64(),
		mergeTTL: collectMaterialWindowMergeTTL(ctx),
		observeReuse: func(observation collectBatchReuseObservation) {
			reuse = observation
		},
		coverage:   func() []int64 { return coveredIDs },
		generation: readGeneration,
		completed: func(ctx context.Context) (collectTargetBatchCompletion, error) {
			var done collectTargetBatchCompletion
			value, err := cache.Instance().Get(ctx, key+":completed")
			if err != nil {
				return done, err
			}
			if !value.IsNil() {
				err = value.Struct(&done)
			}
			return done, err
		},
		owned: lease.Renew,
		finish: func(ctx context.Context, done collectTargetBatchCompletion) error {
			return cache.Instance().Set(ctx, key+":completed", done, collectMaterialWindowMergeTTL(ctx))
		},
		run: func(ctx context.Context) error {
			batchRun.Started = true
			rows, truncated, err := s.loadTargetCollectMaterialWindow(ctx, payload, target, eventAt)
			batchRun.RowsLoaded, batchRun.WindowTruncated = len(rows), truncated
			if err != nil {
				return gerror.Wrap(err, "读取采集合并批次失败")
			}
			if len(rows) == 0 {
				return newCollectProcessRetryError(collectMaterialWaitingVerifyRetryDelay, "目标批次等待状态更新")
			}
			visits := &collectTargetBatchVisit{}
			batchCtx := context.WithValue(ctx, collectTargetBatchVisitKey{}, visits)
			processErr := s.processCollectMessageWindow(batchCtx, payload, payload.SourceChatId, rows, collectTargetBatchProcessIDs(rows), truncated)
			coveredIDs = visits.ids
			return processErr
		},
	}, time.Now())
}

func (s *sSysPublish) loadTargetCollectMaterialWindow(ctx context.Context, payload collectProcessQueuePayload, target gdb.Record, eventAt *gtime.Time) (gdb.Result, bool, error) {
	// The current durable target is always the anchor. Source candidates can
	// contain old history/waiting_verify rows; never move realtime back to them.
	if !cache.Initialized() {
		return nil, false, newCollectProcessRetryError(collectProcessRedisRetryDelay, "采集批次缓存未就绪")
	}
	if _, cacheErr := cache.Instance().Get(ctx, collectTargetBatchKey(payload)+":dirty"); cacheErr != nil {
		return nil, false, collectTargetBatchCacheError(cacheErr)
	}
	maxRows := collectMaterialWindowMaxRows(ctx)
	load := func(verifyOnly bool) (gdb.Result, error) {
		model := collectMaterialWindowModel(ctx, payload, verifyOnly).
			Where("e.source_message_id >= ?", target["source_message_id"].Int64()).
			Where("(e.received_at >= ? OR (e.received_at IS NULL AND e.created_at >= ?))", eventAt.Add(-collectMaterialVerifyWindow()), eventAt.Add(-collectMaterialVerifyWindow())).
			// Keep the timestamp predicate sargable for rows with and without
			// received_at. COALESCE on the column forced PostgreSQL to evaluate
			// the expression for a broad source range before applying LIMIT.
			Where("(e.received_at <= ? OR (e.received_at IS NULL AND e.created_at <= ?))",
				eventAt.Add(collectMaterialVerifyWindow()), eventAt.Add(collectMaterialVerifyWindow()))
		// A new target is already serialized by its channel batch lease. Read
		// passive unmatched verification immediately: its old polling cooldown
		// must not hide it when the previously missing display has just arrived.
		if !verifyOnly {
			model = model.Where("e.processed_at IS NULL").
				Where(collectMaterialGroupReadyCondition(), collectMaterialGroupWaitingVerify)
		}
		query := model.Fields("e.*").OrderAsc("e.source_message_id").OrderAsc("e.id").Limit(maxRows + 1)
		kind := collectWindowQueryPrimary
		if verifyOnly {
			kind = collectWindowQueryRetry
		}
		return observeCollectWindowQuery(ctx, payload, target, kind, maxRows, func() (gdb.Result, error) {
			return query.All()
		})
	}
	primary, err := load(false)
	if err != nil {
		return nil, false, err
	}
	retry, err := load(true)
	if err != nil {
		return nil, false, err
	}
	rows := mergeCollectMaterialWindowRows(primary, retry, maxRows+1)
	windowTruncated := len(rows) > maxRows
	if windowTruncated {
		rows = rows[:maxRows]
	}
	return rows, windowTruncated, nil
}

func (s *sSysPublish) processCollectMessageWindow(ctx context.Context, payload collectProcessQueuePayload, chatID string, rows []gdb.Record, processEventIds map[int64]struct{}, windowTruncated bool) error {
	startedAt := time.Now()
	defer func() {
		g.Log().Info(ctx, "采集消息窗口处理完成", g.Map{"stage": "collect.message.process", "tenantId": payload.TenantId, "accountId": payload.AccountId, "sourceId": payload.SourceId, "rows": len(rows), "durationMs": time.Since(startedAt).Milliseconds()})
	}()
	sort.SliceStable(rows, func(i, j int) bool {
		left := rows[i]["source_message_id"].Int64()
		right := rows[j]["source_message_id"].Int64()
		if left == right {
			return rows[i]["id"].Int64() < rows[j]["id"].Int64()
		}
		return left < right
	})
	eventIds := make([]int64, 0, len(rows))
	for _, row := range rows {
		eventIds = append(eventIds, row["id"].Int64())
	}
	mediaByEvent, err := s.collectEventMediaItemsByEvent(ctx, eventIds)
	if err != nil {
		return err
	}
	messageViews := collectMaterialEventViews(rows, mediaByEvent)
	return runCollectTargetBatchEvents(ctx, len(rows), func(index int) (windowErr error) {
		event := rows[index]
		if _, ok := processEventIds[event["id"].Int64()]; !ok {
			return nil
		}
		visitCollectTargetBatch(ctx, event["id"].Int64())
		pairParent := collectBusinessEventContext(ctx, event, nil, 0)
		ctx, finishPair := startCollectPipelineStage(pairParent, collectPipelineMaterialPair)
		pairFinished := false
		pairOutcome := collectPipelinePending
		finishPairOnce := func(result collectPipelineResult, err error) {
			if !pairFinished {
				finishPair(result, err)
				pairFinished = true
			}
		}
		defer func() { finishPairOnce(collectBusinessStepResult(windowErr, pairOutcome), windowErr) }()
		role := strings.TrimSpace(event["material_role"].String())
		if shouldResumeClassifiedDisplayEvent(event) {
			settled, settleErr := s.collectVerifyEventsSettled(ctx, event, collectMaterialContextClosed(messageViews, index))
			if settleErr != nil {
				return settleErr
			}
			if !settled {
				return newCollectProcessRetryError(s.collectDisplayRetryDelay(ctx, event), "等待验证资料静默期结束")
			}
			finishPairOnce(collectPipelineCompleted, nil)
			ctx = pairParent
			if err = s.processCollectEvent(ctx, event["id"].Int64(), payload.TenantId, payload.AccountId); err != nil {
				// 目标事件的可重试错误必须回传 Outbox，不能误完成后遗留 media_ready。
				return err
			}
			return nil
		}
		if role != "" && role != collectMaterialRolePending && !collectMaterialEventNeedsPairRepair(event) {
			pairOutcome = collectPipelineDuplicate
			return nil
		}
		if !collectMaterialEventOlderThan(event, collectMaterialGroupingDelay) {
			return nil
		}
		classification, err := classifyCollectEventMedia(ctx, event, strings.TrimSpace(event["raw_text"].String()), mediaByEvent[event["id"].Int64()])
		if err != nil {
			return err
		}
		switch classification.Kind {
		case profileMessageKindDisplay:
			if err = s.wakePreviousCollectDisplay(ctx, payload, event); err != nil {
				return err
			}
			verifyIndexes := s.findCollectVerifyEvents(rows, messageViews, index)
			if len(verifyIndexes) > 0 {
				pairedAny := false
				for _, verifyIndex := range verifyIndexes {
					paired, pairErr := s.bindCollectMaterialPair(ctx, event, rows[verifyIndex])
					if pairErr != nil {
						return pairErr
					}
					pairedAny = pairedAny || paired.IsBound()
				}
				if !pairedAny {
					return nil
				}
				// 当前窗口已经完成展示/验证配对，立即处理父展示事件。
				// 不能沿用配对前的事件快照继续等待验证静默窗口。
				finishPairOnce(collectPipelineCompleted, nil)
				ctx = pairParent
				if err = s.processCollectEvent(ctx, event["id"].Int64(), payload.TenantId, payload.AccountId); err != nil {
					return err
				}
				return nil
			}
			if collectMaterialContextClosed(messageViews, index) {
				claimed, claimErr := s.markCollectMaterialRole(ctx, event["id"].Int64(), collectMaterialRoleDisplay, 0, "complete")
				if claimErr != nil {
					return claimErr
				}
				if claimed {
					finishPairOnce(collectPipelineCompleted, nil)
					ctx = pairParent
					if err = s.processCollectEvent(ctx, event["id"].Int64(), payload.TenantId, payload.AccountId); err != nil {
						// 目标事件的重试错误必须回传 Outbox，不能误完成后遗留 media_ready。
						return err
					}
				}
				return nil
			}
			if !collectClassificationHasVerifyMedia(classification) && collectMaterialShouldWaitForVerify(event, windowTruncated) {
				if err = s.markCollectMaterialWaitingVerify(ctx, event["id"].Int64()); err != nil {
					return err
				}
				return newCollectProcessRetryError(s.collectDisplayRetryDelay(ctx, event), "等待验证资料或窗口到期")
			}
			claimed, claimErr := s.markCollectMaterialRole(ctx, event["id"].Int64(), collectMaterialRoleDisplay, 0, "complete")
			if claimErr != nil {
				return claimErr
			}
			if !claimed {
				return nil
			}
			s.appendCollectEventLogForRecord(ctx, event, "group", "timeout_without_verify", "验证窗口到期，按无验证资料继续处理", "")
			finishPairOnce(collectPipelineCompleted, nil)
			ctx = pairParent
			if err = s.processCollectEvent(ctx, event["id"].Int64(), payload.TenantId, payload.AccountId); err != nil {
				// 当前事件是本次窗口的目标，重试错误必须回到 Outbox 调度，避免状态停在 media_ready。
				return err
			}
		case profileMessageKindVerify:
			displayIndex := s.findCollectDisplayEvent(rows, messageViews, index)
			if displayIndex < 0 {
				storedDisplay, lookupErr := s.findStoredCollectDisplayEvent(ctx, event)
				if lookupErr != nil {
					return lookupErr
				}
				if !storedDisplay.IsEmpty() {
					paired, pairErr := s.bindCollectMaterialPair(ctx, storedDisplay, event)
					if pairErr != nil {
						return pairErr
					}
					if !paired.IsBound() {
						return nil
					}
					pairOutcome = collectPipelineCompleted
					if err = s.scheduleCollectDisplaySettlement(ctx, payload, storedDisplay, 0); err != nil {
						return err
					}
					return nil
				}
			}
			if displayIndex >= 0 {
				display := rows[displayIndex]
				paired, pairErr := s.bindCollectMaterialPair(ctx, display, event)
				if pairErr != nil {
					return pairErr
				}
				if !paired.IsBound() {
					return nil
				}
				pairOutcome = collectPipelineCompleted
				if err = s.scheduleCollectDisplaySettlement(ctx, payload, display, 0); err != nil {
					return err
				}
				return nil
			}
			if !collectMaterialEventIngestOlderThan(event, collectMaterialVerifyWindow()) {
				return newCollectProcessRetryError(collectMaterialVerifyRetryDelay, "验证资料等待前序展示资料落库")
			}
			// Persist the classification before marking the event ignored. The
			// bounded retry query only admits ignored verification events; leaving
			// this as pending makes a valid video permanently unrecoverable.
			claimed, claimErr := s.markCollectMaterialRole(ctx, event["id"].Int64(), collectMaterialRoleVerify, 0, "unmatched")
			if claimErr != nil {
				return claimErr
			}
			if !claimed && strings.TrimSpace(event["material_role"].String()) != collectMaterialRoleVerify {
				return nil
			}
			pairOutcome = collectPipelineIgnored
			if err = s.ignoreCollectEvent(ctx, event["id"].Int64(), collectMaterialVerifyUnmatchedMessage, "group"); err != nil {
				return err
			}
			g.Log().Infof(ctx, "验证资料在窗口内未找到前序展示组，已终止自动配对 eventId:%d sourceMessageId:%d window:%s", event["id"].Int64(), event["source_message_id"].Int64(), collectMaterialVerifyWindow())
			return nil
		default:
			pairOutcome = collectPipelineIgnored
			if err = s.ignoreCollectEvent(ctx, event["id"].Int64(), "消息不是资料组或验证组", "group"); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *sSysPublish) scheduleCollectDisplaySettlement(ctx context.Context, payload collectProcessQueuePayload, display gdb.Record, delay time.Duration) error {
	if display.IsEmpty() || display["id"].Int64() <= 0 {
		return nil
	}
	target := payload
	if payload.EventId != display["id"].Int64() {
		// An Outbox claim belongs to its original event, never the parent.
		// Reusing it also reuses the Asynq TaskID and can suppress this wakeup.
		target.OutboxOwner = ""
	}
	target.EventId = display["id"].Int64()
	target.SourceId = display["source_id"].Int64()
	target.SourceChatId = display["source_chat_id"].String()
	if delay < collectProcessMinimumDelay {
		delay = collectProcessMinimumDelay
	}
	_, err := s.enqueueCollectProcessDeferred(ctx, target, delay)
	return gerror.Wrap(err, "投递展示资料截止任务失败")
}

func (s *sSysPublish) wakePreviousCollectDisplay(ctx context.Context, payload collectProcessQueuePayload, current gdb.Record) error {
	previous, err := s.findStoredCollectDisplayEvent(ctx, current)
	if err != nil || previous.IsEmpty() || previous["id"].Int64() == current["id"].Int64() {
		return err
	}
	return s.scheduleCollectDisplaySettlement(ctx, payload, previous, 0)
}

func (s *sSysPublish) collectDisplayRetryDelay(ctx context.Context, display gdb.Record) time.Duration {
	deadline := collectMaterialEventAt(display).Add(collectMaterialVerifyWindow() + collectMaterialVerifySettleDelay)
	if verifies, err := s.pairedCollectVerifyEvents(ctx, display["id"].Int64()); err == nil && len(verifies) > 0 {
		latest := collectMaterialEventAt(verifies[len(verifies)-1])
		if !latest.IsZero() {
			deadline = latest.Add(collectMaterialVerifySettleDelay)
			maximum := collectMaterialEventAt(display).Add(collectMaterialVerifyWindow() + collectMaterialVerifySettleDelay)
			if !maximum.IsZero() && maximum.Before(deadline) {
				deadline = maximum
			}
		}
	}
	delay := time.Until(deadline)
	if delay < collectProcessMinimumDelay {
		return collectProcessMinimumDelay
	}
	return delay
}

// collectClassificationHasVerifyMedia 判断同一消息是否已经包含验证媒体；包含时无需等待外部验证事件。
func collectClassificationHasVerifyMedia(classification profileMessageClassification) bool {
	for _, media := range classification.Media {
		if strings.EqualFold(strings.TrimSpace(media.Purpose), collectMaterialRoleVerify) {
			return true
		}
	}
	return false
}

func collectMaterialShouldWaitForVerify(event gdb.Record, windowTruncated bool) bool {
	return windowTruncated || !collectMaterialEventIngestOlderThan(event, collectMaterialVerifyWindow()+collectMaterialVerifySettleDelay)
}

func shouldResumeClassifiedDisplayEvent(event gdb.Record) bool {
	if strings.TrimSpace(event["material_role"].String()) != collectMaterialRoleDisplay {
		return false
	}
	status := strings.TrimSpace(event["status"].String())
	return status != sysin.CollectEventStatusProcessed &&
		status != sysin.CollectEventStatusDispatched &&
		status != sysin.CollectEventStatusIgnored
}

const collectMaterialVerifyUnmatchedMessage = "验证资料未匹配到前序资料组"

var errCollectMaterialPairRace = errors.New("采集资料组已被并发处理")

func collectMaterialMutableStatuses() []string {
	return []string{
		sysin.CollectEventStatusPending,
		sysin.CollectEventStatusGroupCollect,
		sysin.CollectEventStatusWaitingOrder,
		sysin.CollectEventStatusPrechecked,
		sysin.CollectEventStatusMediaPending,
		sysin.CollectEventStatusMediaReady,
		sysin.CollectEventStatusFailed,
	}
}

func collectMaterialEventNeedsPairRepair(event gdb.Record) bool {
	return event["status"].String() == sysin.CollectEventStatusIgnored &&
		strings.TrimSpace(event["material_role"].String()) == collectMaterialRoleVerify &&
		strings.TrimSpace(event["error_message"].String()) == collectMaterialVerifyUnmatchedMessage
}

func isCollectProcessRetryError(err error) bool {
	var retryErr *collectProcessRetryError
	return errors.As(err, &retryErr)
}

func (s *sSysPublish) findCollectVerifyEvents(rows []gdb.Record, views []collectMaterialMessageView, displayIndex int) []int {
	indexes := make([]int, 0)
	for _, pair := range pairCollectMaterialMessages(views) {
		if pair.DisplayIndex != displayIndex {
			continue
		}
		candidate := rows[pair.VerifyIndex]
		if strings.TrimSpace(candidate["material_role"].String()) != "" && strings.TrimSpace(candidate["material_role"].String()) != collectMaterialRolePending && !collectMaterialEventNeedsPairRepair(candidate) {
			continue
		}
		if !collectMaterialEventOlderThan(candidate, collectMaterialGroupingDelay) {
			continue
		}
		indexes = append(indexes, pair.VerifyIndex)
	}
	return indexes
}

type collectMaterialLookbackCursor struct {
	MessageID   int64  `json:"messageId"`
	EventID     int64  `json:"eventId"`
	HighWaterID int64  `json:"highWaterId"`
	Generation  string `json:"generation"`
}

// A missing dirty token is not evidence that old cache pages are current.
// Producers rotate this token for ingress/edits; deferred retries do not.
func collectMaterialLookbackGeneration(ctx context.Context, verify gdb.Record) (string, error) {
	if !cache.Initialized() {
		return "", newCollectProcessRetryError(5*time.Second, "回看缓存未初始化，等待来源 dirty generation")
	}
	payload := collectProcessQueuePayload{TenantId: verify["tenant_id"].Int64(), AccountId: verify["account_id"].Int64(), SourceId: verify["source_id"].Int64(), SourceChatId: verify["source_chat_id"].String()}
	key := collectTargetBatchKey(payload) + ":dirty"
	value, err := cache.Instance().Get(ctx, key)
	if err != nil {
		return "", newCollectProcessRetryError(5*time.Second, "读取来源 dirty generation 失败，延迟回看")
	}
	if value == nil || value.IsNil() {
		// Old source-window tasks may never pass through realtime initialization.
		// Bootstrap only a missing token; concurrent ingress keeps its own token.
		if _, err := cache.Instance().SetIfNotExist(ctx, key, guid.S(), collectTargetBatchTTL); err != nil {
			return "", newCollectProcessRetryError(5*time.Second, "初始化来源 dirty generation 失败，延迟回看")
		}
		value, err = cache.Instance().Get(ctx, key)
	}
	if err != nil || value == nil || value.IsNil() || value.String() == "" {
		return "", newCollectProcessRetryError(5*time.Second, "来源 dirty generation 不可用，延迟回看")
	}

	return value.String(), nil
}

func collectMaterialResumeLookback(cursor collectMaterialLookbackCursor, generation string) collectMaterialLookbackCursor {
	// Chat-wide ingress is not a reason to discard bounded history progress.
	// Fresh inserts are included via HighWaterID; canonical validation checks
	// the complete candidate interval again before a binding can be committed.
	cursor.Generation = generation
	return cursor
}

func collectMaterialAdvanceLookback(rows []gdb.Record) collectMaterialLookbackCursor {
	if len(rows) == 0 {
		return collectMaterialLookbackCursor{}
	}
	last := rows[len(rows)-1]
	cursor := collectMaterialLookbackCursor{MessageID: last["source_message_id"].Int64(), EventID: last["id"].Int64()}
	for _, row := range rows {
		if id := row["id"].Int64(); id > cursor.HighWaterID {
			cursor.HighWaterID = id
		}
	}
	return cursor
}

func collectMaterialLookbackCursorKey(verify gdb.Record) string {
	return fmt.Sprintf("youban_publish:collect:lookback:%d:%d:%d:%s:%d", verify["tenant_id"].Int64(), verify["account_id"].Int64(), verify["source_id"].Int64(), canonicalCollectSourceChatID(verify["source_chat_id"].String()), verify["id"].Int64())
}

// Finding a parent is not completion: canonical validation reuses this page.
// Only a conclusive negative lookup or a committed binding clears the cursor.
func collectMaterialLookbackCanClear(result gdb.Record, decided, truncated bool) bool {
	return result.IsEmpty() && (decided || !truncated)
}

// findStoredCollectDisplayEvent reads at most one page per channel-ID alias
// per attempt, sharing the same execution deadline. Partial history ZSETs cannot prove adjacency, so
// this path uses a per-verification keyset cursor instead of cached ID filters.
func (s *sSysPublish) findStoredCollectDisplayEvent(ctx context.Context, verify gdb.Record) (resultRecord gdb.Record, resultErr error) {
	defer restoreCollectMaterialTransactionContext(ctx)()
	startedAt := time.Now()
	parentCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	defer func() {
		if resultErr != nil && parentCtx.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			g.Log().Warning(parentCtx, "验证资料回看超时，保留游标等待重试", g.Map{
				"stage": "collect.lookback.timeout", "eventId": verify["id"].Int64(),
				"tenantId": verify["tenant_id"].Int64(), "accountId": verify["account_id"].Int64(),
				"sourceId": verify["source_id"].Int64(), "sourceChatId": verify["source_chat_id"].String(),
				"durationMs": time.Since(startedAt).Milliseconds(),
			})
		}
		resultErr = collectMaterialLookbackRetryError(parentCtx, ctx, resultErr)
	}()
	if verify["tenant_id"].Int64() <= 0 || verify["account_id"].Int64() < 0 || verify["source_id"].Int64() <= 0 || verify["source_chat_id"].String() == "" {
		return nil, gerror.New("验证资料回看 scope 不完整")
	}
	messageID := verify["source_message_id"].Int64()
	if messageID <= 0 {
		return nil, nil
	}
	verifyAt := verify["received_at"].GTime()
	if verifyAt == nil {
		verifyAt = verify["created_at"].GTime()
	}
	if verifyAt == nil || verifyAt.IsZero() {
		return nil, nil
	}
	generation, generationErr := collectMaterialLookbackGeneration(ctx, verify)
	if generationErr != nil {
		return nil, generationErr
	}
	// Generation invalidates continuation state only. A positive result is
	// checked against its finite message interval by bindCollectMaterialPair;
	// unrelated future ingress must not veto that transaction.
	const pageSize = 100

	// Scope and verification identity isolate bounded history continuations.
	cursorKey := collectMaterialLookbackCursorKey(verify)
	var cursor collectMaterialLookbackCursor
	if cache.Initialized() && verify["id"].Int64() > 0 {
		cached, err := cache.Instance().Get(ctx, cursorKey)
		if err != nil {
			return nil, gerror.Wrap(err, "读取验证资料回看游标失败")
		}
		if cached != nil && !cached.IsNil() {
			if err := cached.Scan(&cursor); err != nil {
				return nil, gerror.Wrap(err, "解析验证资料回看游标失败")
			}
		}
	}
	cursor = collectMaterialResumeLookback(cursor, generation)
	rows, err := loadCollectMaterialLookbackPage(ctx, verify, verifyAt, cursor, pageSize+1)
	if err != nil {
		return nil, gerror.Wrap(err, "回看验证资料前序展示组失败")
	}
	truncated := len(rows) > pageSize
	if truncated {
		rows = rows[:pageSize]
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row["id"].Int64())
	}
	if len(ids) == 0 {
		if cache.Initialized() {
			if _, err := cache.Instance().Remove(ctx, cursorKey); err != nil {
				return nil, gerror.Wrap(err, "清理空验证资料回看游标失败")
			}
		}
		return nil, nil
	}
	media, err := s.collectEventMediaItemsByEvent(ctx, ids)
	if err != nil {
		return nil, err
	}
	result, decided := collectStoredDisplayDecision(rows, media, verify)
	if decided || !truncated {
		if collectMaterialLookbackCanClear(result, decided, truncated) && cache.Initialized() {
			if _, err := cache.Instance().Remove(ctx, cursorKey); err != nil {
				return nil, gerror.Wrap(err, "清理验证资料回看游标失败")
			}
		}
		return result, nil
	}
	if !cache.Initialized() || verify["id"].Int64() <= 0 {
		return nil, gerror.New("验证资料回看已截断，缺少可持久化游标，等待缓存恢复")
	}
	nextCursor := collectMaterialAdvanceLookback(rows)
	nextCursor.Generation = generation
	if cursor.HighWaterID > nextCursor.HighWaterID {
		nextCursor.HighWaterID = cursor.HighWaterID
	}
	cursor = nextCursor
	if err := cache.Instance().Set(ctx, cursorKey, cursor, 10*time.Minute); err != nil {
		return nil, gerror.Wrap(err, "保存验证资料回看游标失败")
	}
	return nil, newCollectProcessRetryError(5*time.Second, "验证资料回看已推进游标，等待下一批")

}

// Rows must be newest first. A distinct verification group closes adjacency;
// it must never be skipped to reuse an older display for a second group.
func collectStoredDisplayFromRows(rows []gdb.Record, media map[int64][]collectMediaItem, verify gdb.Record) gdb.Record {
	row, _ := collectStoredDisplayDecision(rows, media, verify)
	return row
}

// decided distinguishes a real boundary from an all-skippable page.
func collectStoredDisplayDecision(rows []gdb.Record, media map[int64][]collectMediaItem, verify gdb.Record) (gdb.Record, bool) {
	for _, row := range rows {
		view := collectMaterialEventViews([]gdb.Record{row}, media)[0]
		if collectMaterialSkippableMessage(view) {
			continue
		}
		switch classifyProfileMessage(view.RawText, view.Media).Kind {
		case profileMessageKindDisplay:
			return row, true
		case profileMessageKindVerify:
			group := strings.TrimSpace(verify["source_grouped_id"].String())
			if group != "" && group == strings.TrimSpace(row["source_grouped_id"].String()) {
				continue
			}
			return nil, true
		default:
			return nil, true
		}
	}
	return nil, false
}

func (s *sSysPublish) findCollectDisplayEvent(rows []gdb.Record, views []collectMaterialMessageView, verifyIndex int) int {
	if verifyIndex < 0 || verifyIndex >= len(rows) {
		return -1
	}
	// The current source window is authoritative. A persisted parent may have
	// been selected by an earlier partial recovery window before a closer
	// display event became eligible for processing.
	for _, pair := range pairCollectMaterialMessages(views) {
		if pair.VerifyIndex == verifyIndex {
			return pair.DisplayIndex
		}
	}
	// A stored parent alone is never a fallback: it must pass the same
	// adjacency, group and time-window validation as an unclassified event.

	return -1
}

func truncateCollectDiagnosticText(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

// Reject a snapshot whose caption/identity/state changed before row locking.
func collectMaterialVerifySnapshotMatches(snapshot, current gdb.Record) bool {
	if snapshot.IsEmpty() || current.IsEmpty() {
		return false
	}
	for _, field := range []string{"id", "tenant_id", "account_id", "tg_account_id", "source_id", "source_chat_id", "source_message_id", "source_grouped_id", "raw_text", "media_json", "media_count", "material_role", "material_parent_event_id", "status"} {
		if snapshot[field].String() != current[field].String() {
			return false
		}
	}
	before, after := snapshot["updated_at"].GTime(), current["updated_at"].GTime()
	if before == nil || after == nil {
		if before != nil || after != nil {
			return false
		}
		// Preserve comparison for absent/unparseable legacy values as well.
		if snapshot["updated_at"].String() != current["updated_at"].String() {
			return false
		}
	} else if !before.Time.Equal(after.Time) {
		return false
	}
	return true
}

func collectMaterialOldParentRepairSafe(parent gdb.Record, hasDispatch, hasOrigin, hasOtherChild bool) bool {
	if parent.IsEmpty() || hasDispatch || hasOrigin || hasOtherChild || parent["processed_at"].GTime() != nil {
		return false
	}
	switch parent["status"].String() {
	case sysin.CollectEventStatusPending, sysin.CollectEventStatusGroupCollect, sysin.CollectEventStatusWaitingOrder:
		return true
	}
	return false
}

func (s *sSysPublish) bindCollectMaterialPair(ctx context.Context, display gdb.Record, verify gdb.Record) (publishconsts.CollectPairOutcome, error) {
	defer restoreCollectMaterialTransactionContext(ctx)()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cols := pdao.YoubanPublishCollectEvent.Columns()

	if display["id"].Int64() <= 0 || verify["id"].Int64() <= 0 {
		return publishconsts.CollectPairUnresolved, gerror.New("展示或验证事件ID无效")
	}
	now := gtime.Now()
	outcome := publishconsts.CollectPairUnresolved
	rejectReason := "展示资料已绑定另一组验证资料"
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {

		// Lock both parents and verify in stable ID order, including the snapshot's
		// old parent. Every repair/normal binding follows the same lock ordering.
		lockIDs := []int64{display[cols.Id].Int64(), verify[cols.Id].Int64()}
		oldParentID := verify["material_parent_event_id"].Int64()
		if oldParentID > 0 {
			lockIDs = append(lockIDs, oldParentID)
		}
		locked, txErr := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).
			WhereIn(cols.Id, lockIDs).OrderAsc(cols.Id).LockUpdate().All()
		if txErr != nil {
			return txErr
		}
		var parent, current, oldParent gdb.Record
		for _, row := range locked {
			if row[cols.Id].Int64() == display[cols.Id].Int64() {
				parent = row
			}
			if row[cols.Id].Int64() == verify[cols.Id].Int64() {
				current = row
			}
			if row[cols.Id].Int64() == oldParentID {
				oldParent = row
			}
		}
		if parent.IsEmpty() {
			return newCollectProcessRetryError(5*time.Second, "展示资料已变化，重新匹配")
		}
		if !collectMaterialVerifySnapshotMatches(verify, current) {
			return newCollectProcessRetryError(5*time.Second, "验证资料分类快照已变化，重新加载后匹配")
		}
		for _, field := range []string{cols.TenantId, cols.AccountId, cols.SourceId, cols.TgAccountId} {
			if parent[field].String() != current[field].String() {
				return gerror.New("展示和验证资料归属不一致，拒绝配对")
			}
		}
		if !collectMaterialSameSourceChat(parent, current) {
			return gerror.New("展示和验证资料频道不一致，拒绝配对")
		}
		refuse := func(result publishconsts.CollectPairOutcome, reason string) error {
			// Settle while this verification row is still locked. A late repair
			// must not overwrite a binding created after the transaction commits.
			if current["material_parent_event_id"].Int64() > 0 || current[cols.Status].String() == sysin.CollectEventStatusProcessed || current[cols.Status].String() == sysin.CollectEventStatusDispatched {
				return gerror.New("验证资料已有归属或已完成，拒绝自动丢弃，需显式修复")
			}
			status := sysin.CollectEventStatusIgnored
			if result == publishconsts.CollectPairNeedsRepair {
				status = sysin.CollectEventStatusFailed
				reason = "历史配对待人工修复：" + reason
			}
			if _, err := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).Where(cols.Id, current[cols.Id].Int64()).Data(g.Map{cols.Status: status, cols.ErrorMessage: reason, cols.ProcessedAt: now, cols.UpdatedAt: now}).Update(); err != nil {
				return err
			}
			if result == publishconsts.CollectPairRejected {
				m := pdao.YoubanPublishCollectEventMedia.Columns()
				if _, err := tx.Model(pdao.YoubanPublishCollectEventMedia.Table()).Ctx(ctx).Where(m.EventId, current[cols.Id].Int64()).WhereIn(m.CacheStatus, []string{collectMediaCachePending, collectMediaCacheDownloading}).Data(g.Map{m.CacheStatus: collectMediaCacheCanceled, m.ErrorMessage: reason, m.UpdatedAt: now}).Update(); err != nil {
					return err
				}
			}
			outcome = result
			rejectReason = reason
			return nil
		}
		// A persisted rejection must not be reopened by a raw-caption match.
		// Historical parents already attached elsewhere require explicit repair.
		if parent[cols.Status].String() == sysin.CollectEventStatusIgnored && parent["material_role"].String() != collectMaterialRoleDisplay {
			return refuse(publishconsts.CollectPairNeedsRepair, "前序消息历史分类不完整")
		}
		if parent[cols.Status].String() == sysin.CollectEventStatusIgnored {
			return refuse(publishconsts.CollectPairRejected, "父展示资料已忽略："+parent[cols.ErrorMessage].String())
		}
		if role := parent["material_role"].String(); role != "" && role != collectMaterialRolePending && role != collectMaterialRoleDisplay {
			return refuse(publishconsts.CollectPairNeedsRepair, "前序消息已归属其他资料")
		}
		repairing := oldParentID > 0 && oldParentID != parent[cols.Id].Int64()
		if repairing {
			if current["processed_at"].GTime() != nil || current["status"].String() == sysin.CollectEventStatusProcessed || current["status"].String() == sysin.CollectEventStatusDispatched {
				return gerror.New("验证事件已完成或已分发，需显式资料修复")
			}
			for _, field := range []string{cols.TenantId, cols.AccountId, cols.SourceId} {
				if oldParent[field].String() != parent[field].String() {
					return gerror.New("验证资料旧父关系跨来源，需显式资料修复")
				}
			}
			if !collectMaterialSameSourceChat(oldParent, parent) {
				return gerror.New("验证资料旧父关系跨频道，需显式资料修复")
			}
			d := pdao.YoubanPublishCollectDispatch.Columns()
			dispatched, readErr := tx.Model(pdao.YoubanPublishCollectDispatch.Table()).Ctx(ctx).
				Where(d.TenantId, parent[cols.TenantId].Int64()).Where(d.AccountId, parent[cols.AccountId].Int64()).Where(d.SourceId, parent[cols.SourceId].Int64()).
				WhereIn(d.EventId, []int64{oldParentID, verify[cols.Id].Int64()}).Fields(d.Id).Limit(1).One()
			if readErr != nil {
				return readErr
			}
			o := pdao.YoubanPublishProfileOrigin.Columns()
			origin, readErr := tx.Model(pdao.YoubanPublishProfileOrigin.Table()).Ctx(ctx).
				Where(o.TenantId, parent[cols.TenantId].Int64()).Where(o.AccountId, parent[cols.AccountId].Int64()).Where(o.SourceId, parent[cols.SourceId].Int64()).
				WhereIn(o.EventId, []int64{oldParentID, verify[cols.Id].Int64()}).Fields(o.Id).Limit(1).One()
			if readErr != nil {
				return readErr
			}
			sibling, readErr := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).
				Where(cols.TenantId, parent[cols.TenantId].Int64()).Where(cols.AccountId, parent[cols.AccountId].Int64()).
				Where(cols.SourceId, parent[cols.SourceId].Int64()).WhereIn(cols.SourceChatId, tgChannelCacheLookupIds(parent[cols.SourceChatId].String())).
				Where("material_role", collectMaterialRoleVerify).Where("material_parent_event_id", oldParentID).
				Where(cols.Id+" <> ?", verify[cols.Id].Int64()).Fields(cols.Id).Limit(1).One()
			if readErr != nil {
				return readErr
			}
			if !collectMaterialOldParentRepairSafe(oldParent, !dispatched.IsEmpty(), !origin.IsEmpty(), !sibling.IsEmpty()) {
				return gerror.New("验证资料旧父关系可能已入库、已分发或包含其他验证事件，需显式资料修复")
			}
		}

		// Reject a different verification group with an indexed existence probe;
		// all members of the same nonempty grouped_id remain one logical group.
		conflict := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).
			Where(cols.TenantId, display[cols.TenantId].Int64()).Where(cols.AccountId, display[cols.AccountId].Int64()).
			Where(cols.SourceId, display[cols.SourceId].Int64()).WhereIn(cols.SourceChatId, tgChannelCacheLookupIds(parent[cols.SourceChatId].String())).
			Where("material_parent_event_id", display[cols.Id].Int64()).Where("material_role", collectMaterialRoleVerify).
			Where(cols.Id+" <> ?", verify[cols.Id].Int64())
		group := strings.TrimSpace(verify[cols.SourceGroupedId].String())
		if group != "" {
			conflict = conflict.Where("COALESCE("+cols.SourceGroupedId+",'') <> ?", group)
		}
		child, txErr := conflict.Fields(cols.Id).Limit(1).One()
		if txErr != nil {
			return txErr
		}
		if !child.IsEmpty() {
			if repairing {
				return gerror.New("新展示已绑定另一验证组，需显式资料修复")
			}
			return refuse(publishconsts.CollectPairRejected, "展示资料已绑定另一组验证资料")
		}

		// Validate only the actual pairing interval under the parent lock.
		// Future source traffic must not invalidate this finite interval.
		canonical, canonicalErr := s.isCanonicalCollectMaterialPair(ctx, parent, verify)
		if canonicalErr != nil {
			return canonicalErr
		}
		if !canonical {
			if cache.Initialized() {
				if _, clearErr := cache.Instance().Remove(ctx, collectMaterialLookbackCursorKey(verify)); clearErr != nil {
					return clearErr
				}
			}
			return newCollectProcessRetryError(5*time.Second, "配对区间已变化，清理回看游标后从头重查")
		}

		if current["material_role"].String() == collectMaterialRoleVerify && current["material_parent_event_id"].Int64() == display[cols.Id].Int64() {
			outcome = publishconsts.CollectPairAlreadyBound
			return nil
		}

		displayStatus := strings.TrimSpace(parent["status"].String())
		displayGroupStatus := strings.TrimSpace(parent["material_group_status"].String())
		// Completed displays may retain ready after an explicit replay. Their
		// first late verification still needs the normal paired-parent wakeup.
		// Ownership, canonical adjacency and single-group checks above still apply.
		displayCanReattach := (displayStatus == sysin.CollectEventStatusProcessed || displayStatus == sysin.CollectEventStatusDispatched) &&
			(displayGroupStatus == "ready" || displayGroupStatus == "complete" || displayGroupStatus == "paired")
		displayQuery := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).
			Where("id", display["id"].Int64()).
			WhereIn("material_role", []string{collectMaterialRolePending, collectMaterialRoleDisplay})
		if !displayCanReattach {
			displayQuery = displayQuery.WhereIn("status", collectMaterialMutableStatuses())
		}
		result, txErr := displayQuery.
			Data(g.Map{"material_role": collectMaterialRoleDisplay, "material_group_status": "paired", "updated_at": now}).Update()
		if txErr != nil {
			return gerror.Wrap(txErr, "绑定资料展示组失败")
		}
		affected, txErr := result.RowsAffected()
		if txErr != nil {
			return gerror.Wrap(txErr, "确认资料展示组绑定结果失败")
		}
		if affected == 0 {
			return errCollectMaterialPairRace
		}
		verifyModel := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).Where(cols.Id, verify[cols.Id].Int64())
		if repairing {
			verifyModel = verifyModel.Where("material_parent_event_id", oldParentID).Where("material_role", collectMaterialRoleVerify)
		} else {
			verifyModel = verifyModel.Where("material_role = ? OR (status = ? AND error_message = ?)", collectMaterialRolePending, sysin.CollectEventStatusIgnored, collectMaterialVerifyUnmatchedMessage)
		}
		result, txErr = verifyModel.Data(g.Map{"status": sysin.CollectEventStatusGroupCollect, "material_role": collectMaterialRoleVerify, "material_parent_event_id": display["id"].Int64(), "material_group_status": "paired", "error_message": "", "processed_at": nil, "updated_at": now}).Update()
		if txErr != nil {
			return gerror.Wrap(txErr, "绑定资料验证组失败")
		}
		affected, txErr = result.RowsAffected()
		if txErr != nil {
			return gerror.Wrap(txErr, "确认资料验证组绑定结果失败")
		}
		if affected == 0 {
			return errCollectMaterialPairRace
		}
		if _, txErr = tx.Model(pdao.YoubanPublishCollectEventMedia.Table()).Ctx(ctx).
			Where("event_id", verify["id"].Int64()).Where("cache_status", collectMediaCacheCanceled).
			Data(g.Map{"cache_status": collectMediaCachePending, "error_message": "", "next_retry_at": nil, "updated_at": now}).Update(); txErr != nil {
			return gerror.Wrap(txErr, "恢复验证媒体下载失败")
		}
		if repairing {
			if _, updateErr := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).Where(cols.Id, oldParentID).
				Data(g.Map{"material_group_status": collectMaterialGroupWaitingVerify}).Update(); updateErr != nil {
				return updateErr
			}
		}
		outcome = publishconsts.CollectPairBound
		return nil
	})
	if errors.Is(err, errCollectMaterialPairRace) {
		g.Log().Infof(ctx, "采集验证组配对竞态已恢复重试 displayEventId:%d verifyEventId:%d", display["id"].Int64(), verify["id"].Int64())
		// 另一 Worker 已经完成配对；当前事件必须回到 Outbox 重试，不能被当成成功结束。
		return publishconsts.CollectPairUnresolved, newCollectProcessRetryError(2*time.Second, "资料组正在被其他Worker配对，稍后重试")
	}
	if err != nil {
		return publishconsts.CollectPairUnresolved, err
	}
	if outcome == publishconsts.CollectPairNeedsRepair || outcome == publishconsts.CollectPairRejected {
		stage := "collect.pair.rejected"
		if outcome == publishconsts.CollectPairNeedsRepair {
			stage = "collect.pair.needs_repair"
		}
		g.Log().Warning(ctx, "验证配对已记录明确结果", g.Map{"stage": stage, "displayEventId": display["id"].Int64(), "verifyEventId": verify["id"].Int64(), "reason": rejectReason})
		s.appendCollectEventLogForRecord(ctx, verify, "group", stage, rejectReason, "")
		return outcome, nil
	}
	if !outcome.IsBound() {
		return publishconsts.CollectPairUnresolved, newCollectProcessRetryError(2*time.Second, "配对未完成，重新加载后重试")
	}

	// The transaction has committed, so the pagination proof can now be dropped.
	// Cache cleanup failure cannot undo a successful durable, idempotent bind.
	if cache.Initialized() {
		if _, removeErr := cache.Instance().Remove(ctx, collectMaterialLookbackCursorKey(verify)); removeErr != nil {
			g.Log().Warning(ctx, "绑定完成后清理回看游标失败", g.Map{"eventId": verify["id"].Int64(), "error": removeErr.Error()})
		}
	}
	g.Log().Infof(ctx, "采集资料组绑定完成 chat:%s displayEventId:%d verifyEventId:%d", strings.TrimSpace(display["source_chat_id"].String()), display["id"].Int64(), verify["id"].Int64())
	s.appendCollectEventLogForRecord(ctx, display, "group", "paired", "展示资料与验证资料已在同一窗口配对", fmt.Sprintf(`{"verifyEventId":%d}`, verify["id"].Int64()))
	return outcome, nil
}

// isCanonicalCollectMaterialPair validates against every persisted event in
// the proposed range, including terminal and temporarily ignored rows. Normal
// processing windows intentionally filter those rows, but that filter must not
// allow an older display to steal a verification video from a closer display.
func (s *sSysPublish) isCanonicalCollectMaterialPair(ctx context.Context, display, verify gdb.Record) (bool, error) {
	defer restoreCollectMaterialTransactionContext(ctx)()
	displayMessageID := display["source_message_id"].Int64()
	verifyMessageID := verify["source_message_id"].Int64()
	if displayMessageID <= 0 || verifyMessageID <= displayMessageID {
		return false, nil
	}
	for _, field := range []string{"tenant_id", "account_id", "tg_account_id", "source_id"} {
		if display[field].String() != verify[field].String() {
			return false, nil
		}
	}

	if !collectMaterialSameSourceChat(display, verify) {
		return false, nil
	}

	displayAt := display["received_at"].GTime()
	if displayAt == nil {
		displayAt = display["created_at"].GTime()
	}
	verifyAt := verify["received_at"].GTime()
	if verifyAt == nil {
		verifyAt = verify["created_at"].GTime()
	}
	if displayAt == nil || verifyAt == nil || displayAt.Before(verifyAt.Add(-collectMaterialVerifyWindow())) || displayAt.After(verifyAt) {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cols := pdao.YoubanPublishCollectEvent.Columns()
	// This is a finite interval, not a source-history sweep. Each page has an
	// explicit keyset boundary and row budget; all pages share one deadline.
	var after collectMaterialLookbackCursor
	for {
		if err := ctx.Err(); err != nil {
			return false, newCollectProcessRetryError(5*time.Second, "配对区间校验超时，稍后重试")
		}
		model := pdao.YoubanPublishCollectEvent.Ctx(ctx).
			Where(cols.TenantId, verify[cols.TenantId].Int64()).Where(cols.AccountId, verify[cols.AccountId].Int64()).
			Where(cols.SourceId, verify[cols.SourceId].Int64()).Where(cols.TgAccountId, verify[cols.TgAccountId].Int64()).
			WhereIn(cols.SourceChatId, tgChannelCacheLookupIds(verify[cols.SourceChatId].String())).
			Where(cols.SourceMessageId+" >= ? AND "+cols.SourceMessageId+" < ?", displayMessageID, verifyMessageID)
		if after.MessageID > 0 {
			model = model.Where("("+cols.SourceMessageId+"<? OR ("+cols.SourceMessageId+"=? AND "+cols.Id+"<?))", after.MessageID, after.MessageID, after.EventID)
		}
		rows, err := model.OrderDesc(cols.SourceMessageId).OrderDesc(cols.Id).Limit(101).All()
		if err != nil {
			if ctx.Err() != nil {
				return false, newCollectProcessRetryError(5*time.Second, "配对区间校验超过5秒，稍后重试")
			}
			return false, gerror.Wrap(err, "校验展示验证区间失败")
		}
		truncated := len(rows) > 100
		if truncated {
			rows = rows[:100]
		}
		if len(rows) == 0 {
			return false, nil
		}
		ids := make([]int64, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row[cols.Id].Int64())
		}
		media, err := s.collectEventMediaItemsByEvent(ctx, ids)
		if err != nil {
			if ctx.Err() != nil {
				return false, newCollectProcessRetryError(5*time.Second, "配对区间媒体校验超过5秒，稍后重试")
			}
			return false, err
		}
		previous, decided := collectStoredDisplayDecision(rows, media, verify)
		if decided {
			return previous[cols.Id].Int64() == display[cols.Id].Int64(), nil
		}
		if !truncated {
			return false, nil
		}
		after = collectMaterialAdvanceLookback(rows)
	}

}

func (s *sSysPublish) markCollectMaterialRole(ctx context.Context, eventID int64, role string, parentID int64, status string) (bool, error) {
	result, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("id", eventID).
		Where("material_role", collectMaterialRolePending).
		WhereIn("status", collectMaterialMutableStatuses()).
		Data(g.Map{
			"material_role":            role,
			"material_parent_event_id": parentID,
			"material_group_status":    status,
			"updated_at":               gtime.Now(),
		}).Update()
	if err != nil {
		return false, gerror.Wrap(err, "更新采集资料组角色失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, gerror.Wrap(err, "确认采集资料组角色失败")
	}
	return affected > 0, nil
}

func (s *sSysPublish) markCollectMaterialWaitingVerify(ctx context.Context, eventID int64) error {
	result, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("id", eventID).
		WhereIn("material_role", []string{collectMaterialRolePending, collectMaterialRoleDisplay}).
		Where("material_group_status IS NULL OR material_group_status = '' OR material_group_status IN (?, ?)", collectMaterialGroupWaitingVerify, collectMaterialGroupCollecting).
		Data(g.Map{
			"material_group_status": collectMaterialGroupWaitingVerify,
			"updated_at":            gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新采集资料验证等待状态失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "确认采集资料验证等待状态失败")
	}
	if affected > 0 {
		s.appendCollectEventLog(ctx, eventID, "group", "waiting_verify", "展示资料进入固定20分钟验证窗口", "")
	}
	return nil
}

func (s *sSysPublish) pairedCollectVerifyEvents(ctx context.Context, displayEventID int64) (gdb.Result, error) {
	if displayEventID <= 0 {
		return nil, nil
	}
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("material_parent_event_id", displayEventID).
		Where("material_role", collectMaterialRoleVerify).
		OrderAsc("source_message_id").
		OrderAsc("id").
		All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料验证组列表失败")
	}
	return rows, nil
}

func (s *sSysPublish) collectVerifyEventsSettled(ctx context.Context, display gdb.Record, contextClosed bool) (bool, error) {
	if contextClosed {
		return true, nil
	}
	rows, err := s.pairedCollectVerifyEvents(ctx, display["id"].Int64())
	if err != nil {
		return false, err
	}
	// 已经写入 material_parent_event_id 就表示展示与验证资料完成配对。
	// 配对成功后立即进入资料落库，不再等待验证静默窗口；只有尚未配对时才等待窗口。
	return len(rows) > 0, nil
}

func collectMaterialVerifyDeadlineReached(displayAt, latestVerifyAt, now time.Time) bool {
	if latestVerifyAt.IsZero() {
		return false
	}
	deadline := latestVerifyAt.Add(collectMaterialVerifySettleDelay)
	if !displayAt.IsZero() {
		maximum := displayAt.Add(collectMaterialVerifyWindow() + collectMaterialVerifySettleDelay)
		if maximum.Before(deadline) {
			deadline = maximum
		}
	}
	return !now.Before(deadline)
}

func (s *sSysPublish) ensureCollectPairedVerifyReady(ctx context.Context, display gdb.Record) (bool, error) {
	verifies, err := s.pairedCollectVerifyEvents(ctx, display["id"].Int64())
	if err != nil || len(verifies) == 0 {
		return err == nil, err
	}
	ready := true
	for _, verify := range verifies {
		needsMediaCache, mediaErr := s.collectEventNeedsMediaCacheResult(ctx, verify)
		if mediaErr != nil {
			return false, gerror.Wrap(mediaErr, "检查配对验证资料媒体缓存状态失败")
		}
		if !needsMediaCache {
			continue
		}
		ready = false
		if err = s.markCollectEvent(ctx, verify["id"].Int64(), sysin.CollectEventStatusMediaPending, "验证媒体缓存中"); err != nil {
			return false, err
		}
		if err = s.enqueueCollectMediaCache(ctx, collectMediaQueuePayloadFromEvent(verify), 0); err != nil {
			return false, err
		}
	}
	return ready, nil
}

func collectMaterialWindowBatchSize(ctx context.Context) int {
	batchSize := g.Cfg().MustGet(ctx, "youbanPublish.collect.materialWindowBatchSize", 100).Int()
	if batchSize < 20 {
		return 20
	}
	if batchSize > 200 {
		return 200
	}
	return batchSize
}

func (s *sSysPublish) mergeCollectMaterialContent(ctx context.Context, display gdb.Record, content *collectContentResult) (*collectContentResult, error) {
	if content == nil {
		return content, nil
	}
	verifies, err := s.pairedCollectVerifyEvents(ctx, display["id"].Int64())
	if err != nil || len(verifies) == 0 {
		return content, err
	}
	verifyItems := make([]collectMediaItem, 0)
	for _, verify := range verifies {
		ready, readyErr := s.collectEventMediaReady(ctx, verify["id"].Int64())
		if readyErr != nil {
			return nil, readyErr
		}
		if !ready {
			return nil, newCollectProcessRetryError(30*time.Second, "验证资料媒体尚未就绪")
		}
		verifyRows, rowsErr := s.collectEventMediaRows(ctx, verify["id"].Int64())
		if rowsErr != nil {
			return nil, rowsErr
		}
		verifyItems = append(verifyItems, collectMediaRowsToItems(verifyRows, collectMaterialRoleVerify)...)
	}
	content.Media = mergeCollectMediaItems(
		collectMediaWithPurpose(content.Media, collectMaterialRoleDisplay),
		verifyItems,
	)
	content.MediaCount = len(content.Media)
	content.DedupeKey = collectHash(content.NormalizedText + ":" + collectMediaSignature(content.Media))
	return content, nil
}

func collectMaterialEventOlderThan(event gdb.Record, age time.Duration) bool {
	// History imports retain Telegram's original received_at. Grouping must wait
	// from local ingestion, otherwise an old album is classified before all of
	// its messages have been merged.
	eventAt := collectMaterialEventCreatedAt(event)
	if eventAt.IsZero() {
		return false
	}
	return time.Since(eventAt) >= age
}

func collectMaterialEventIngestOlderThan(event gdb.Record, age time.Duration) bool {
	eventAt := collectMaterialEventCreatedAt(event)
	if eventAt.IsZero() {
		return false
	}
	return time.Since(eventAt) >= age
}

func collectMaterialEventTime(event gdb.Record) string {
	if eventAt := collectMaterialEventAt(event); !eventAt.IsZero() {
		return eventAt.Format("2006-01-02 15:04:05 -0700")
	}
	return ""
}

func collectMaterialEventAt(event gdb.Record) time.Time {
	value := event["received_at"].GTime()
	if value == nil {
		value = event["created_at"].GTime()
	}
	if value == nil || value.IsZero() {
		return time.Time{}
	}
	// PostgreSQL `timestamp` has no timezone. The pgx driver may return it with
	// UTC attached, while the stored wall-clock value is local application time.
	// Preserve the database wall-clock fields before comparing with time.Now().
	wallClock := value.Time
	return time.Date(
		wallClock.Year(), wallClock.Month(), wallClock.Day(),
		wallClock.Hour(), wallClock.Minute(), wallClock.Second(),
		wallClock.Nanosecond(), time.Local,
	)
}

func collectMaterialEventCreatedAt(event gdb.Record) time.Time {
	value := event["created_at"].GTime()
	if value == nil {
		value = event["received_at"].GTime()
	}
	return collectMaterialDatabaseWallClock(value)
}

func collectMaterialDatabaseWallClock(value *gtime.Time) time.Time {
	if value == nil || value.IsZero() {
		return time.Time{}
	}
	wallClock := value.Time
	return time.Date(
		wallClock.Year(), wallClock.Month(), wallClock.Day(),
		wallClock.Hour(), wallClock.Minute(), wallClock.Second(),
		wallClock.Nanosecond(), time.Local,
	)
}

// Compare legacy and Bot API channel IDs without rewriting raw event identity.
func collectMaterialSameSourceChat(left, right gdb.Record) bool {
	chat := canonicalCollectSourceChatID(left["source_chat_id"].String())
	return chat != "" && chat == canonicalCollectSourceChatID(right["source_chat_id"].String())
}

// A lookup timeout proves neither absence nor a classification failure. Leave
// the cursor and event intact and yield to another task before retrying.
func collectMaterialLookbackRetryError(parent, lookup context.Context, err error) error {
	if err != nil && parent.Err() == nil && errors.Is(lookup.Err(), context.DeadlineExceeded) {
		return newCollectProcessRetryError(5*time.Second, "验证资料回看超时，保留进度稍后重试")
	}
	return err
}

func loadCollectMaterialLookbackPage(ctx context.Context, verify gdb.Record, verifyAt *gtime.Time, cursor collectMaterialLookbackCursor, limit int) (gdb.Result, error) {
	cols := pdao.YoubanPublishCollectEvent.Columns()
	fields := []string{cols.Id, cols.TenantId, cols.AccountId, cols.TgAccountId, cols.SourceId,
		cols.SourceChatId, cols.SourceMessageId, cols.SourceGroupedId, cols.RawText, cols.MediaCount,
		cols.ReceivedAt, cols.CreatedAt, cols.UpdatedAt, cols.ProcessedAt, cols.Status,
		"material_role", "material_parent_event_id", "material_group_status"}
	var rows gdb.Result
	for _, chatID := range tgChannelCacheLookupIds(verify[cols.SourceChatId].String()) {
		// PostgreSQL must restrict by the expression-time index before applying
		// message ordering. Without the materialization boundary it can walk a
		// source's entire message index to find a few rows in this time window.
		model := pdao.YoubanPublishCollectEvent.Ctx(ctx).
			Fields(fields).
			Where(cols.TenantId, verify[cols.TenantId].Int64()).
			Where(cols.AccountId, verify[cols.AccountId].Int64()).
			Where(cols.TgAccountId, verify[cols.TgAccountId].Int64()).
			Where(cols.SourceId, verify[cols.SourceId].Int64()).Where(cols.SourceChatId, chatID).
			Where("COALESCE("+cols.ReceivedAt+","+cols.CreatedAt+") BETWEEN ? AND ?", verifyAt.Add(-collectMaterialVerifyWindow()), verifyAt)
		applyPage := func(model *gdb.Model) *gdb.Model {
			model = model.Where(cols.SourceMessageId+" < ?", verify[cols.SourceMessageId].Int64())
			if cursor.MessageID > 0 && cursor.EventID > 0 && cursor.HighWaterID > 0 {
				model = model.Where("("+cols.Id+">? OR "+cols.SourceMessageId+"<? OR ("+cols.SourceMessageId+"=? AND "+cols.Id+"<?))", cursor.HighWaterID, cursor.MessageID, cursor.MessageID, cursor.EventID)
			}
			return model.OrderDesc(cols.SourceMessageId).OrderDesc(cols.Id).Limit(limit)
		}
		var batch gdb.Result
		var err error
		if strings.EqualFold(g.DB().GetConfig().Type, "pgsql") {
			// Keep all values parameterized. Only DAO-owned identifiers enter SQL.
			query, args := collectMaterialLookbackPostgresSQL(verify, verifyAt, chatID, cursor, limit, fields)
			batch, err = g.DB().GetAll(ctx, query, args...)
		} else {
			batch, err = applyPage(model).All()
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, batch...)
	}
	// Taking the top page from each disjoint alias preserves the global top
	// page, including message-ID ties and late predecessors above HighWaterID.
	sort.Slice(rows, func(i, j int) bool {
		if rows[i][cols.SourceMessageId].Int64() == rows[j][cols.SourceMessageId].Int64() {
			return rows[i][cols.Id].Int64() > rows[j][cols.Id].Int64()
		}
		return rows[i][cols.SourceMessageId].Int64() > rows[j][cols.SourceMessageId].Int64()
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func collectMaterialLookbackPostgresSQL(verify gdb.Record, verifyAt *gtime.Time, chatID string, cursor collectMaterialLookbackCursor, limit int, fields []string) (string, []any) {
	cols := pdao.YoubanPublishCollectEvent.Columns()
	query := "WITH collect_lookback_window AS MATERIALIZED (SELECT " + strings.Join(fields, ",") + " FROM " + pdao.YoubanPublishCollectEvent.Table() +
		" WHERE " + cols.TenantId + "=? AND " + cols.AccountId + "=? AND " + cols.TgAccountId + "=? AND " + cols.SourceId + "=? AND " + cols.SourceChatId + "=?" +
		" AND COALESCE(" + cols.ReceivedAt + "," + cols.CreatedAt + ") BETWEEN ? AND ?) SELECT * FROM collect_lookback_window WHERE " + cols.SourceMessageId + " < ?"
	args := []any{verify[cols.TenantId].Int64(), verify[cols.AccountId].Int64(), verify[cols.TgAccountId].Int64(), verify[cols.SourceId].Int64(), chatID, verifyAt.Add(-collectMaterialVerifyWindow()), verifyAt, verify[cols.SourceMessageId].Int64()}
	if cursor.MessageID > 0 && cursor.EventID > 0 && cursor.HighWaterID > 0 {
		query += " AND (" + cols.Id + ">? OR " + cols.SourceMessageId + "<? OR (" + cols.SourceMessageId + "=? AND " + cols.Id + "<?))"
		args = append(args, cursor.HighWaterID, cursor.MessageID, cursor.MessageID, cursor.EventID)
	}
	return query + " ORDER BY " + cols.SourceMessageId + " DESC," + cols.Id + " DESC LIMIT ?", append(args, limit)
}

// GoFrame reuses TXCore across nested contexts. Restore its caller context
// before returning so canceling a bounded lookup cannot poison a savepoint.
func restoreCollectMaterialTransactionContext(ctx context.Context) func() {
	tx := gdb.TXFromCtx(ctx, g.DB().GetGroup())
	if tx == nil {
		return func() {}
	}
	previous := tx.GetCtx()
	return func() { tx.Ctx(previous) }
}
