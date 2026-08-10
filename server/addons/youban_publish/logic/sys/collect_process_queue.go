package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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
	AccountId int64 `json:"accountId"`
	EventId   int64 `json:"eventId"`
	SourceId  int64 `json:"sourceId"`
	TenantId  int64 `json:"tenantId"`
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
	if payload.TenantId <= 0 || payload.AccountId <= 0 || payload.SourceId <= 0 {
		return false, nil
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
		asynq.Queue(tgQueueNameBackground),
		asynq.MaxRetry(collectProcessTaskMaxRetry),
		asynq.Timeout(30 * time.Minute),
	}
	if unique {
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
	return fmt.Sprintf("%s%d:%d:%d", publishconsts.CollectProcessScheduleKeyPrefix, payload.TenantId, payload.AccountId, payload.SourceId)
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
	return ok, nil
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
		TenantId:  payload.TenantId,
		AccountId: payload.AccountId,
		SourceId:  payload.SourceId,
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
	key := fmt.Sprintf("youban_publish:collect:source:%d:%d:%d", payload.TenantId, payload.AccountId, payload.SourceId)
	distributedLock := lock.NewConfig(15*time.Minute, time.Second).Mutex(key)
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
		Where("status", 1).
		WhereNull("deleted_at").
		Count()
	return count > 0, err
}

func (s *sSysPublish) processCollectSourceTask(ctx context.Context, payload collectProcessQueuePayload) (time.Duration, bool, error) {
	enabled, err := collectProcessSourceEnabled(ctx, payload)
	if err != nil {
		return 0, false, err
	}
	if !enabled {
		return 0, false, nil
	}
	executed, err := s.processCollectSourceWindowWithLock(ctx, payload)
	if err != nil {
		return 0, false, err
	}
	if !executed {
		return collectProcessMinimumDelay, true, nil
	}
	delay, pending, err := nextCollectProcessDelay(ctx, payload)
	if err != nil {
		return 0, false, err
	}
	return delay, pending, nil
}

func nextCollectProcessDelay(ctx context.Context, payload collectProcessQueuePayload) (time.Duration, bool, error) {
	statuses := []string{
		sysin.CollectEventStatusPending,
		sysin.CollectEventStatusGroupCollect,
		sysin.CollectEventStatusWaitingOrder,
		sysin.CollectEventStatusPrechecked,
		sysin.CollectEventStatusMediaPending,
		sysin.CollectEventStatusMediaReady,
		sysin.CollectEventStatusIgnored,
	}
	now := time.Now()
	groupDeadline := now.Add(-collectMaterialGroupingDelay)
	waitingVerifyDeadline := now.Add(-collectMaterialWaitingVerifyRetryDelay)
	verifyDeadline := now.Add(-collectMaterialVerifyRetryDelay)
	due, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields("id").
		Where("tenant_id", payload.TenantId).
		Where("account_id", payload.AccountId).
		Where("source_id", payload.SourceId).
		WhereIn("status", statuses).
		Where("material_role IS NULL OR material_role = '' OR material_role = ? OR (status = ? AND material_role = ? AND error_message = ?)", collectMaterialRolePending, sysin.CollectEventStatusIgnored, collectMaterialRoleVerify, collectMaterialVerifyUnmatchedMessage).
		Where("(processed_at IS NULL AND (material_group_status IS NULL OR material_group_status <> ?) AND (received_at <= ? OR (received_at IS NULL AND created_at <= ?))) OR (processed_at IS NULL AND material_group_status = ? AND updated_at <= ?) OR (status = ? AND material_role = ? AND error_message = ? AND updated_at <= ?)", collectMaterialGroupWaitingVerify, groupDeadline, groupDeadline, collectMaterialGroupWaitingVerify, waitingVerifyDeadline, sysin.CollectEventStatusIgnored, collectMaterialRoleVerify, collectMaterialVerifyUnmatchedMessage, verifyDeadline).
		Limit(1).
		One()
	if err != nil {
		return 0, false, gerror.Wrap(err, "读取采集源后续任务失败")
	}
	if !due.IsEmpty() {
		return 0, true, nil
	}
	normal, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields("COALESCE(received_at, created_at) AS ready_from").
		Where("tenant_id", payload.TenantId).
		Where("account_id", payload.AccountId).
		Where("source_id", payload.SourceId).
		WhereIn("status", statuses).
		Where("material_role IS NULL OR material_role = '' OR material_role = ?", collectMaterialRolePending).
		Where("material_group_status IS NULL OR material_group_status <> ?", collectMaterialGroupWaitingVerify).
		WhereNull("processed_at").
		OrderAsc("ready_from").
		Limit(1).
		One()
	if err != nil {
		return 0, false, gerror.Wrap(err, "读取采集源分组等待任务失败")
	}
	waitingVerify, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields("updated_at AS ready_from").
		Where("tenant_id", payload.TenantId).
		Where("account_id", payload.AccountId).
		Where("source_id", payload.SourceId).
		WhereIn("status", statuses).
		Where("material_role", collectMaterialRolePending).
		Where("material_group_status", collectMaterialGroupWaitingVerify).
		WhereNull("processed_at").
		OrderAsc("updated_at").
		Limit(1).
		One()
	if err != nil {
		return 0, false, gerror.Wrap(err, "读取采集源验证配对等待任务失败")
	}
	verify, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields("updated_at AS ready_from").
		Where("tenant_id", payload.TenantId).
		Where("account_id", payload.AccountId).
		Where("source_id", payload.SourceId).
		Where("status", sysin.CollectEventStatusIgnored).
		Where("material_role", collectMaterialRoleVerify).
		Where("error_message", collectMaterialVerifyUnmatchedMessage).
		OrderAsc("updated_at").
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
