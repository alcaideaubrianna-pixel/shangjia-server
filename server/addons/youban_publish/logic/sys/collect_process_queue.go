package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

type collectProcessQueuePayload struct {
	AccountId    int64  `json:"accountId"`
	EventId      int64  `json:"eventId"`
	SourceId     int64  `json:"sourceId"`
	SourceChatId string `json:"sourceChatId,omitempty"`
	TenantId     int64  `json:"tenantId"`
	OutboxOwner  string `json:"outboxOwner,omitempty"`
	Recovery     bool   `json:"recovery,omitempty"`
}

const (
	collectProcessTaskUniqueTTL = 30 * time.Minute
	collectProcessTaskMaxRetry  = 1000
	collectProcessScheduleTTL   = 35 * time.Minute
	collectProcessMinimumDelay  = 5 * time.Second
)

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

func (s *sSysPublish) enqueueCollectProcessTaskWithOutbox(ctx context.Context, payload collectProcessQueuePayload, delay time.Duration, unique, persist bool) (bool, error) {
	if payload.TenantId <= 0 || payload.AccountId <= 0 || payload.SourceId <= 0 {
		return false, nil
	}
	if persist {
		if err := ensureCollectProcessOutbox(ctx, payload, delay); err != nil {
			if !isMissingCollectProcessOutboxError(err) {
				return false, err
			}
			g.Log().Warningf(ctx, "采集处理Outbox尚未迁移，临时沿用Redis任务 eventId:%d err:%+v", payload.EventId, err)
		} else if payload.EventId > 0 {
			return true, nil // Only the durable dispatcher creates event tasks.
		}
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
		TenantId:     payload.TenantId,
		AccountId:    payload.AccountId,
		EventId:      payload.EventId,
		SourceId:     payload.SourceId,
		SourceChatId: payload.SourceChatId,
		OutboxOwner:  payload.OutboxOwner,
		Recovery:     payload.Recovery,
	})
}

func decodeCollectProcessQueuePayload(task *asynq.Task) (collectProcessQueuePayload, error) {
	var payload collectProcessQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析采集事件处理任务失败: %w", err)
	}
	if payload.SourceId <= 0 || payload.TenantId <= 0 || payload.AccountId <= 0 {
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
	count, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", payload.SourceId).
		Where("tenant_id", payload.TenantId).
		Where("account_id", payload.AccountId).
		Where("collect_enabled", 1).
		Where(pdao.YoubanPublishCollectSource.Columns().ChannelBlacklisted, 0).
		Where("status", 1).
		WhereNull("deleted_at").
		Count()
	return count > 0, err
}

func (s *sSysPublish) processCollectSourceTask(ctx context.Context, payload collectProcessQueuePayload) (delay time.Duration, pending bool, removeSchedule bool, err error) {
	windowCtx, cancelWindow := context.WithTimeout(ctx, collectMaterialWindowExecutionTimeout)
	defer cancelWindow()
	ctx = windowCtx
	enabled, err := collectProcessSourceEnabled(ctx, payload)
	if err != nil {
		return 0, false, false, err
	}
	if !enabled {
		return 0, false, true, nil
	}
	if payload.EventId > 0 {
		event, readErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(payload.EventId).One()
		if readErr != nil {
			return 0, false, false, gerror.Wrap(readErr, "读取实时采集事件失败")
		}
		if event.IsEmpty() {
			return 0, false, true, nil
		}
		if !collectProcessEventMatchesPayload(event, payload) {
			return 0, false, false, gerror.New("采集事件与任务归属不一致")
		}
		if collectProcessEventFinished(event) {
			return 0, false, true, nil
		}
		if event["processed_at"].GTime() != nil && event["status"].String() != sysin.CollectEventStatusIgnored {
			cols := pdao.YoubanPublishCollectEvent.Columns()
			if _, clearErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(payload.EventId).Where(cols.Status, event["status"].String()).Data(cols.ProcessedAt, nil).Update(); clearErr != nil {
				return 0, false, false, gerror.Wrap(clearErr, "清理未完成事件的历史完成时间失败")
			}
		}
		var processErr error
		if event["material_role"].String() == collectMaterialRoleVerify && event["material_parent_event_id"].Int64() > 0 {
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
	statuses := collectProcessWindowStatuses()
	now := time.Now()
	groupDeadline := now.Add(-collectMaterialGroupingDelay)
	waitingVerifyDeadline := now.Add(-collectMaterialWaitingVerifyRetryDelay)
	verifyDeadline := now.Add(-collectMaterialVerifyRetryDelay)
	dueModel := collectProcessEventModel(ctx, payload)
	due, err := dueModel.
		Fields("e.id").
		WhereIn("e.status", statuses).
		Where("((e.status <> ? AND (e.material_role IS NULL OR e.material_role = '' OR e.material_role IN (?,?))) OR (e.status = ? AND e.material_role = ? AND e.error_message = ?))", sysin.CollectEventStatusIgnored, collectMaterialRolePending, collectMaterialRoleDisplay, sysin.CollectEventStatusIgnored, collectMaterialRoleVerify, collectMaterialVerifyUnmatchedMessage).
		Where("(e.processed_at IS NULL AND (e.material_group_status IS NULL OR e.material_group_status <> ?) AND e.created_at <= ?) OR (e.processed_at IS NULL AND e.material_group_status = ? AND e.updated_at <= ?) OR (e.status = ? AND e.material_role = ? AND e.error_message = ? AND e.updated_at <= ?)", collectMaterialGroupWaitingVerify, groupDeadline, collectMaterialGroupWaitingVerify, waitingVerifyDeadline, sysin.CollectEventStatusIgnored, collectMaterialRoleVerify, collectMaterialVerifyUnmatchedMessage, verifyDeadline).
		Limit(1).
		One()
	if err != nil {
		return 0, false, gerror.Wrap(err, "读取采集源后续任务失败")
	}
	if !due.IsEmpty() {
		return 0, true, nil
	}
	normal, err := collectProcessEventModel(ctx, payload).
		Fields("e.created_at AS ready_from").
		WhereIn("e.status", statuses).
		Where("e.status <> ? AND (e.material_role IS NULL OR e.material_role = '' OR e.material_role = ?)", sysin.CollectEventStatusIgnored, collectMaterialRolePending).
		Where("e.material_group_status IS NULL OR e.material_group_status <> ?", collectMaterialGroupWaitingVerify).
		WhereNull("e.processed_at").
		OrderAsc("e.created_at").
		Limit(1).
		One()
	if err != nil {
		return 0, false, gerror.Wrap(err, "读取采集源分组等待任务失败")
	}
	waitingVerify, err := collectProcessEventModel(ctx, payload).
		Fields("e.updated_at AS ready_from").
		WhereIn("e.status", statuses).
		Where("e.material_role", collectMaterialRolePending).
		Where("e.material_group_status", collectMaterialGroupWaitingVerify).
		WhereNull("e.processed_at").
		OrderAsc("e.updated_at").
		Limit(1).
		One()
	if err != nil {
		return 0, false, gerror.Wrap(err, "读取采集源验证配对等待任务失败")
	}
	verify, err := collectProcessEventModel(ctx, payload).
		Fields("e.updated_at AS ready_from").
		Where("e.status", sysin.CollectEventStatusIgnored).
		Where("e.material_role", collectMaterialRoleVerify).
		Where("e.error_message", collectMaterialVerifyUnmatchedMessage).
		OrderAsc("e.updated_at").
		Limit(1).
		One()
	if err != nil {
		return 0, false, gerror.Wrap(err, "读取采集源验证等待任务失败")
	}
	var nextDelay time.Duration
	hasNextDelay := false
	if !normal.IsEmpty() && normal["ready_from"].GTime() != nil {
		nextDelay = collectLocalWindowRemaining(normal["ready_from"].GTime(), collectMaterialGroupingDelay)
		hasNextDelay = true
	}
	if !waitingVerify.IsEmpty() && waitingVerify["ready_from"].GTime() != nil {
		waitingDelay := collectLocalWindowRemaining(waitingVerify["ready_from"].GTime(), collectMaterialWaitingVerifyRetryDelay)
		if !hasNextDelay || waitingDelay < nextDelay {
			nextDelay = waitingDelay
			hasNextDelay = true
		}
	}
	if !verify.IsEmpty() && verify["ready_from"].GTime() != nil {
		verifyDelay := collectLocalWindowRemaining(verify["ready_from"].GTime(), collectMaterialVerifyRetryDelay)
		if !hasNextDelay || verifyDelay < nextDelay {
			nextDelay = verifyDelay
			hasNextDelay = true
		}
	}
	if !hasNextDelay {
		return 0, false, nil
	}
	if nextDelay <= 0 {
		return 0, true, nil
	}
	return nextDelay, true, nil
}

func collectProcessEventModel(ctx context.Context, payload collectProcessQueuePayload) *gdb.Model {
	model := g.DB().Model(publishCollectEventTable+" e").Ctx(ctx).
		InnerJoin(publishCollectSourceTable+" s", "s.id=e.source_id AND s.tenant_id=e.tenant_id").
		Where("e.tenant_id", payload.TenantId).
		Where("e.account_id", payload.AccountId).
		Where("e.source_id", payload.SourceId).
		Where("s.channel_blacklisted", 0)
	if chatID := canonicalCollectSourceChatID(payload.SourceChatId); chatID != "" {
		model = model.WhereIn("e.source_chat_id", tgChannelCacheLookupIds(chatID))
	}
	return model
}

func collectProcessWindowStatuses() []string {
	return []string{
		sysin.CollectEventStatusPending,
		sysin.CollectEventStatusGroupCollect,
		sysin.CollectEventStatusWaitingOrder,
		sysin.CollectEventStatusPrechecked,
		sysin.CollectEventStatusMediaPending,
		sysin.CollectEventStatusMediaReady,
		sysin.CollectEventStatusIgnored,
		sysin.CollectEventStatusFailed,
	}
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
		return event["material_role"].String() != collectMaterialRoleVerify || event["error_message"].String() != collectMaterialVerifyUnmatchedMessage
	}
	return false
}

func collectProcessEventMatchesPayload(event gdb.Record, payload collectProcessQueuePayload) bool {
	return event["tenant_id"].Int64() == payload.TenantId && event["account_id"].Int64() == payload.AccountId && event["source_id"].Int64() == payload.SourceId
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
