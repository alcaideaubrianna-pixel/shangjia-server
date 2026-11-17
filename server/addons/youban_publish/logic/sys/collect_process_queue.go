package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.opentelemetry.io/otel/attribute"
	"hotgo/internal/library/collectobserve"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"

	publishconsts "hotgo/addons/youban_publish/consts"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/hgrds/lock"
)

type collectProcessRetryError struct {
	delay   time.Duration
	message string
}

func (e *collectProcessRetryError) Error() string {
	if e.message == "" {
		return "采集事件等待后重试"
	}
	return e.message
}

func newCollectProcessRetryError(delay time.Duration, message string) error {
	if delay <= 0 {
		delay = 30 * time.Second
	}
	return &collectProcessRetryError{delay: delay, message: message}
}

func markCollectProcessSourceActive(ctx context.Context, payload collectProcessQueuePayload) {
	if payload.TenantId <= 0 || payload.AccountId < 0 || payload.SourceId <= 0 || !cache.Initialized() {
		return
	}
	client, err := cache.RedisClient()
	if err != nil {
		return
	}
	member := fmt.Sprintf("%d:%d:%d", payload.TenantId, payload.AccountId, payload.SourceId)
	_, _ = client.Do(ctx, "SADD", collectProcessActiveSourcesKey, member)
	// Redis-first needs recovery membership, but must not wake the legacy Outbox.
	if collectProcessOutboxMode(ctx) != "postgres_outbox" {
		return
	}
	// Keep wakeups isolated per tenant/account/source. The database remains
	// authoritative; this Redis list only wakes the source-scoped dispatcher.
	wakeLockKey := collectProcessSourceWakeLockKey(member)
	woken, wakeErr := client.Do(ctx, "SET", wakeLockKey, "1", "NX", "EX", 30)
	if wakeErr == nil && wakeAccepted(woken.String()) {
		_, _ = client.Do(ctx, "LPUSH", collectProcessSourceWakeQueueKey(member), member)
	}
}

type collectProcessQueuePayload struct {
	History      bool   `json:"history,omitempty"`
	AccountId    int64  `json:"accountId"`
	EventId      int64  `json:"eventId"`
	SourceId     int64  `json:"sourceId"`
	SourceChatId string `json:"sourceChatId,omitempty"`
	TenantId     int64  `json:"tenantId"`
	RuleId       int64  `json:"ruleId,omitempty"`
	RuleQueueKey string `json:"ruleQueueKey,omitempty"`
	OutboxOwner  string `json:"outboxOwner,omitempty"`
	Recovery     bool   `json:"recovery,omitempty"`
}

const collectProcessActiveSourcesKey = "youban_publish:collect:active_sources"

func collectProcessHistoryMarkerKey(eventID int64) string {
	return fmt.Sprintf("youban_publish:collect:history:event:%d", eventID)
}

func markCollectProcessHistory(ctx context.Context, payload collectProcessQueuePayload) {
	if !payload.History || payload.EventId <= 0 || !cache.Initialized() {
		return
	}
	if err := cache.Instance().Set(ctx, collectProcessHistoryMarkerKey(payload.EventId), 1, 24*time.Hour); err != nil {
		g.Log().Debugf(ctx, "写入采集历史任务标记失败 eventId:%d err:%+v", payload.EventId, err)
	}
}

func collectProcessSourceWakeLockKey(member string) string {
	return "youban_publish:collect:source:wake:lock:" + member
}

// collectProcessSourceWakeKey is kept as the compatibility name used by
// recovery code; it identifies the per-source wake lock, never the queue.
func collectProcessSourceWakeKey(member string) string {
	return collectProcessSourceWakeLockKey(member)
}

func collectProcessSourceWakeQueueKey(member string) string {
	return "youban_publish:collect:source:wake:queue:" + member
}

func wakeAccepted(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "OK")
}

const (
	collectProcessTaskUniqueTTL   = 30 * time.Minute
	collectProcessTaskMaxRetry    = 1000
	collectProcessScheduleTTL     = 35 * time.Minute
	collectProcessMinimumDelay    = 5 * time.Second
	collectProcessRedisRetryDelay = 15 * time.Second

	collectProcessCandidateIndexTTL        = 15 * time.Minute
	collectProcessCandidateIndexBatchSize  = 256
	collectProcessCandidateIndexReadFactor = 4
)

// collectProcessOutboxMode controls the durability boundary for the rule
// processing queue. Redis/Asynq is the realtime execution queue; PostgreSQL
// Outbox is retained for history and explicit recovery only.
//
// "redis_first" is the default so every realtime collection event avoids a
// PostgreSQL upsert before it reaches the worker. Set "postgres_outbox" only
// while migrating an older deployment or replaying a legacy backlog.
func collectProcessOutboxMode(ctx context.Context) string {
	mode := g.Cfg().MustGet(ctx, "youbanPublish.collect.processOutboxMode", "redis_first").String()
	return normalizeCollectProcessOutboxMode(mode)
}

func normalizeCollectProcessOutboxMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "postgres_outbox", "outbox", "legacy":
		return "postgres_outbox"
	default:
		return "redis_first"
	}
}

func collectProcessShouldPersistOutbox(ctx context.Context, payload collectProcessQueuePayload) bool {
	return collectProcessShouldPersistOutboxMode(collectProcessOutboxMode(ctx), payload)
}

func collectProcessShouldPersistOutboxMode(mode string, payload collectProcessQueuePayload) bool {
	if normalizeCollectProcessOutboxMode(mode) == "postgres_outbox" {
		return true
	}
	// Only a task already claimed by the legacy dispatcher keeps the old
	// PostgreSQL lease fence. New realtime and history tasks both use Redis;
	// their priority/classification travels in the task payload.
	return payload.OutboxOwner != ""
}

// isCollectProcessRedisTransientError keeps Redis startup/reconnect failures
// out of the permanent collection-failure path. Redis returns LOADING while
// restoring AOF/RDB; the source event itself is still valid and should retry
// shortly after Redis becomes writable.
func isCollectProcessRedisTransientError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "redis is loading the dataset") || strings.Contains(message, "loading redis") {
		return true
	}
	if !strings.Contains(message, "redis") && !strings.Contains(message, "cache") {
		return false
	}
	for _, marker := range []string{
		"connection refused",
		"connection reset by peer",
		"broken pipe",
		"i/o timeout",
		"context deadline exceeded",
		"timeout",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func collectProcessCandidateIndexKey(payload collectProcessQueuePayload) string {
	return fmt.Sprintf("youban_publish:collect:candidates:%d:%d:%d", payload.TenantId, payload.AccountId, payload.SourceId)
}

func collectSourceHistoryCursorKey(tenantID, accountID, sourceID int64, sourceChatID string) string {
	return fmt.Sprintf("youban_publish:collect:history:%d:%d:%d:%s", tenantID, accountID, sourceID, canonicalCollectSourceChatID(sourceChatID))
}

func indexCollectSourceHistoryCursor(ctx context.Context, tenantID, accountID, sourceID int64, sourceChatID string, sourceMessageID, eventID int64) {
	if tenantID <= 0 || accountID < 0 || sourceID <= 0 || sourceMessageID <= 0 || eventID <= 0 || !cache.Initialized() {
		return
	}
	if err := cache.ZSetAdd(ctx, collectSourceHistoryCursorKey(tenantID, accountID, sourceID, sourceChatID), float64(sourceMessageID), fmt.Sprintf("%d", eventID)); err != nil {
		g.Log().Debugf(ctx, "写入采集源历史Redis游标失败 eventId:%d err:%+v", eventID, err)
	}
}

func collectProcessCandidateIndexMarkerKey(payload collectProcessQueuePayload) string {
	return collectProcessCandidateIndexKey(payload) + ":alive"
}

func collectProcessCandidateIndexReady(ctx context.Context, payload collectProcessQueuePayload) bool {
	if !cache.Initialized() {
		return false
	}
	value, err := cache.Instance().Get(ctx, collectProcessCandidateIndexMarkerKey(payload))
	if err == nil && !value.IsNil() && value.Int() == 1 {
		return true
	}
	// The marker is the logical TTL for the ZSET. Do not delete the ZSET on a
	// marker miss: a concurrent producer may have refreshed candidates after
	// the marker expired, and deleting it would force every caller to rebuild
	// the same source window.
	return false
}

func indexCollectProcessCandidate(ctx context.Context, payload collectProcessQueuePayload, eventID int64) {
	if eventID <= 0 || payload.TenantId <= 0 || payload.AccountId < 0 || payload.SourceId <= 0 || !cache.Initialized() {
		return
	}
	key := collectProcessCandidateIndexKey(payload)
	if err := cache.ZSetAdd(ctx, key, float64(eventID), fmt.Sprintf("%d", eventID)); err != nil {
		g.Log().Debugf(ctx, "写入采集候选Redis索引失败 eventId:%d err:%+v", eventID, err)
		return
	}
	if err := cache.Instance().Set(ctx, collectProcessCandidateIndexMarkerKey(payload), 1, collectProcessCandidateIndexTTL); err != nil {
		g.Log().Debugf(ctx, "刷新采集候选Redis索引TTL失败 eventId:%d err:%+v", eventID, err)
	}
}

func collectProcessCandidateIDs(ctx context.Context, payload collectProcessQueuePayload, limit int) []int64 {
	if limit <= 0 || !collectProcessCandidateIndexReady(ctx, payload) {
		return nil
	}
	if limit > collectProcessCandidateIndexBatchSize*collectProcessCandidateIndexReadFactor {
		limit = collectProcessCandidateIndexBatchSize * collectProcessCandidateIndexReadFactor
	}
	members, err := cache.ZSetRange(ctx, collectProcessCandidateIndexKey(payload), 0, limit-1)
	if err != nil {
		g.Log().Debugf(ctx, "读取采集候选Redis索引失败 err:%+v", err)
		return nil
	}
	ids := make([]int64, 0, len(members))
	for _, member := range members {
		var eventID int64
		if _, scanErr := fmt.Sscan(member, &eventID); scanErr == nil && eventID > 0 {
			ids = append(ids, eventID)
		}
	}
	return ids
}

func removeCollectProcessCandidate(ctx context.Context, payload collectProcessQueuePayload, eventID int64) {
	if eventID <= 0 || !cache.Initialized() {
		return
	}
	if err := cache.ZSetRemove(ctx, collectProcessCandidateIndexKey(payload), fmt.Sprintf("%d", eventID)); err != nil {
		g.Log().Debugf(ctx, "移除采集候选Redis索引失败 eventId:%d err:%+v", eventID, err)
	}
}

func (s *sSysPublish) enqueueCollectProcess(ctx context.Context, payload collectProcessQueuePayload, delay time.Duration) error {
	_, err := s.enqueueCollectProcessTask(ctx, payload, delay, true)
	return err
}

func (s *sSysPublish) enqueueCollectProcessDeferred(ctx context.Context, payload collectProcessQueuePayload, delay time.Duration) (bool, error) {
	return s.enqueueCollectProcessTask(ctx, payload, delay, false)
}

func (s *sSysPublish) enqueueCollectProcessTask(ctx context.Context, payload collectProcessQueuePayload, delay time.Duration, unique bool) (bool, error) {
	return s.enqueueCollectProcessTaskWithOutbox(ctx, payload, delay, unique, true)
}

func (s *sSysPublish) enqueueCollectProcessTaskWithOutbox(ctx context.Context, payload collectProcessQueuePayload, delay time.Duration, unique, persist bool) (enqueued bool, err error) {
	// BOT 采集事件可能没有协议号账户归属，account_id=0 仍是合法租户+来源分片。
	if payload.TenantId <= 0 || payload.AccountId < 0 || payload.SourceId <= 0 {
		return false, nil
	}
	sourcePendingReserved := false
	if payload.EventId == 0 && persist {
		reserved, reserveErr := reserveCollectProcessSourcePending(ctx, payload)
		if reserveErr != nil {
			return false, reserveErr
		}
		if !reserved {
			return false, nil
		}
		sourcePendingReserved = true
		defer func() {
			if sourcePendingReserved && err != nil {
				removeCollectProcessSourcePending(ctx, payload)
			}
		}()
	}
	// Event-specific enqueue paths are the low-cost producer for the candidate
	// index. The index is only an accelerator; PG remains authoritative below.
	if payload.EventId > 0 {
		if persist && unique && payload.SourceChatId != "" {
			if dirtyErr := dirtyCollectTargetBatch(ctx, payload); dirtyErr != nil {
				return false, dirtyErr
			}
		}
		if registerErr := registerCollectProcessRecoveryScope(ctx, payload); registerErr != nil {
			return false, registerErr
		}
		indexCollectProcessCandidate(ctx, payload, payload.EventId)
		markCollectProcessHistory(ctx, payload)
	}
	if persist && collectProcessShouldPersistOutbox(ctx, payload) {
		if err := ensureCollectProcessOutbox(ctx, payload, delay); err != nil {
			if !isMissingCollectProcessOutboxError(err) {
				return false, err
			}
			g.Log().Warningf(ctx, "采集处理Outbox尚未迁移，临时沿用Redis任务 eventId:%d err:%+v", payload.EventId, err)
		} else if payload.EventId > 0 {
			markCollectProcessSourceActive(ctx, payload)
			// Persist first, then hand the event directly to the realtime Redis queue.
			// The durable outbox remains the recovery source if Redis enqueue fails.
			return s.enqueueCollectProcessTaskWithOutbox(ctx, payload, delay, unique, false)
		}
	}
	// Redis-first realtime work is already durable in Asynq. Do not create an
	// otherwise-unused PostgreSQL Outbox row merely to describe a Redis task.
	// The event table remains the business projection and the task's unique key
	// plus retry policy provide the execution handoff.
	if persist && payload.EventId > 0 && collectProcessOutboxMode(ctx) == "postgres_outbox" {
		markCollectProcessSourceActive(ctx, payload)
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return false, err
	}
	body, err := collectProcessTaskBody(payload)
	if err != nil {
		return false, err
	}
	task := asynq.NewTask(tgTaskTypeCollectProcess, body)
	uniqueTTL := collectProcessTaskUniqueTTL
	if delay > 0 && delay+10*time.Second > uniqueTTL {
		uniqueTTL = delay + 10*time.Second
	}
	options := []asynq.Option{
		asynq.Queue(collectProcessQueueName(payload)),
		asynq.MaxRetry(collectProcessTaskMaxRetry),
		asynq.Timeout(2 * time.Minute),
	}
	if payload.OutboxOwner != "" {
		options = append(options, asynq.TaskID("collect-"+payload.OutboxOwner))
	} else if unique {
		reserved, reserveErr := reserveCollectProcessSchedule(ctx, payload, uniqueTTL)
		if reserveErr != nil {
			return false, reserveErr
		}
		if !reserved {
			return false, nil
		}
		options = append(options, asynq.Unique(uniqueTTL))
	} else if err = refreshCollectProcessSchedule(ctx, payload, uniqueTTL); err != nil {
		return false, err
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
		return false, nil
	}
	if err != nil {
		removeCollectProcessSchedule(ctx, payload)
	}
	return err == nil, err
}

func collectProcessScheduleKey(payload collectProcessQueuePayload) string {
	if payload.EventId > 0 {
		return fmt.Sprintf("%s%d:%d:event:%d", publishconsts.CollectProcessScheduleKeyPrefix, payload.TenantId, payload.AccountId, payload.EventId)
	}
	return fmt.Sprintf("%s%d:%d:%s", publishconsts.CollectProcessScheduleKeyPrefix, payload.TenantId, payload.AccountId, collectProcessPartition(payload))
}

func collectProcessThrottleKey(payload collectProcessQueuePayload) string {
	return fmt.Sprintf("%s%d:%d:%s", publishconsts.CollectProcessThrottleKeyPrefix, payload.TenantId, payload.AccountId, collectProcessPartition(payload))
}

func collectProcessSourcePendingKey(payload collectProcessQueuePayload) string {
	return fmt.Sprintf("youban_publish:collect:source:pending:%d:%d:%s", payload.TenantId, payload.AccountId, collectProcessPartition(payload))
}

func reserveCollectProcessSourcePending(ctx context.Context, payload collectProcessQueuePayload) (bool, error) {
	if payload.EventId > 0 || !cache.Initialized() || g.Cfg().MustGet(ctx, "cache.adapter").String() != "redis" {
		return true, nil
	}
	ok, err := cache.Instance().SetIfNotExist(ctx, collectProcessSourcePendingKey(payload), 1, collectProcessScheduleTTL)
	if err != nil {
		g.Log().Warningf(ctx, "采集源合并缓存不可用，放行任务 sourceId:%d err:%+v", payload.SourceId, err)
		return true, nil
	}
	return ok, nil
}

func removeCollectProcessSourcePending(ctx context.Context, payload collectProcessQueuePayload) {
	if payload.EventId > 0 || !cache.Initialized() || g.Cfg().MustGet(ctx, "cache.adapter").String() != "redis" {
		return
	}
	if _, err := cache.Instance().Remove(ctx, collectProcessSourcePendingKey(payload)); err != nil {
		g.Log().Warningf(ctx, "清理采集源合并缓存失败 sourceId:%d err:%+v", payload.SourceId, err)
	}
}

func collectProcessPartition(payload collectProcessQueuePayload) string {
	chatID := canonicalCollectSourceChatID(payload.SourceChatId)
	if chatID == "" {
		return fmt.Sprintf("%d", payload.SourceId)
	}
	return fmt.Sprintf("%d:%s", payload.SourceId, chatID)
}

func reserveCollectProcessExecution(ctx context.Context, payload collectProcessQueuePayload) (bool, error) {
	if !cache.Initialized() || g.Cfg().MustGet(ctx, "cache.adapter").String() != "redis" {
		return true, nil
	}
	ok, err := cache.Instance().SetIfNotExist(ctx, collectProcessThrottleKey(payload), 1, collectProcessMinimumDelay)
	if err != nil {
		g.Log().Warningf(ctx, "采集源执行节流缓存不可用，放行任务 sourceId:%d err:%+v", payload.SourceId, err)
		return true, nil
	}
	return ok, nil
}

func reserveCollectProcessSchedule(ctx context.Context, payload collectProcessQueuePayload, ttl time.Duration) (bool, error) {
	if !cache.Initialized() || g.Cfg().MustGet(ctx, "cache.adapter").String() != "redis" {
		return true, nil
	}
	if ttl <= 0 {
		ttl = collectProcessScheduleTTL
	}
	ok, err := cache.Instance().SetIfNotExist(ctx, collectProcessScheduleKey(payload), 1, ttl)
	if err != nil {
		g.Log().Warningf(ctx, "采集源调度去重缓存不可用，放行任务 sourceId:%d err:%+v", payload.SourceId, err)
		return true, nil
	}
	if !ok {
		if err = cache.Instance().Set(ctx, collectProcessScheduleKey(payload), 1, ttl); err != nil {
			return false, gerror.Wrap(err, "续期采集源调度去重缓存失败")
		}
	}
	return ok, nil
}

func (s *sSysPublish) reconcileCollectProcessSchedule(ctx context.Context, payload collectProcessQueuePayload) error {
	if payload.EventId > 0 {
		removeCollectProcessCandidate(ctx, payload, payload.EventId)
		removeCollectProcessSchedule(ctx, payload)
		return nil
	}
	removeCollectProcessSchedule(ctx, payload)
	enabled, err := collectProcessSourceEnabled(ctx, payload)
	if err != nil || !enabled {
		return err
	}
	delay, pending, err := nextCollectProcessDelay(ctx, payload)
	if err != nil || !pending {
		return err
	}
	if delay < collectProcessMinimumDelay {
		delay = collectProcessMinimumDelay
	}
	_, err = s.enqueueCollectProcessTask(ctx, payload, delay, true)
	return err
}

func refreshCollectProcessSchedule(ctx context.Context, payload collectProcessQueuePayload, ttl time.Duration) error {
	if !cache.Initialized() || g.Cfg().MustGet(ctx, "cache.adapter").String() != "redis" {
		return nil
	}
	if ttl <= 0 {
		ttl = collectProcessScheduleTTL
	}
	if err := cache.Instance().Set(ctx, collectProcessScheduleKey(payload), 1, ttl); err != nil {
		return gerror.Wrap(err, "刷新采集源调度去重缓存失败")
	}
	return nil
}

func removeCollectProcessSchedule(ctx context.Context, payload collectProcessQueuePayload) {
	if !cache.Initialized() || g.Cfg().MustGet(ctx, "cache.adapter").String() != "redis" {
		return
	}
	if _, err := cache.Instance().Remove(ctx, collectProcessScheduleKey(payload)); err != nil {
		g.Log().Warningf(ctx, "清理采集源调度去重缓存失败 sourceId:%d err:%+v", payload.SourceId, err)
	}
}

func collectProcessTaskBody(payload collectProcessQueuePayload) ([]byte, error) {
	return json.Marshal(collectProcessQueuePayload{
		History:      payload.History,
		TenantId:     payload.TenantId,
		AccountId:    payload.AccountId,
		EventId:      payload.EventId,
		SourceId:     payload.SourceId,
		SourceChatId: payload.SourceChatId,
		RuleId:       payload.RuleId,
		RuleQueueKey: collectProcessRuleQueueKey(payload),
		OutboxOwner:  payload.OutboxOwner,
		Recovery:     payload.Recovery,
	})
}

func decodeCollectProcessQueuePayload(task *asynq.Task) (collectProcessQueuePayload, error) {
	var payload collectProcessQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析采集事件处理任务失败: %w", err)
	}
	// account_id=0 是 BOT 采集的合法值，不能在任务解码阶段丢弃。
	if payload.SourceId <= 0 || payload.TenantId <= 0 || payload.AccountId < 0 {
		return payload, fmt.Errorf("采集源处理任务参数不完整")
	}
	return payload, nil
}

func (s *sSysPublish) processCollectSourceWindowWithLock(ctx context.Context, payload collectProcessQueuePayload) (bool, error) {
	enabled, err := collectProcessSourceEnabled(ctx, payload)
	if err != nil {
		return false, err
	}
	if !enabled {
		return false, nil
	}
	key := fmt.Sprintf("youban_publish:collect:source:%d:%d:%s", payload.TenantId, payload.AccountId, collectProcessPartition(payload))
	distributedLock := lock.NewConfig(2*time.Minute, time.Second).Mutex(key)
	if err = distributedLock.TryLock(ctx); err != nil {
		if errors.Is(err, lock.ErrLockFailed) {
			g.Log().Debugf(ctx, "采集源已有任务执行，本任务主动让出 sourceId:%d", payload.SourceId)
			return false, nil
		}
		return false, err
	}
	defer func() { _ = distributedLock.Unlock(context.Background()) }()
	g.Log().Infof(ctx, "采集窗口开始执行 sourceId:%d tenantId:%d accountId:%d", payload.SourceId, payload.TenantId, payload.AccountId)
	if err = s.processCollectSourceWindow(ctx, payload); err != nil {
		return false, gerror.Wrapf(err, "采集窗口执行失败 sourceId:%d", payload.SourceId)
	}
	return true, nil
}

func collectProcessSourceEnabled(ctx context.Context, payload collectProcessQueuePayload) (bool, error) {
	columns := pdao.YoubanPublishCollectSource.Columns()
	sourceModel := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", payload.SourceId).
		Where("tenant_id", payload.TenantId).
		Where("collect_enabled", 1).
		Where(columns.ChannelBlacklisted, 0).
		Where("status", 1).
		WhereNull("deleted_at").
		Fields(columns.Id)
	if payload.AccountId > 0 {
		sourceModel = sourceModel.Where(columns.AccountId, payload.AccountId)
	}
	// 实时路径只需判断是否存在，不能使用 COUNT 扫描来源表。
	matchedSource, err := sourceModel.One()
	if err != nil {
		return false, err
	}
	if !matchedSource.IsEmpty() {
		return true, nil
	}
	source, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Fields(columns.Id, columns.AccountId, columns.TgAccountId, columns.CollectEnabled, columns.Status, columns.ChannelBlacklisted).
		Where(columns.Id, payload.SourceId).Where(columns.TenantId, payload.TenantId).One()
	if err != nil {
		return false, err
	}
	g.Log().Warning(ctx, "采集处理跳过：采集源归属或状态不匹配", g.Map{
		"reason":   "source_account_or_status_mismatch",
		"tenantId": payload.TenantId, "sourceId": payload.SourceId,
		"payloadAccountId":   payload.AccountId,
		"sourceAccountId":    source[columns.AccountId].Int64(),
		"sourceTgAccountId":  source[columns.TgAccountId].Int64(),
		"collectEnabled":     source[columns.CollectEnabled].Int(),
		"sourceStatus":       source[columns.Status].Int(),
		"channelBlacklisted": source[columns.ChannelBlacklisted].Int(),
	})
	return false, nil
}

func (s *sSysPublish) processCollectSourceTask(ctx context.Context, payload collectProcessQueuePayload) (delay time.Duration, pending bool, removeSchedule bool, err error) {
	mode := "unknown"
	if payload.History {
		mode = "history"
	} else if !payload.Recovery && payload.EventId > 0 {
		mode = "realtime"
	}
	ctx = collectobserve.With(ctx, collectobserve.Metadata{TenantID: payload.TenantId, AccountID: payload.AccountId, SourceID: payload.SourceId, EventID: payload.EventId, ChatID: payload.SourceChatId, RuleID: payload.RuleId, Mode: mode})
	observationStartedAt := time.Now()
	attemptResult := collectPipelineCompleted
	defer func() {
		result := attemptResult
		if err != nil {
			result = collectPipelineFailed
		} else if pending {
			result = collectPipelinePending
		}
		observeCollectPipeline(ctx, collectPipelineSourceAttempt, result, observationStartedAt, err, attribute.String("collect.wait_reason", collectSourceAttemptWaitReason(pending, err)))
	}()
	windowCtx, cancelWindow := context.WithTimeout(ctx, collectMaterialWindowExecutionTimeout)
	defer cancelWindow()
	ctx = windowCtx
	if !cache.Initialized() {
		return collectProcessRedisRetryDelay, true, false, nil
	}
	if _, cacheErr := cache.Instance().Get(ctx, collectTargetBatchKey(payload)+":dirty"); cacheErr != nil {
		return collectProcessRedisRetryDelay, true, false, nil
	}
	enabled, err := collectProcessSourceEnabled(ctx, payload)
	if err != nil {
		return 0, false, false, err
	}
	if !enabled {
		attemptResult = collectPipelineIgnored
		return 0, false, true, nil
	}
	if payload.EventId > 0 {
		event, readErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(payload.EventId).One()
		if readErr != nil {
			return 0, false, false, gerror.Wrap(readErr, "读取实时采集事件失败")
		}
		if event.IsEmpty() {
			attemptResult = collectPipelineIgnored
			return 0, false, true, nil
		}
		if !collectProcessEventMatchesPayload(event, payload) {
			return 0, false, false, gerror.New("采集事件与任务归属不一致")
		}
		ctx = bindCollectEventObservation(ctx, event)
		lateVerificationRepair := collectMaterialLateVerificationRepairCandidate(event)
		if collectProcessEventFinished(event) {
			if !lateVerificationRepair {
				attemptResult = collectPipelineDuplicate
				if event["status"].String() == sysin.CollectEventStatusIgnored {
					attemptResult = collectPipelineIgnored
				}
				if collectMaterialEventNeedsPairRepair(event) {
					g.Log().Info(ctx, "未匹配验证已终止主动轮询，保留被动配对", g.Map{
						"event": "collect_unmatched_verify_wakeup_closed", "tenantId": payload.TenantId,
						"accountId": payload.AccountId, "sourceId": payload.SourceId, "collectEventId": payload.EventId,
						"sourceChatId": event["source_chat_id"].String(), "passivePairingEligible": true,
					})
				}
				return 0, false, true, nil
			}
			// Only a late binding (group=paired) may reopen completed material.
			// Reuse existing branch/profile repair; do not create a second pipeline.
			needsRepair, repairErr := s.collectEventNeedsProfileRepair(ctx, payload.EventId)
			if repairErr != nil {
				return 0, false, false, repairErr
			}
			if !needsRepair {
				attemptResult = collectPipelineDuplicate
				return 0, false, true, nil
			}
		}
		if event["processed_at"].GTime() != nil && event["status"].String() != sysin.CollectEventStatusIgnored {
			cols := pdao.YoubanPublishCollectEvent.Columns()
			if _, clearErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(payload.EventId).Where(cols.Status, event["status"].String()).Data(cols.ProcessedAt, nil).Update(); clearErr != nil {
				return 0, false, false, gerror.Wrap(clearErr, "清理未完成事件的历史完成时间失败")
			}
		}
		var processErr error
		if lateVerificationRepair || (event["material_role"].String() == collectMaterialRoleVerify && event["material_parent_event_id"].Int64() > 0) {
			processErr = s.processCollectEvent(ctx, payload.EventId, payload.TenantId, payload.AccountId)
		} else {
			processErr = s.processTargetCollectEventWindow(ctx, payload, event)
		}
		if err = processErr; err != nil {
			var retryErr *collectProcessRetryError
			if errors.As(err, &retryErr) {
				reportCollectProgress(ctx, "waiting", retryErr.message)
				return retryErr.delay, true, false, nil
			}
			return 0, false, false, err
		}
		// A nil business return can mean media was queued, not that the event
		// finished. Keep the wakeup until its durable state confirms completion.
		current, readErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(payload.EventId).
			Fields("id,status,material_role,material_group_status,error_message").One()
		if readErr != nil {
			return 0, false, false, gerror.Wrap(readErr, "确认采集事件完成状态失败")
		}
		if !current.IsEmpty() && !collectProcessEventFinished(current) {
			return 5 * time.Second, true, false, nil
		}
		return 0, false, true, nil
	}
	reserved, err := reserveCollectProcessExecution(ctx, payload)
	if err != nil {
		return 0, false, false, err
	}
	if !reserved {
		g.Log().Debugf(ctx, "采集源任务命中跨节点节流，丢弃重复任务 sourceId:%d", payload.SourceId)
		// Do not drop the task while leaving its schedule reservation alive.
		// A short retry keeps the source self-healing under contention.
		return collectProcessMinimumDelay, true, false, nil
	}
	executed, err := s.processCollectSourceWindowWithLock(ctx, payload)
	if err != nil {
		return 0, false, false, err
	}
	if !executed {
		return collectProcessMinimumDelay, true, false, nil
	}
	delay, pending, err = nextCollectProcessDelay(ctx, payload)
	if err != nil {
		return 0, false, false, err
	}
	return delay, pending, !pending, nil
}

func nextCollectProcessDelay(ctx context.Context, payload collectProcessQueuePayload) (time.Duration, bool, error) {
	// Read a small page, not a COUNT/MIN aggregation over a source's history.
	// Candidate-only emptiness must be checked against one scoped fallback page.
	candidateIDs := collectProcessCandidateIDs(ctx, payload, collectProcessCandidateIndexBatchSize*collectProcessCandidateIndexReadFactor)
	rows, err := collectProcessScheduleProbe(candidateIDs, func(ids []int64) (gdb.Result, error) {
		model := collectMaterialActiveWindowModel(ctx, payload)
		if len(ids) > 0 {
			model = model.WhereIn("e.id", ids)
		}
		return model.Fields("e.id,e.status,e.material_role,e.material_group_status,e.processed_at,e.created_at,e.updated_at").
			OrderAsc("e.updated_at").OrderAsc("e.id").Limit(collectProcessScheduleProbeLimit).All()
	})
	if err != nil {
		return 0, false, gerror.Wrap(err, "读取采集源后续任务窗口失败")
	}
	return collectProcessScheduleRowsDelay(rows)
}

func collectProcessEventModel(ctx context.Context, payload collectProcessQueuePayload) *gdb.Model {
	// The caller has already passed collectProcessSourceEnabled before entering
	// the source window. Rejoining collect_source here made every realtime
	// candidate scan touch the source table again and prevented the event
	// index from being the sole driving relation. The source status is checked
	// again on the next scheduled run, while the event query stays scoped by
	// tenant/source/account/chat.
	model := g.DB().Model(publishCollectEventTable+" e").Ctx(ctx).
		Where("e.tenant_id", payload.TenantId).
		Where("e.source_id", payload.SourceId)
	if payload.AccountId > 0 {
		model = model.Where("e.account_id", payload.AccountId)
	}
	if chatID := canonicalCollectSourceChatID(payload.SourceChatId); chatID != "" {
		model = model.WhereIn("e.source_chat_id", tgChannelCacheLookupIds(chatID))
	}
	return model
}

func collectProcessWindowStatuses() []string {
	// Wider recovery/target windows may inspect failed or passive verification;
	// ordinary source scheduling uses only the active prefix above.
	return append(collectProcessActiveWindowStatuses(), sysin.CollectEventStatusIgnored, sysin.CollectEventStatusFailed)
}

func collectLocalWindowRemaining(value *gtime.Time, window time.Duration) time.Duration {
	if value == nil || window <= 0 {
		return 0
	}
	elapsed := collectLocalElapsedSince(value)
	if elapsed >= window {
		return 0
	}
	if elapsed < 0 {
		return window
	}
	return window - elapsed
}

// Terminal events can still have stale tasks in Redis. Completing these tasks
// is idempotent; returning an error would retry them up to 1000 times.
func collectProcessEventFinished(event gdb.Record) bool {
	status := event["status"].String()
	switch status {
	case sysin.CollectEventStatusProcessed, sysin.CollectEventStatusDispatched:
		return true
	case sysin.CollectEventStatusFailed:
		return !shouldRecoverCollectEvent(event)
	case sysin.CollectEventStatusIgnored:
		// A rejected/unmatched event is finished for its own durable wakeup.
		// Late pairing may still read it as passive context when new material
		// arrives; recoverability must not keep an ignored task polling forever.
		return true
	}
	return false
}

func collectProcessEventMatchesPayload(event gdb.Record, payload collectProcessQueuePayload) bool {
	if event["tenant_id"].Int64() != payload.TenantId || event["source_id"].Int64() != payload.SourceId {
		return false
	}
	// BOT 事件没有协议号账户，按租户+来源匹配即可；有账户的任务仍严格隔离。
	return payload.AccountId == 0 || event["account_id"].Int64() == payload.AccountId
}

// Serialize legacy and v2 tasks for the same event, including queue migration.
func lockCollectProcessEvent(ctx context.Context, payload collectProcessQueuePayload) (func(), error) {
	if payload.EventId <= 0 {
		return func() {}, nil
	}
	mutex := lock.NewConfig(2*time.Minute, time.Second).Mutex(fmt.Sprintf("youban_publish:collect:process:event:%d:%d", payload.TenantId, payload.EventId))
	if err := mutex.TryLock(ctx); err != nil {
		return nil, err
	}
	return func() { _ = mutex.Unlock(context.Background()) }, nil
}

// A successful late bind persists this marker before queuing the parent. Never
// reopen arbitrary ignored/failed events or bypass their authorization checks.
func collectMaterialLateVerificationRepairCandidate(event gdb.Record) bool {
	return event["material_role"].String() == collectMaterialRoleDisplay &&
		event["material_group_status"].String() == "paired" &&
		(event["status"].String() == sysin.CollectEventStatusProcessed || event["status"].String() == sysin.CollectEventStatusDispatched)
}
