package sys

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	publishconsts "hotgo/addons/youban_publish/consts"
	pdao "hotgo/addons/youban_publish/internal/dao"
	pdo "hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	hglock "hotgo/internal/library/hgrds/lock"
)

const (
	collectHistoryRecoverAfter = 2 * time.Minute
	collectEventRecoverAfter   = 10 * time.Second
	// A single media item is capped at 15 minutes. Leave a small commit grace
	// period, then reclaim abandoned downloading rows instead of waiting 35m.
	collectMediaRecoverAfter = 20 * time.Minute
	// This guard only prevents concurrent recovery passes. It must not hide a
	// lost queue task for the lifetime of the old task's uniqueness window.
	collectMediaRecoveryDedupTTL        = 2 * time.Minute
	collectRecoveryRealtimeMaxPending   = 500
	collectRecoveryRealtimeMaxAge       = 10 * time.Second
	collectRecoveryBackpressureCacheKey = "youban_publish:collect:recovery:backpressure"
	collectRecoveryBackpressureLockKey  = collectRecoveryBackpressureCacheKey + ":lock"
	collectRecoveryBackpressureCacheTTL = 5 * time.Second
	collectRecoveryMediaCursorTTL       = 2 * time.Minute
	collectRecoveryMediaCursorKey       = "youban_publish:collect:recovery:media:cursor"
	// 恢复查询本身只处理超过 10 秒未推进的事件，因此实时优先窗口
	// 必须覆盖这段短暂积压，而不能再次使用 10 秒阈值导致全部命中历史。
	collectRecoveryRealtimeWindow     = 10 * time.Minute
	collectRecoverySourceBatchDefault = 8
	collectRecoverySourceBatchMaximum = 64
)

func reserveCollectMediaRecovery(ctx context.Context, eventID int64) bool {
	if eventID <= 0 || !cache.Initialized() || g.Cfg().MustGet(ctx, "cache.adapter").String() != "redis" {
		return true
	}
	key := fmt.Sprintf("%s%d", publishconsts.CollectMediaRecoveryKeyPrefix, eventID)
	ok, err := cache.Instance().SetIfNotExist(ctx, key, 1, collectMediaRecoveryDedupTTL)
	if err != nil {
		g.Log().Warningf(ctx, "采集媒体恢复去重缓存不可用，放行任务 eventId:%d err:%+v", eventID, err)
		return true
	}
	return ok
}

func releaseCollectMediaRecovery(ctx context.Context, eventID int64) {
	if eventID <= 0 || !cache.Initialized() || g.Cfg().MustGet(ctx, "cache.adapter").String() != "redis" {
		return
	}
	key := fmt.Sprintf("%s%d", publishconsts.CollectMediaRecoveryKeyPrefix, eventID)
	if _, err := cache.Instance().Remove(ctx, key); err != nil {
		g.Log().Warningf(ctx, "释放采集媒体恢复去重缓存失败 eventId:%d err:%+v", eventID, err)
	}
}

func collectRecoveryInterval(ctx context.Context) time.Duration {
	seconds := g.Cfg().MustGet(ctx, "youbanPublish.collect.recoveryIntervalSeconds", 60).Int()
	if seconds < 10 {
		seconds = 10
	}
	if seconds > 3600 {
		seconds = 3600
	}
	return time.Duration(seconds) * time.Second
}

func (s *sSysPublish) runCollectRecovery(ctx context.Context) {
	time.Sleep(12 * time.Second)
	s.runCollectRecoveryCycle(ctx)
	ticker := time.NewTicker(collectRecoveryInterval(ctx))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runCollectRecoveryCycle(ctx)
		}
	}
}

func (s *sSysPublish) runCollectRecoveryCycle(ctx context.Context) {
	lockCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	distributedLock := hglock.NewConfig(2*time.Minute, 100*time.Millisecond).
		Mutex("youban_publish:collect:recovery")
	if err := distributedLock.TryLock(lockCtx); err != nil {
		if !errors.Is(err, hglock.ErrLockFailed) && ctx.Err() == nil {
			g.Log().Warningf(ctx, "获取采集恢复任务锁失败：%+v", err)
		}
		return
	}
	defer func() { _ = distributedLock.Unlock(context.Background()) }()
	s.recoverCollectOnce(ctx)
}

func activeCollectProcessScopes(ctx context.Context) []publishAccountScope {
	if !cache.Initialized() {
		return nil
	}
	maxScopes := collectRecoverySourceBatchSize(ctx)
	// Advance the shared source cursor; Redis emptiness never proves PG work is drained.
	members, err := cache.SetRotate(ctx, collectProcessActiveSourcesKey, maxScopes)
	if err != nil {
		g.Log().Warning(ctx, "读取采集恢复来源轮转失败", g.Map{"stage": "collect.recovery.scope_failed", "error": err.Error()})
		return nil
	}
	scopes := make([]publishAccountScope, 0, min(len(members), maxScopes))
	for _, member := range members {
		if len(scopes) >= maxScopes {
			break
		}
		parts := strings.Split(member, ":")
		if len(parts) != 3 {
			continue
		}
		tenantID, errTenant := strconv.ParseInt(parts[0], 10, 64)
		accountID, errAccount := strconv.ParseInt(parts[1], 10, 64)
		sourceID, errSource := strconv.ParseInt(parts[2], 10, 64)
		if errTenant != nil || errAccount != nil || errSource != nil || tenantID <= 0 || accountID < 0 || sourceID <= 0 {
			// Bad legacy members must not remain in the compatibility set.
			_ = cache.SetRemove(ctx, collectProcessActiveSourcesKey, member)
			continue
		}
		scopes = append(scopes, publishAccountScope{TenantID: tenantID, AccountID: accountID, SourceID: sourceID})
	}

	return scopes
}

func collectRecoverySourceBatchSize(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "youbanPublish.collect.recoverySourceBatchSize", collectRecoverySourceBatchDefault).Int()
	if value < 1 {
		return collectRecoverySourceBatchDefault
	}
	if value > collectRecoverySourceBatchMaximum {
		return collectRecoverySourceBatchMaximum
	}
	return value
}

func collectSourceRecoveryConcurrency(ctx context.Context) int {
	value := g.Cfg().MustGet(ctx, "youbanPublish.collect.sourceRecoveryConcurrency", 16).Int()
	if value < 1 {
		return 1
	}
	if value > 128 {
		return 128
	}
	return value
}

func (s *sSysPublish) recoverCollectOnce(ctx context.Context) {
	// Recovery is lane-driven. Without Redis' active tenant/account/source
	// scopes there is no safe bounded PostgreSQL predicate to use; never fall
	// back to a global recovery scan while the scope index is unavailable.
	scopes := activeCollectProcessScopes(ctx)
	if len(scopes) == 0 {
		g.Log().Warning(ctx, "采集恢复已跳过：Redis 来源 scope 不可用，延迟到下一轮", g.Map{
			"stage":  "collect.recovery.skipped",
			"reason": "active_source_scope_unavailable",
		})
		return
	}
	g.Log().Debug(ctx, "采集恢复使用 Redis 来源 scope", g.Map{"stage": "collect.recovery.scoped", "scopeCount": len(scopes)})
	s.recoverCollectMediaSourceScopes(ctx, scopes)
	// Automatic recovery is source-scoped only. Legacy migration handlers are
	// deliberately not reachable here, even when an old ENV flag remains set.
	if err := s.recoverCollectEvents(ctx, 100, scopes...); err != nil {
		g.Log().Warningf(ctx, "恢复采集事件处理失败：%+v", err)
	}
}

// recoverMediaReadyWithoutDispatch 恢复媒体已完成、事件仍未终态且没有
// collect_dispatch 的历史事件。该入口只重新投递采集处理 Outbox，不直接
// 创建资料或发布任务，因此不会绕过规则、审核和去重流程。
func (s *sSysPublish) recoverMediaReadyWithoutDispatch(ctx context.Context, limit int, scopes ...publishAccountScope) error {
	if len(scopes) != 1 || scopes[0].TenantID <= 0 || scopes[0].AccountID < 0 || scopes[0].SourceID <= 0 {
		return gerror.New("恢复媒体就绪但缺少分发记录的采集事件必须指定唯一且有效的租户、账户和来源范围")
	}
	scope := scopes[0]
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if heavyRecoveryDisabled("YOUBAN_DISABLE_MEDIA_READY_RECOVERY") {
		return nil
	}
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	now := gtime.Now()
	e := pdao.YoubanPublishCollectEvent.Columns()
	m := pdao.YoubanPublishCollectEventMedia.Table()
	d := pdao.YoubanPublishCollectDispatch.Table()
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).As("e").
		Fields("e.id,e.tenant_id,e.account_id,e.source_id,e.source_chat_id,e.updated_at").
		Where("e."+e.TenantId, scope.TenantID).
		Where("e."+e.AccountId, scope.AccountID).
		Where("e."+e.SourceId, scope.SourceID).
		Where("e.status", sysin.CollectEventStatusMediaReady).
		WhereNull("e.processed_at").
		WhereLTE("e.updated_at", now.Add(-collectEventRecoverAfter)).
		Where(enabledCollectSourceExistsSQL("e")).
		Where("EXISTS (SELECT 1 FROM "+m+" m WHERE m.event_id=e.id)").
		Where("NOT EXISTS (SELECT 1 FROM "+m+" m WHERE m.event_id=e.id AND m.cache_status<>?)", collectMediaCacheReady).
		Where("NOT EXISTS (SELECT 1 FROM " + d + " d WHERE d.event_id=e.id AND d.tenant_id=e.tenant_id AND d.account_id=e.account_id)").
		OrderAsc("e.account_id").OrderAsc("e.source_id").OrderAsc("e.updated_at").OrderAsc("e.id").
		Limit(limit).All()
	if err != nil {
		return gerror.Wrap(err, "读取媒体就绪但缺少分发记录的采集事件失败")
	}
	requeued := 0
	for _, row := range rows {
		eventID := row[e.Id].Int64()
		if eventID <= 0 {
			continue
		}
		queued, queueErr := s.enqueueCollectProcessDeferred(ctx, collectProcessQueuePayload{
			Recovery: true, EventId: eventID, TenantId: row[e.TenantId].Int64(),
			AccountId: row[e.AccountId].Int64(), SourceId: row[e.SourceId].Int64(),
			SourceChatId: row[e.SourceChatId].String(),
		}, 0)
		if queueErr != nil {
			g.Log().Warningf(ctx, "恢复媒体就绪采集事件入队失败 eventId:%d accountId:%d sourceId:%d err:%+v", eventID, row[e.AccountId].Int64(), row[e.SourceId].Int64(), queueErr)
			continue
		}
		if queued {
			requeued++
		}
	}
	if requeued > 0 {
		g.Log().Infof(ctx, "恢复媒体就绪但缺少分发记录的采集事件 count:%d", requeued)
	}
	return nil
}

// Rebuild only recent reviewing dispatches whose review row disappeared. Old
// orphaned rows are excluded because they may reflect an intentional action in
// an older application version.
func (s *sSysPublish) recoverRecentOrphanCollectReviews(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 20
	}
	now := gtime.Now()
	reviewTable := pdao.YoubanPublishCollectReview.Table()
	ruleTable := pdao.YoubanPublishCollectRule.Table()
	rows, err := g.DB().Model(publishCollectDispatchTable+" d").Safe().Ctx(ctx).
		Fields("d.id,d.event_id,d.review_id,d.tenant_id,d.account_id,d.rule_id,d.updated_at").
		Where("d.status", sysin.CollectDispatchStatusReviewing).
		Where("d.profile_id", 0).
		WhereGTE("d.updated_at", now.Add(-24*time.Hour)).
		WhereLTE("d.updated_at", now.Add(-5*time.Minute)).
		Where("(COALESCE(d.review_id,0)=0 OR NOT EXISTS (SELECT 1 FROM "+reviewTable+" r WHERE r.id=d.review_id))").
		Where("EXISTS (SELECT 1 FROM "+publishCollectEventTable+" e WHERE e.id=d.event_id AND e.status=?)", sysin.CollectEventStatusDispatched).
		Where("EXISTS (SELECT 1 FROM " + ruleTable + " r WHERE r.id=d.rule_id AND r.status=1 AND r.review_enabled=1 AND r.deleted_at IS NULL)").
		// Drain the newest orphan wave first; older records remain eligible in
		// later bounded passes after recent user-facing reviews are restored.
		OrderDesc("d.updated_at").Limit(limit).All()
	if err != nil {
		return gerror.Wrap(err, "读取近期孤立采集审核分发失败")
	}
	for _, row := range rows {
		dispatchID, eventID := row["id"].Int64(), row["event_id"].Int64()
		if dispatchID <= 0 || eventID <= 0 {
			continue
		}
		eventLock := hglock.NewConfig(2*time.Minute, 50*time.Millisecond).
			Mutex(fmt.Sprintf("youban_publish:collect:event:%d", eventID))
		if err = eventLock.TryLock(ctx); err != nil {
			if !errors.Is(err, hglock.ErrLockFailed) && ctx.Err() == nil {
				g.Log().Warningf(ctx, "获取孤立审核事件锁失败 eventId:%d err:%+v", eventID, err)
			}
			continue
		}
		recoverErr := s.recoverOneOrphanCollectReview(ctx, row)
		_ = eventLock.Unlock(context.Background())
		if recoverErr != nil {
			g.Log().Warningf(ctx, "恢复孤立采集审核失败 dispatchId:%d eventId:%d err:%+v", dispatchID, eventID, recoverErr)
		}
	}
	return nil
}

func (s *sSysPublish) recoverOneOrphanCollectReview(ctx context.Context, dispatch gdb.Record) error {
	dispatchID, eventID := dispatch["id"].Int64(), dispatch["event_id"].Int64()
	current, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("id", dispatchID).Where("status", sysin.CollectDispatchStatusReviewing).One()
	if err != nil || current.IsEmpty() {
		return err
	}
	existing, err := pdao.YoubanPublishCollectReview.Ctx(ctx).Where("dispatch_id", dispatchID).OrderDesc("id").One()
	if err != nil {
		return gerror.Wrap(err, "检查孤立审核的已有记录失败")
	}
	if !existing.IsEmpty() {
		if existing["status"].String() != sysin.CollectReviewStatusPending {
			return nil
		}
		_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where("id", dispatchID).
			Where("status", sysin.CollectDispatchStatusReviewing).
			Data(g.Map{"review_id": existing["id"].Int64(), "updated_at": gtime.Now()}).Update()
		return gerror.Wrap(err, "重新关联孤立审核记录失败")
	}
	event, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", eventID).
		Where("status", sysin.CollectEventStatusDispatched).One()
	if err != nil || event.IsEmpty() {
		return err
	}
	rule, err := pdao.YoubanPublishCollectRule.Ctx(ctx).
		Where("id", current["rule_id"].Int64()).Where("status", 1).
		Where("review_enabled", 1).WhereNull("deleted_at").One()
	if err != nil || rule.IsEmpty() {
		return err
	}
	// Clear only the dangling pointer. The row and event are fenced by status;
	// createCollectReview then uses the normal media preparation and review path.
	reviewTable := pdao.YoubanPublishCollectReview.Table()
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		locked, lockErr := tx.Model(publishCollectDispatchTable).Where("id", dispatchID).
			Where("status", sysin.CollectDispatchStatusReviewing).
			Where("profile_id", 0).LockUpdate().One()
		if lockErr != nil || locked.IsEmpty() {
			return lockErr
		}
		stillMissing, checkErr := tx.Model(reviewTable).Where("dispatch_id", dispatchID).Count()
		if checkErr != nil || stillMissing > 0 {
			return checkErr
		}
		_, updateErr := tx.Model(publishCollectDispatchTable).Where("id", dispatchID).
			Where("status", sysin.CollectDispatchStatusReviewing).
			Where("profile_id", 0).
			Where("review_id", locked["review_id"].Int64()).
			Data(g.Map{"review_id": 0, "updated_at": gtime.Now()}).Update()
		return updateErr
	})
	if err != nil {
		return gerror.Wrap(err, "清理孤立审核引用失败")
	}
	content, err := s.collectContentSnapshot(ctx, event)
	if err != nil {
		return gerror.Wrap(err, "恢复孤立审核资料快照失败")
	}
	text := strings.TrimSpace(event["raw_text"].String())
	if err = s.createCollectReview(ctx, event, content, rule, dispatchID, text); err != nil {
		return err
	}
	g.Log().Warningf(ctx, "已通过正常采集审核流程恢复孤立审核 dispatchId:%d eventId:%d", dispatchID, eventID)
	return nil
}

// A fairness retry can leave the event in media_pending after all media rows
// have already become ready. Such rows are intentionally excluded from the
// pending-media query, so normalize the event and put it back on the process
// queue instead of leaving an orphaned state forever.
func (s *sSysPublish) recoverReadyMediaPendingEvents(ctx context.Context, limit int, scopes ...publishAccountScope) error {
	if len(scopes) != 1 || scopes[0].TenantID <= 0 || scopes[0].AccountID < 0 || scopes[0].SourceID <= 0 {
		return gerror.New("恢复媒体已就绪的待处理采集事件必须指定唯一且有效的租户、账户和来源范围")
	}
	scope := scopes[0]
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	e := pdao.YoubanPublishCollectEvent.Columns()
	m := pdao.YoubanPublishCollectEventMedia.Table()
	realtimeCutoff := gtime.Now().Add(-collectRecoveryRealtimeMaxAge)
	buildModel := func() *gdb.Model {
		return pdao.YoubanPublishCollectEvent.Ctx(ctx).As("e").
			Fields("e.id,e.tenant_id,e.account_id,e.source_id,e.source_chat_id").
			Where("e."+e.TenantId, scope.TenantID).
			Where("e."+e.AccountId, scope.AccountID).
			Where("e."+e.SourceId, scope.SourceID).
			Where("e."+e.Status, sysin.CollectEventStatusMediaPending).
			Where(enabledCollectSourceExistsSQL("e")).
			Where("EXISTS (SELECT 1 FROM "+m+" m WHERE m.event_id=e."+e.Id+")").
			Where("NOT EXISTS (SELECT 1 FROM "+m+" m WHERE m.event_id=e."+e.Id+" AND m.cache_status<>?)", collectMediaCacheReady)
	}
	queryRows := func(recent bool, queryLimit int) ([]gdb.Record, error) {
		if queryLimit <= 0 {
			return nil, nil
		}
		model := buildModel()
		if recent {
			model = model.WhereGTE("e."+e.UpdatedAt, realtimeCutoff)
		} else {
			model = model.WhereLT("e."+e.UpdatedAt, realtimeCutoff)
		}
		return model.OrderAsc("e." + e.UpdatedAt).OrderAsc("e.id").Limit(queryLimit).All()
	}

	rows, err := queryRows(true, limit)
	if err != nil {
		return gerror.Wrap(err, "读取实时优先媒体已就绪采集事件失败")
	}
	if len(rows) < limit {
		historyRows, historyErr := queryRows(false, limit-len(rows))
		if historyErr != nil {
			return gerror.Wrap(historyErr, "读取历史媒体已就绪采集事件失败")
		}
		rows = append(rows, historyRows...)
	}
	for _, row := range rows {
		id := row[e.Id].Int64()
		if id <= 0 {
			continue
		}
		if _, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).Where(e.Id, id).
			Where(e.TenantId, scope.TenantID).Where(e.AccountId, scope.AccountID).Where(e.SourceId, scope.SourceID).
			Where(e.Status, sysin.CollectEventStatusMediaPending).
			Data(g.Map{e.Status: sysin.CollectEventStatusMediaReady, e.ErrorMessage: "", e.ProcessedAt: nil, e.UpdatedAt: gtime.Now()}).Update(); err != nil {
			return gerror.Wrap(err, "更新媒体已就绪采集事件状态失败")
		}
		if err = s.enqueueCollectProcess(ctx, collectProcessQueuePayload{
			EventId: id, TenantId: row[e.TenantId].Int64(), AccountId: row[e.AccountId].Int64(),
			SourceId: row[e.SourceId].Int64(), SourceChatId: row[e.SourceChatId].String(),
		}, 0); err != nil {
			return gerror.Wrap(err, "重新投递媒体已就绪采集事件失败")
		}
	}
	if len(rows) > 0 {
		g.Log().Infof(ctx, "恢复媒体已就绪但事件仍待处理的采集任务 count:%d", len(rows))
	}
	return nil
}

// Recovery must yield to realtime when the shared database/worker budget is
// under pressure. This is a bounded gate, not a queue deletion or reset.
func (s *sSysPublish) collectRecoveryBackpressure(ctx context.Context) (bool, error) {
	if collectProcessOutboxMode(ctx) != "postgres_outbox" {
		// Redis-first realtime work has no PostgreSQL Outbox rows to sample.
		// Asynq owns retry/backlog pressure in this mode; querying the legacy
		// table here would reintroduce the exact hot read this mode removes.
		return false, nil
	}
	if cached, cacheErr := cache.Instance().Get(ctx, collectRecoveryBackpressureCacheKey); cacheErr == nil && !cached.IsNil() {
		var value collectRecoveryBackpressureValue
		if cached.Scan(&value) == nil {
			return value.blocked(), nil
		}
	}
	if locked, lockErr := cache.Instance().SetIfNotExist(ctx, collectRecoveryBackpressureLockKey, 1, 2*time.Second); lockErr == nil && !locked {
		// Another recovery pass is refreshing the bounded sample. Yield this
		// pass instead of querying the outbox again.
		return true, nil
	} else if lockErr != nil {
		g.Log().Warningf(ctx, "采集恢复背压缓存不可用，回退数据库有界检查 err:%+v", lockErr)
	}
	defer func() { _, _ = cache.Instance().Remove(ctx, collectRecoveryBackpressureLockKey) }()

	// Do not use COUNT/MIN here. Backpressure only needs to know whether the
	// cap is exceeded and the oldest sampled row. LIMIT cap+1 bounds the work
	// even when the historical outbox is very large.
	rows, err := g.DB().Model(collectProcessOutboxTable).Ctx(ctx).
		Fields("updated_at").
		Where("history", false).
		WhereIn("status", []string{"pending", "processing"}).
		OrderAsc("updated_at").
		Limit(collectRecoveryRealtimeMaxPending + 1).
		All()
	if err != nil {
		return true, err
	}
	value := collectRecoveryBackpressureValue{Pending: int64(len(rows))}
	if len(rows) > 0 && rows[0]["updated_at"].GTime() != nil {
		value.OldestUnixNano = rows[0]["updated_at"].GTime().UnixNano()
	}
	_ = cache.Instance().Set(ctx, collectRecoveryBackpressureCacheKey, value, collectRecoveryBackpressureCacheTTL)
	blocked := value.blocked()
	if blocked {
		age := time.Duration(0)
		if value.OldestUnixNano > 0 {
			age = time.Since(time.Unix(0, value.OldestUnixNano)).Round(time.Millisecond)
		}
		g.Log().Infof(ctx, "实时采集背压，跳过本轮历史恢复 sampled:%d oldestAge:%s", value.Pending, age)
	}
	return blocked, nil
}

type collectRecoveryBackpressureValue struct {
	Pending        int64 `json:"pending"`
	OldestUnixNano int64 `json:"oldestUnixNano"`
}

func (v collectRecoveryBackpressureValue) blocked() bool {
	if v.Pending >= collectRecoveryRealtimeMaxPending {
		return true
	}
	return v.OldestUnixNano > 0 && time.Since(time.Unix(0, v.OldestUnixNano)) >= collectRecoveryRealtimeMaxAge
}

// recoverInvalidReadyCollectMedia repairs legacy rows that treated a Telegram
// file_id as a durable cache. They must re-enter the normal download queue;
// otherwise the parent event can remain in media_pending indefinitely.
func (s *sSysPublish) recoverInvalidReadyCollectMedia(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 1000
	}
	mediaCols := pdao.YoubanPublishCollectEventMedia.Columns()
	rows, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).As("m").
		Fields("m."+mediaCols.Id).
		Where("m."+mediaCols.CacheStatus, collectMediaCacheReady).
		Where("COALESCE(TRIM(m." + mediaCols.StoragePath + "), '') = ''").
		Where("COALESCE(TRIM(m." + mediaCols.FileUrl + "), '') = ''").
		Where("NOT (COALESCE(TRIM(m." + mediaCols.BackupChatId + "), '') <> '' AND m." + mediaCols.BackupMessageId + " > 0)").
		Where(enabledCollectSourceExistsSQL("m")).
		OrderAsc("m." + mediaCols.Id).
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取无持久化文件的采集媒体失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := row[mediaCols.Id].Int64(); id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	now := gtime.Now()
	result, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		WhereIn(mediaCols.Id, ids).
		Where(mediaCols.CacheStatus, collectMediaCacheReady).
		Where("COALESCE(TRIM(" + mediaCols.StoragePath + "), '') = ''").
		Where("COALESCE(TRIM(" + mediaCols.FileUrl + "), '') = ''").
		Where("NOT (COALESCE(TRIM(" + mediaCols.BackupChatId + "), '') <> '' AND " + mediaCols.BackupMessageId + " > 0)").
		Data(g.Map{
			mediaCols.CacheStatus:   collectMediaCachePending,
			collectMediaNextRetryAt: nil,
			mediaCols.ErrorMessage:  "历史媒体缺少持久化文件，已自动恢复下载",
			mediaCols.UpdatedAt:     now,
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "重置无持久化文件的采集媒体失败")
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		g.Log().Warningf(ctx, "已恢复无持久化文件的采集媒体 count:%d", affected)
	}
	return nil
}

// Submission/duplicate notices are terminal and must never enter media or
// publish processing. Clean up old pending rows as well as new ones so a
// backlog cannot consume the collection queues.
func (s *sSysPublish) ignoreCollectNoticeEvents(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 500
	}
	e := pdao.YoubanPublishCollectEvent.Columns()
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Fields(e.Id, e.RawText).
		WhereIn(e.Status, []string{sysin.CollectEventStatusPending, sysin.CollectEventStatusGroupCollect, sysin.CollectEventStatusWaitingOrder, sysin.CollectEventStatusPrechecked, sysin.CollectEventStatusMediaPending}).
		WhereNull(e.ProcessedAt).OrderAsc(e.UpdatedAt).Limit(limit).All()
	if err != nil {
		return gerror.Wrap(err, "读取采集通知事件失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if profileMessageIgnoredNotice(row[e.RawText].String()) {
			ids = append(ids, row[e.Id].Int64())
		}
	}
	if len(ids) == 0 {
		return nil
	}
	now := gtime.Now()
	_, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).WhereIn(e.Id, ids).Data(g.Map{
		e.Status: sysin.CollectEventStatusIgnored, e.ErrorMessage: "删除关键字忽略:投稿/重复通知", e.ProcessedAt: now, e.UpdatedAt: now,
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "标记采集通知事件失败")
	}
	_, _ = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).WhereIn("event_id", ids).Data(g.Map{"cache_status": collectMediaCacheCanceled, "error_message": "删除关键字忽略:投稿/重复通知", "updated_at": now}).Update()
	g.Log().Infof(ctx, "已忽略采集通知事件 count:%d", len(ids))
	return nil
}

// recoverBotCollectMedia is deliberately event-scoped. Older source-scoped
// recovery can keep a busy BOT source alive without ever putting a particular
// media group back on the media queue. Re-enqueueing each stale BOT event
// lets the normal BOT downloader resume the group independently.
func (s *sSysPublish) recoverBotCollectMedia(ctx context.Context, limit int, scopes ...publishAccountScope) error {
	if len(scopes) != 1 || scopes[0].TenantID <= 0 || scopes[0].AccountID < 0 || scopes[0].SourceID <= 0 {
		return gerror.New("BOT媒体恢复必须指定租户、账户和来源")
	}
	scope := scopes[0]
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 50)
	e := pdao.YoubanPublishCollectEvent.Columns()
	deadline := gtime.Now().Add(-collectEventRecoverAfter)
	model := pdao.YoubanPublishCollectEvent.Ctx(ctx).As("e").
		Fields("e.*").
		Where("e.tenant_id", scope.TenantID).Where("e.account_id", scope.AccountID).Where("e.source_id", scope.SourceID).
		Where("e."+e.SourceType, sysin.CollectSourceTypeBot).
		WhereIn("e."+e.Status, []string{sysin.CollectEventStatusMediaPending, sysin.CollectEventStatusMediaReady, sysin.CollectEventStatusGroupCollect, sysin.CollectEventStatusPending, sysin.CollectEventStatusFailed}).
		WhereNull("e."+e.ProcessedAt).
		WhereLTE("e."+e.UpdatedAt, deadline).
		Where(enabledCollectSourceExistsSQL("e"))
	// Keep ordering and pagination on the outer event query. GoFrame may attach
	// later Order/Limit calls to an EXISTS expression when its predicate is
	// built inline, producing invalid PostgreSQL SQL (ORDER inside EXISTS).
	model = model.Where("e."+e.Id+" IN (SELECT m.event_id FROM "+pdao.YoubanPublishCollectEventMedia.Table()+" m WHERE m.tenant_id=? AND m.account_id=? AND m.source_id=? AND m.cache_status=? AND (m.next_retry_at IS NULL OR m.next_retry_at<=?))", scope.TenantID, scope.AccountID, scope.SourceID, collectMediaCachePending, gtime.Now())
	rows, err := model.OrderAsc("e." + e.UpdatedAt).Limit(limit).All()
	if err != nil {
		return gerror.Wrap(err, "读取待恢复BOT媒体事件失败")
	}
	requeued := 0
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return gerror.Wrap(err, "BOT媒体恢复预算已耗尽，保留下一轮处理")
		}
		payload := collectMediaQueuePayloadFromEvent(row)
		payload.Reason = "bot_recovery"
		if payload.EventId <= 0 || payload.TenantId <= 0 || payload.SourceId <= 0 {
			continue
		}
		if !reserveCollectMediaRecovery(ctx, payload.EventId) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return gerror.Wrap(err, "BOT媒体恢复预算已耗尽，保留下一轮处理")
		}
		queued, enqueueErr := s.enqueueCollectMediaCacheDeferred(ctx, payload, 0)
		if enqueueErr != nil {
			releaseCollectMediaRecovery(ctx, payload.EventId)
			// Enqueue failure leaves the event eligible in PostgreSQL. Stop this
			// source pass instead of sending the rest of its batch into an
			// unhealthy Redis; the existing recovery cycle will retry it.
			return gerror.Wrapf(enqueueErr, "BOT媒体事件重新入队失败，暂停本轮来源恢复 eventId:%d", payload.EventId)
		}
		if queued {
			status := row[e.Status].String()
			if _, updateErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).
				Where(e.Id, payload.EventId).
				Where(e.Status, status).
				WhereLTE(e.UpdatedAt, deadline).
				Data(pdo.YoubanPublishCollectEvent{Status: status}).Update(); updateErr != nil {
				g.Log().Warningf(ctx, "更新BOT媒体事件恢复心跳失败 eventId:%d err:%+v", payload.EventId, updateErr)
				continue
			}
			requeued++
			g.Log().Infof(ctx, "BOT媒体事件已按事件重新入队 eventId:%d sourceId:%d", payload.EventId, payload.SourceId)
		}
	}
	if requeued > 0 {
		g.Log().Infof(ctx, "BOT媒体事件恢复完成 count:%d", requeued)
	}
	return nil
}

// RepairBotCollectMedia exposes the one-shot historical repair for the CLI.
func RepairBotCollectMedia(ctx context.Context, limit int) error {
	scopes := activeCollectProcessScopes(ctx)
	if len(scopes) == 0 {
		return gerror.New("没有可恢复的BOT来源 scope，禁止全局扫描")
	}
	service := NewSysPublish()
	budget := max(1, min(50, limit/max(1, len(scopes))))
	for _, scope := range scopes {
		if err := service.recoverBotCollectMedia(ctx, budget, scope); err != nil {
			return err
		}
	}
	return nil
}

// IgnoreCollectNoticeEvents exposes the one-shot notification cleanup for the CLI.
func IgnoreCollectNoticeEvents(ctx context.Context, limit int) error {
	return NewSysPublish().ignoreCollectNoticeEvents(ctx, limit)
}

func (s *sSysPublish) ignoreStaleCollectEventsForDisabledSources(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	deadline := gtime.Now().Add(-collectEventRecoverAfter)
	statuses := []string{
		sysin.CollectEventStatusPending,
		sysin.CollectEventStatusGroupCollect,
		sysin.CollectEventStatusWaitingOrder,
		sysin.CollectEventStatusPrechecked,
		sysin.CollectEventStatusMediaPending,
		sysin.CollectEventStatusMediaReady,
		sysin.CollectEventStatusFailed,
	}
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).As("e").
		Fields("e.id").
		WhereIn("e.status", statuses).
		WhereLTE("e.updated_at", deadline).
		Where("NOT " + enabledCollectSourceExistsSQL("e")).
		OrderAsc("e.updated_at").
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取已关闭采集源事件失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if id := row["id"].Int64(); id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	now := gtime.Now()
	if _, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).WhereIn("id", ids).Data(g.Map{
		"status":        sysin.CollectEventStatusIgnored,
		"error_message": "采集源已关闭，事件不再处理",
		"processed_at":  now,
		"updated_at":    now,
	}).Update(); err != nil {
		return gerror.Wrap(err, "标记已关闭采集源事件失败")
	}
	return nil
}

func (s *sSysPublish) recoverStaleCollectMediaRows(ctx context.Context, limit int, scopes ...publishAccountScope) error {
	// A missing or ambiguous lane must never fall back to a global media scan.
	if len(scopes) != 1 || scopes[0].TenantID <= 0 || scopes[0].AccountID < 0 || scopes[0].SourceID <= 0 {
		return gerror.New("采集媒体恢复需要唯一有效的 tenant/account/source scope，已跳过全局扫描")
	}
	scope := scopes[0]
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	deadline := gtime.Now().Add(-collectMediaRecoverAfter)
	mediaCols := pdao.YoubanPublishCollectEventMedia.Columns()
	rows, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).As("m").
		Fields("m."+mediaCols.Id, "m."+mediaCols.EventId).
		Where("m."+mediaCols.TenantId, scope.TenantID).
		Where("m."+mediaCols.AccountId, scope.AccountID).
		Where("m."+mediaCols.SourceId, scope.SourceID).
		Where("m."+mediaCols.CacheStatus, collectMediaCacheDownloading).
		WhereLTE("m."+mediaCols.UpdatedAt, deadline).
		Where(enabledCollectSourceExistsSQL("m")).
		OrderAsc("m." + mediaCols.UpdatedAt).
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取超时采集媒体失败")
	}
	if len(rows) == 0 {
		return nil
	}

	mediaIds := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.IsEmpty() || row[mediaCols.Id].Int64() <= 0 {
			continue
		}
		mediaIds = append(mediaIds, row[mediaCols.Id].Int64())
	}
	if len(mediaIds) == 0 {
		return nil
	}
	_, err = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		WhereIn(mediaCols.Id, mediaIds).
		Where(mediaCols.TenantId, scope.TenantID).
		Where(mediaCols.AccountId, scope.AccountID).
		Where(mediaCols.SourceId, scope.SourceID).
		Where(mediaCols.CacheStatus, collectMediaCacheDownloading).
		WhereLTE(mediaCols.UpdatedAt, deadline).
		Data(g.Map{
			mediaCols.CacheStatus:   collectMediaCachePending,
			collectMediaNextRetryAt: nil,
			mediaCols.ErrorMessage:  "媒体缓存任务超时，已自动恢复重试",
			mediaCols.UpdatedAt:     gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "重置超时采集媒体失败")
	}
	g.Log().Warningf(ctx, "已恢复超时采集媒体 count:%d", len(mediaIds))
	return nil
}

// RepairStaleCollectMedia performs the same bounded recovery as the runtime
// watchdog, but runs synchronously for an operator after a deployment or
// queue outage. It first normalizes timed-out downloads and then immediately
// requeues the affected event-scoped media tasks.
func RepairStaleCollectMedia(ctx context.Context, limit int) error {
	service := NewSysPublish()
	scopes := activeCollectProcessScopes(ctx)
	if len(scopes) == 0 {
		return gerror.New("没有可恢复的采集来源 scope，禁止全局媒体扫描")
	}
	budget := max(1, min(50, limit/max(1, len(scopes))))
	for _, scope := range scopes {
		if err := service.recoverStaleCollectMediaRows(ctx, budget, scope); err != nil {
			return err
		}
		if err := service.recoverPendingCollectMedia(ctx, budget, scope); err != nil {
			return err
		}
		if err := service.recoverBotCollectMedia(ctx, budget, scope); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) recoverPendingCollectMedia(ctx context.Context, limit int, scopes ...publishAccountScope) error {
	// A missing or ambiguous lane must never fall back to a global media scan.
	if len(scopes) != 1 || scopes[0].TenantID <= 0 || scopes[0].AccountID < 0 || scopes[0].SourceID <= 0 {
		return gerror.New("采集媒体恢复需要唯一有效的 tenant/account/source scope，已跳过全局扫描")
	}
	scope := scopes[0]
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	now := gtime.Now()
	eventCols := pdao.YoubanPublishCollectEvent.Columns()
	mediaTable := pdao.YoubanPublishCollectEventMedia.Table()
	candidateLimit := limit
	cursorKey := fmt.Sprintf("%s:%d:%d:%d", collectRecoveryMediaCursorKey, scope.TenantID, scope.AccountID, scope.SourceID)

	var cursorUpdatedAt *gtime.Time
	var cursorID int64
	if cached, cacheErr := cache.Instance().Get(ctx, cursorKey); cacheErr == nil && !cached.IsNil() {
		var cursor struct {
			UpdatedAtUnixNano int64 `json:"updated_at_unix_nano"`
			ID                int64 `json:"id"`
		}
		if cached.Scan(&cursor) == nil && cursor.UpdatedAtUnixNano > 0 && cursor.ID > 0 {
			cursorUpdatedAt = gtime.New(time.Unix(0, cursor.UpdatedAtUnixNano))
			cursorID = cursor.ID
		}
	}

	model := pdao.YoubanPublishCollectEvent.Ctx(ctx).As("e").
		Where("e."+eventCols.TenantId, scope.TenantID).
		Where("e."+eventCols.AccountId, scope.AccountID).
		Where("e."+eventCols.SourceId, scope.SourceID).
		Where("e."+eventCols.Status, sysin.CollectEventStatusMediaPending).
		WhereNot("e."+eventCols.SourceType, sysin.CollectSourceTypeBot).
		Where(enabledCollectSourceExistsSQL("e")).
		Where("EXISTS (SELECT 1 FROM "+mediaTable+" m WHERE m.event_id=e."+eventCols.Id+" AND m.tenant_id=? AND m.account_id=? AND m.source_id=? AND m.cache_status=? AND (m.next_retry_at IS NULL OR m.next_retry_at<=?))", scope.TenantID, scope.AccountID, scope.SourceID, collectMediaCachePending, now)
	if cursorUpdatedAt != nil {
		model = model.Where("(e."+eventCols.UpdatedAt+">? OR (e."+eventCols.UpdatedAt+"=? AND e."+eventCols.Id+">?))", cursorUpdatedAt, cursorUpdatedAt, cursorID)
	}
	rows, err := model.OrderAsc("e." + eventCols.UpdatedAt).OrderAsc("e." + eventCols.Id).Limit(candidateLimit).All()
	if err != nil {
		return gerror.Wrap(err, "读取待恢复采集媒体事件失败")
	}
	if len(rows) == 0 && cursorUpdatedAt != nil {
		_, _ = cache.Instance().Remove(ctx, cursorKey)
		return nil
	}
	if len(rows) == 0 {
		return nil
	}

	recovered := 0
	for _, row := range fairCollectMediaRecoveryRows(rows, limit) {
		if row.IsEmpty() {
			continue
		}
		payload := collectMediaQueuePayloadFromEvent(row)
		payload.Reason = "recovery"
		if payload.EventId <= 0 || payload.TenantId <= 0 || payload.AccountId < 0 || payload.SourceId <= 0 {
			continue
		}
		if !reserveCollectMediaRecovery(ctx, payload.EventId) {
			observeCollectMediaScheduler(ctx, "recovery_duplicate_skipped", payload.TenantId, payload.TgAccountId)
			continue
		}
		enqueued, enqueueErr := s.enqueueCollectMediaCacheDeferred(ctx, payload, 0)
		if enqueueErr != nil {
			releaseCollectMediaRecovery(ctx, payload.EventId)
			g.Log().Warningf(ctx, "恢复待处理采集媒体任务投递失败 eventId:%d sourceId:%d err:%+v", payload.EventId, payload.SourceId, enqueueErr)
			continue
		}
		if !enqueued {
			observeCollectMediaScheduler(ctx, "recovery_enqueue_skipped", payload.TenantId, payload.TgAccountId)
			continue
		}
		observeCollectMediaScheduler(ctx, "recovery_reenqueued", payload.TenantId, payload.TgAccountId)
		if _, updateErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).
			Where(eventCols.Id, payload.EventId).
			Where(eventCols.TenantId, scope.TenantID).
			Where(eventCols.AccountId, scope.AccountID).
			Where(eventCols.SourceId, scope.SourceID).
			Where(eventCols.Status, sysin.CollectEventStatusMediaPending).
			Data(g.Map{eventCols.ProcessedAt: nil, eventCols.UpdatedAt: now}).Update(); updateErr != nil {
			g.Log().Warningf(ctx, "更新待处理采集媒体恢复心跳失败 eventId:%d err:%+v", payload.EventId, updateErr)
			continue
		}
		recovered++
	}

	last := rows[len(rows)-1]
	lastUpdatedAt := last[eventCols.UpdatedAt].GTime()
	lastID := last[eventCols.Id].Int64()
	if lastUpdatedAt != nil && lastID > 0 {
		_ = cache.Instance().Set(ctx, cursorKey, struct {
			UpdatedAtUnixNano int64 `json:"updated_at_unix_nano"`
			ID                int64 `json:"id"`
		}{lastUpdatedAt.UnixNano(), lastID}, collectRecoveryMediaCursorTTL)
	}
	if recovered > 0 {
		g.Log().Infof(ctx, "已重新投递待处理采集媒体任务 count:%d", recovered)
	}
	return nil
}

const legacyCollectMissingVerifyMessage = "BOT资料在5分钟窗口内未匹配到验证视频"

// recoverLegacyMissingVerifyCollectEvents reopens only records terminated by
// the retired mandatory-verification policy. The normal process pipeline owns
// media caching, deduplication and dispatch after this bounded repair.
func (s *sSysPublish) recoverLegacyMissingVerifyCollectEvents(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	e := pdao.YoubanPublishCollectEvent.Columns()
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields(e.Id, e.TenantId, e.AccountId, e.SourceId, e.SourceChatId).
		Where(e.SourceType, sysin.CollectSourceTypeBot).
		Where(e.Status, sysin.CollectEventStatusIgnored).
		Where(e.ErrorMessage, legacyCollectMissingVerifyMessage).
		OrderAsc(e.Id).Limit(limit).All()
	if err != nil {
		return gerror.Wrap(err, "读取历史缺少验证视频事件失败")
	}
	for _, row := range rows {
		eventID := row[e.Id].Int64()
		if eventID <= 0 {
			continue
		}
		changed := false
		err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			result, txErr := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Ctx(ctx).
				Where(e.Id, eventID).Where(e.Status, sysin.CollectEventStatusIgnored).
				Where(e.ErrorMessage, legacyCollectMissingVerifyMessage).
				Data(g.Map{e.Status: sysin.CollectEventStatusGroupCollect, "material_role": collectMaterialRoleDisplay,
					"material_parent_event_id": 0, "material_group_status": "complete",
					e.ErrorMessage: "", e.ProcessedAt: nil, e.UpdatedAt: gtime.Now()}).Update()
			if txErr != nil {
				return txErr
			}
			affected, _ := result.RowsAffected()
			if affected == 0 {
				return nil
			}
			changed = true
			_, txErr = tx.Model(pdao.YoubanPublishCollectEventMedia.Table()).Ctx(ctx).
				Where("event_id", eventID).Where("cache_status", collectMediaCacheCanceled).
				Data(g.Map{"cache_status": collectMediaCachePending, "error_message": "", "next_retry_at": nil, "updated_at": gtime.Now()}).Update()
			return txErr
		})
		if err != nil {
			return gerror.Wrapf(err, "恢复历史缺少验证视频事件失败 eventId:%d", eventID)
		}
		if !changed {
			continue
		}
		if err = s.enqueueCollectProcess(ctx, collectProcessQueuePayload{
			EventId: eventID, TenantId: row[e.TenantId].Int64(), AccountId: row[e.AccountId].Int64(),
			SourceId: row[e.SourceId].Int64(), SourceChatId: row[e.SourceChatId].String(),
		}, 0); err != nil {
			return gerror.Wrapf(err, "重新投递历史缺少验证视频事件失败 eventId:%d", eventID)
		}
	}
	return nil
}

func fairCollectMediaRecoveryRows(rows gdb.Result, limit int) gdb.Result {
	if limit <= 0 || len(rows) <= 1 {
		return rows
	}
	accountOrder := make([]string, 0)
	byAccount := make(map[string]gdb.Result)
	for _, row := range rows {
		if row.IsEmpty() {
			continue
		}
		key := collectMediaAccountKey(row["tenant_id"].Int64(), row["tg_account_id"].Int64())
		if _, exists := byAccount[key]; !exists {
			accountOrder = append(accountOrder, key)
		}
		byAccount[key] = append(byAccount[key], row)
	}
	result := make(gdb.Result, 0, minInt(limit, len(rows)))
	for len(result) < limit {
		progressed := false
		for _, key := range accountOrder {
			accountRows := byAccount[key]
			if len(accountRows) == 0 {
				continue
			}
			result = append(result, accountRows[0])
			byAccount[key] = accountRows[1:]
			progressed = true
			if len(result) >= limit {
				break
			}
		}
		if !progressed {
			break
		}
	}
	return result
}

func collectMediaRecoveryBatchSize(ctx context.Context) int {
	batchSize := g.Cfg().MustGet(ctx, "youbanPublish.collect.mediaRecoveryBatchSize", 200).Int()
	if batchSize < 20 {
		return 20
	}
	if batchSize > 500 {
		return 500
	}
	return batchSize
}

func (s *sSysPublish) recoverCollectHistoryTasks(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 20
	}
	now := gtime.Now()
	stale := now.Add(-collectHistoryRecoverAfter)
	rows, err := pdao.YoubanPublishCollectHistoryTask.Ctx(ctx).As("h").
		Where(enabledCollectSourceExistsSQL("h")).
		Where(
			"(status=? OR (status=? AND updated_at<=?) OR (status=? AND (next_run_at IS NULL OR next_run_at<=?)) OR (status=? AND (error_message LIKE ? OR error_message LIKE ? OR error_message LIKE ? OR error_message LIKE ?)))",
			sysin.CollectHistoryTaskStatusPending,
			sysin.CollectHistoryTaskStatusRunning,
			stale,
			sysin.CollectHistoryTaskStatusPaused,
			now,
			sysin.CollectHistoryTaskStatusFailed,
			"%TG账号连接正在使用，拒绝创建第二个客户端%",
			"%client closed%context canceled%",
			"%DC is closed%",
			"%engine forcibly closed%",
		).
		OrderAsc("h.updated_at").
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取待恢复历史采集任务失败")
	}
	for _, row := range rows {
		if row.IsEmpty() || row["id"].Int64() <= 0 {
			continue
		}
		if err = s.recoverCollectHistoryTask(ctx, row); err != nil {
			g.Log().Warningf(ctx, "恢复历史采集任务失败 task:%d err:%+v", row["id"].Int64(), err)
		}
	}
	return nil
}

func (s *sSysPublish) recoverCollectHistoryTask(ctx context.Context, row gdb.Record) error {
	taskId := row["id"].Int64()
	if taskId <= 0 {
		return nil
	}
	if strings.Contains(row["error_message"].String(), "TG账号连接正在使用，拒绝创建第二个客户端") {
		g.Log().Errorf(ctx, "历史采集任务曾因重复创建TG客户端失败，正在恢复 task:%d accountId:%d tgAccountId:%d err:%s", taskId, row["account_id"].Int64(), row["tg_account_id"].Int64(), row["error_message"].String())
	}
	_, err := pdao.YoubanPublishCollectHistoryTask.Ctx(ctx).Where("id", taskId).Data(g.Map{
		"status":        sysin.CollectHistoryTaskStatusPending,
		"error_message": "",
		"updated_at":    gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "重置历史采集任务失败")
	}
	s.appendCollectHistoryLog(ctx, taskId, row["tenant_id"].Int64(), row["account_id"].Int64(), "info", "requeue", "历史采集任务恢复并重新投递", nil)
	return s.enqueueCollectHistoryTask(ctx, taskId, 0)
}

func (s *sSysPublish) recoverCollectEvents(ctx context.Context, limit int, suppliedScopes ...publishAccountScope) error {
	parentCtx := ctx
	ctx, cancelPass := context.WithTimeout(ctx, 30*time.Second)
	defer cancelPass()
	// Query cancellation must not cancel processing of rows already collected.
	// Reserve at least half of the pass for claims and queue submissions.
	queryCtx, cancelQueries := context.WithTimeout(ctx, 15*time.Second)
	defer cancelQueries()
	if limit <= 0 {
		limit = 100
	}
	deadline := gtime.Now().Add(-collectEventRecoverAfter)
	realtimeCutoff := gtime.Now().Add(-collectRecoveryRealtimeWindow)
	activeStatuses := []string{sysin.CollectEventStatusPending, sysin.CollectEventStatusGroupCollect, sysin.CollectEventStatusWaitingOrder, sysin.CollectEventStatusPrechecked, sysin.CollectEventStatusMediaPending, sysin.CollectEventStatusMediaReady}
	scopes := suppliedScopes
	if len(scopes) == 0 {
		scopes = activeCollectProcessScopes(queryCtx)
	}
	if len(scopes) == 0 {
		// Redis is the source of active recovery lanes. Never fall back to a
		// global event scan when the candidate index is unavailable.
		return nil
	}
	buildModel := func(queryCtx context.Context, scope publishAccountScope) *gdb.Model {
		return pdao.YoubanPublishCollectEvent.Ctx(queryCtx).As("e").
			Fields("e.id,e.tenant_id,e.account_id,e.source_id,e.source_chat_id,e.status,e.error_message").
			WhereIn("e.status", activeStatuses).
			WhereLTE("e.updated_at", deadline).
			Where("e.tenant_id", scope.TenantID).
			Where("e.account_id", scope.AccountID).
			Where("e.source_id", scope.SourceID).
			Where(enabledCollectSourceExistsSQL("e"))
	}
	queryRows := func(queryCtx context.Context, scope publishAccountScope, recent bool, queryLimit int) ([]gdb.Record, error) {
		if queryLimit <= 0 {
			return nil, nil
		}
		model := buildModel(queryCtx, scope)
		if recent {
			model = model.WhereGTE("e.updated_at", realtimeCutoff)
		} else {
			model = model.WhereLT("e.updated_at", realtimeCutoff)
		}
		return model.OrderAsc("e.updated_at").OrderAsc("e.id").Limit(queryLimit).All()
	}

	rows := make([]gdb.Record, 0, limit)
	perScope := max(1, (limit+len(scopes)-1)/len(scopes))
	for scopeIndex, scope := range scopes {
		if len(rows) >= limit || queryCtx.Err() != nil {
			break
		}
		// Reserve time for every remaining lane; one slow source cannot
		// consume the page budget before later sources get a query attempt.
		queryDeadline, _ := queryCtx.Deadline()
		sourceBudget, branchBudget := collectEventRecoveryTimeBudget(time.Until(queryDeadline), len(scopes)-scopeIndex)
		sourceCtx, cancelSource := context.WithTimeout(queryCtx, sourceBudget)
		budget := min(perScope, limit-len(rows))
		// Reserve slots for history and failed events even under continuous
		// realtime traffic. Unused slots are borrowed by the realtime query.
		recentBudget, historyBudget, failedBudget := collectEventRecoveryBudgets(budget)
		load := func(recent bool, count int) int {
			if count <= 0 || sourceCtx.Err() != nil {
				return 0
			}
			branchCtx, cancelBranch := context.WithTimeout(sourceCtx, branchBudget)
			defer cancelBranch()
			batch, err := queryRows(branchCtx, scope, recent, count)
			if err != nil {
				g.Log().Warningf(ctx, "读取来源采集恢复事件失败 tenant:%d account:%d source:%d recent:%t err:%+v", scope.TenantID, scope.AccountID, scope.SourceID, recent, err)
				return 0
			}
			rows = append(rows, batch...)
			return len(batch)
		}
		historyCount := load(false, historyBudget)
		failedCount := 0
		if failedBudget > 0 && sourceCtx.Err() == nil {
			branchCtx, cancelBranch := context.WithTimeout(sourceCtx, branchBudget)
			failedRows, err := recoverableFailedCollectEventModel(branchCtx, deadline).
				Where("e.tenant_id", scope.TenantID).Where("e.account_id", scope.AccountID).
				Where("e.source_id", scope.SourceID).
				OrderAsc("e.updated_at").OrderAsc("e.id").Limit(failedBudget).All()
			cancelBranch()
			if err != nil {
				g.Log().Warningf(ctx, "读取来源失败采集事件失败 source:%d err:%+v", scope.SourceID, err)
			} else {
				rows = append(rows, failedRows...)
				failedCount = len(failedRows)
			}
		}
		if sourceCtx.Err() == nil {
			load(true, recentBudget+historyBudget-historyCount+failedBudget-failedCount)
		}
		cancelSource()
	}
	cancelQueries()
	for _, row := range rows {
		if ctx.Err() != nil {
			break
		}
		func() {
			ctx, cancelEvent := context.WithTimeout(ctx, 5*time.Second)
			defer cancelEvent()
			if row.IsEmpty() || row["id"].Int64() <= 0 || !shouldRecoverCollectEvent(row) {
				return
			}
			if row["status"].String() == sysin.CollectEventStatusMediaPending {
				canResume, mediaErr := s.collectEventMediaCanResume(ctx, row["id"].Int64())
				if mediaErr != nil {
					g.Log().Warningf(ctx, "检查采集事件媒体恢复条件失败 event:%d err:%+v", row["id"].Int64(), mediaErr)
					return
				}
				if !canResume {
					return
				}
			}
			claimData := g.Map{"updated_at": gtime.Now()}
			if row["status"].String() == sysin.CollectEventStatusFailed {
				claimData["status"] = sysin.CollectEventStatusPending
				claimData["error_message"] = ""
				claimData["processed_at"] = nil
			}
			claimResult, claimErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).
				Where("id", row["id"].Int64()).
				WhereLTE("updated_at", deadline).
				Data(claimData).Update()
			if claimErr != nil {
				g.Log().Warningf(ctx, "抢占待恢复采集事件失败 event:%d err:%+v", row["id"].Int64(), claimErr)
				return
			}
			claimed, claimErr := claimResult.RowsAffected()
			if claimErr != nil {
				g.Log().Warningf(ctx, "确认待恢复采集事件抢占结果失败 event:%d err:%+v", row["id"].Int64(), claimErr)
				return
			}
			if claimed == 0 {
				return
			}
			queued, processErr := s.enqueueCollectProcessDeferred(ctx, collectProcessQueuePayload{
				Recovery:     true,
				EventId:      row["id"].Int64(),
				SourceId:     row["source_id"].Int64(),
				SourceChatId: row["source_chat_id"].String(),
				TenantId:     row["tenant_id"].Int64(),
				AccountId:    row["account_id"].Int64(),
			}, 0)
			if processErr != nil {
				g.Log().Warningf(ctx, "恢复采集事件投递失败 source:%d event:%d err:%+v", row["source_id"].Int64(), row["id"].Int64(), processErr)
				return
			}
			if !queued {
				g.Log().Warningf(ctx, "恢复采集事件任务未实际入队 source:%d event:%d", row["source_id"].Int64(), row["id"].Int64())
				return
			}
		}()
	}
	return parentCtx.Err()
}

// Reserve a small scheduling margin as well as independent branch deadlines.
func collectEventRecoveryTimeBudget(remaining time.Duration, lanes int) (source, branch time.Duration) {
	if remaining <= 0 || lanes <= 0 {
		return 0, 0
	}
	source = min(5*time.Second, remaining/time.Duration(lanes+1))
	return source, min(time.Second, source/3)
}

// collectEventRecoveryBudgets reserves bounded slots for all recovery classes.
func collectEventRecoveryBudgets(budget int) (recent, history, failed int) {
	if budget <= 0 {
		return 0, 0, 0
	}
	if budget < 3 {
		return budget, 0, 0
	}
	history = max(1, budget/5)
	failed = max(1, budget/5)
	return budget - history - failed, history, failed
}

func recoverableFailedCollectEventModel(ctx context.Context, deadline *gtime.Time) *gdb.Model {
	model := pdao.YoubanPublishCollectEvent.Ctx(ctx).As("e").
		Fields("e.id,e.tenant_id,e.account_id,e.source_id,e.source_chat_id,e.status,e.error_message").
		Where("e.status", sysin.CollectEventStatusFailed).
		WhereLTE("e.updated_at", deadline).
		Where(enabledCollectSourceExistsSQL("e"))
	for _, message := range collectEventTerminalFailureMessages {
		model = model.Where("LOWER(COALESCE(e.error_message, '')) NOT LIKE ?", "%"+strings.ToLower(message)+"%")
	}
	patterns := make([]string, 0, len(collectEventRecoverableFailureMessages))
	args := make([]interface{}, 0, len(collectEventRecoverableFailureMessages))
	for _, message := range collectEventRecoverableFailureMessages {
		patterns = append(patterns, "LOWER(COALESCE(e.error_message, '')) LIKE ?")
		args = append(args, "%"+strings.ToLower(message)+"%")
	}
	return model.Where("("+strings.Join(patterns, " OR ")+")", args...)
}

// RecoverCollectEvents runs one bounded recovery pass through the normal
// collection queue. It is intended for operational repair tooling.
func (s *sSysPublish) RecoverCollectEvents(ctx context.Context, limit int) error {
	return s.recoverCollectEvents(ctx, limit)
}

func enabledCollectSourceExistsSQL(ownerAlias string) string {
	// Bot transport records may retain account_id=0 in events, media and
	// history. Only the same tenant/source with a real Bot owner may alias 0;
	// positive accounts must still match exactly.
	return "EXISTS (SELECT 1 FROM " + pdao.YoubanPublishCollectSource.Table() + " s WHERE s.id=" + ownerAlias + ".source_id AND s.tenant_id=" + ownerAlias + ".tenant_id AND (s.account_id=" + ownerAlias + ".account_id OR (" + ownerAlias + ".account_id=0 AND s.account_id>0 AND s.source_type='bot')) AND s.collect_enabled=1 AND s.status=1 AND s.deleted_at IS NULL)"
}

func (s *sSysPublish) collectEventMediaCanResume(ctx context.Context, eventId int64) (bool, error) {
	if eventId <= 0 {
		return false, nil
	}
	cols := pdao.YoubanPublishCollectEventMedia.Columns()
	rows, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Fields(cols.CacheStatus, cols.NextRetryAt, cols.StoragePath, cols.FileUrl, cols.BackupChatId, cols.BackupMessageId).
		Where(cols.EventId, eventId).
		All()
	if err != nil {
		return false, gerror.Wrapf(err, "读取采集事件媒体失败 event:%d", eventId)
	}
	return collectEventMediaRecordsCanResume(rows, gtime.Now()), nil
}

func collectEventMediaRecordsCanResume(rows gdb.Result, now *gtime.Time) bool {
	if len(rows) == 0 || now == nil {
		return false
	}
	cols := pdao.YoubanPublishCollectEventMedia.Columns()
	for _, row := range rows {
		status := strings.TrimSpace(row[cols.CacheStatus].String())
		if status == collectMediaCacheReady &&
			(strings.TrimSpace(row[cols.StoragePath].String()) != "" || strings.TrimSpace(row[cols.FileUrl].String()) != "" ||
				(strings.TrimSpace(row[cols.BackupChatId].String()) != "" && row[cols.BackupMessageId].Int64() > 0)) {
			continue
		}
		if status == collectMediaCacheReady {
			continue
		}
		if status == collectMediaCachePending {
			nextRetryAt := row[cols.NextRetryAt].GTime()
			if nextRetryAt == nil || !nextRetryAt.After(now) {
				continue
			}
		}
		return false
	}
	// A crash can happen after the last media row becomes ready but before the
	// parent event is re-enqueued. That state is immediately safe to resume.
	return true
}

func shouldRecoverCollectEvent(row gdb.Record) bool {
	status := strings.TrimSpace(row["status"].String())
	if status == sysin.CollectEventStatusPending || status == sysin.CollectEventStatusGroupCollect || status == sysin.CollectEventStatusWaitingOrder || status == sysin.CollectEventStatusPrechecked || status == sysin.CollectEventStatusMediaPending || status == sysin.CollectEventStatusMediaReady {
		return true
	}
	if status != sysin.CollectEventStatusFailed {
		return false
	}
	message := strings.ToLower(row["error_message"].String())
	for _, terminal := range collectEventTerminalFailureMessages {
		if strings.Contains(message, strings.ToLower(terminal)) {
			return false
		}
	}
	for _, recoverable := range collectEventRecoverableFailureMessages {
		if strings.Contains(message, strings.ToLower(recoverable)) {
			return true
		}
	}
	return false
}

var (
	collectEventTerminalFailureMessages = []string{
		"未找到原消息",
		"无法刷新文件引用",
		"缺少原消息引用",
		"缺少下载元数据",
	}
	collectEventRecoverableFailureMessages = []string{
		"app_id",
		"资料指纹索引正在初始化",
		"账号采集媒体",
		"媒体",
		"上一条采集资料",
		"验证视频暂未匹配到前序资料",
	}
)

func (s *sSysPublish) reconcileCollectDispatchesFromSentJobs(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	dispatchTable := pdao.YoubanPublishCollectDispatch.Table()
	rows, err := g.DB().Model(dispatchTable+" d").Ctx(ctx).
		Fields("d.id,d.event_id").
		Where("d.profile_id > 0").
		Where("d.status", sysin.CollectDispatchStatusPending).
		Where(collectDispatchAllJobsSentCondition("d")).
		OrderAsc("d.updated_at").OrderAsc("d.id").Limit(limit).All()
	if err != nil {
		return gerror.Wrap(err, "读取已发送但未收口的采集分发失败")
	}
	if len(rows) == 0 {
		return nil
	}
	dispatchIDs := make([]int64, 0, len(rows))
	eventIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		dispatchIDs = append(dispatchIDs, row["id"].Int64())
		if eventID := row["event_id"].Int64(); eventID > 0 {
			eventIDs = append(eventIDs, eventID)
		}
	}
	now := gtime.Now()
	result, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		WhereIn("id", dispatchIDs).
		Where("status", sysin.CollectDispatchStatusPending).
		Where(collectDispatchAllJobsSentCondition(dispatchTable)).
		Data(g.Map{
			"status":        sysin.CollectDispatchStatusSent,
			"error_message": "",
			"finished_at":   now,
			"updated_at":    now,
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "批量收口采集分发状态失败")
	}
	reconciled, _ := result.RowsAffected()
	if len(eventIDs) > 0 {
		if _, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).
			WhereIn("id", eventIDs).
			WhereIn("status", []string{sysin.CollectEventStatusDispatched, sysin.CollectEventStatusFailed}).
			Where(collectEventAllDispatchesFinishedCondition(pdao.YoubanPublishCollectEvent.Table())).
			Data(g.Map{
				"status":        sysin.CollectEventStatusProcessed,
				"error_message": "",
				"processed_at":  now,
				"updated_at":    now,
			}).Update(); err != nil {
			return gerror.Wrap(err, "批量收口采集事件状态失败")
		}
	}
	if reconciled > 0 {
		g.Log().Infof(ctx, "已批量对账收口采集分发：%d条", reconciled)
	}
	return nil
}

func (s *sSysPublish) recoverCollectPublishTasks(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 50
	}
	// 采集分发恢复必须使用独立的小预算，不能因为历史积压拖住实时采集。
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	deadline := gtime.Now().Add(-collectEventRecoverAfter)
	d := pdao.YoubanPublishCollectDispatch.Columns()
	rows, err := pdao.YoubanPublishCollectDispatch.Ctx(queryCtx).
		Fields(d.Id, d.EventId, d.ProfileId, d.RuleId, d.UpdatedAt).
		Where("profile_id > 0").
		Where("status", sysin.CollectDispatchStatusPending).
		WhereLTE("updated_at", deadline).
		OrderAsc("updated_at").OrderAsc("id").Limit(limit).All()
	if err != nil {
		return gerror.Wrap(err, "读取待恢复采集资料分发失败")
	}
	repaired := 0
	for _, row := range rows {
		event, eventErr := pdao.YoubanPublishCollectEvent.Ctx(queryCtx).
			Fields("id,tenant_id,account_id,source_id,bot_id,source_chat_id,source_message_id").
			Where("id", row["event_id"].Int64()).One()
		if eventErr != nil || event.IsEmpty() {
			continue
		}
		if err = s.submitCollectProfileDispatch(queryCtx, row["id"].Int64(), row["profile_id"].Int64(), event); err != nil {
			g.Log().Warningf(ctx, "恢复采集资料TG任务失败 dispatch:%d err:%+v", row["id"].Int64(), err)
			continue
		}
		repaired++
	}
	if repaired > 0 {
		g.Log().Infof(ctx, "已恢复采集资料推送：%d条", repaired)
	}
	return nil
}
