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

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
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
	collectProcessTaskBatchRuns = 4
)

func (s *sSysPublish) enqueueCollectProcess(ctx context.Context, payload collectProcessQueuePayload, delay time.Duration) error {
	if payload.TenantId <= 0 || payload.AccountId <= 0 || payload.SourceId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	body, err := collectProcessTaskBody(payload)
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeCollectProcess, body)
	uniqueTTL := collectProcessTaskUniqueTTL
	if delay > 0 && delay+10*time.Second > uniqueTTL {
		uniqueTTL = delay + 10*time.Second
	}
	options := []asynq.Option{
		asynq.Queue(tgQueueNameBackground),
		asynq.Unique(uniqueTTL),
		asynq.MaxRetry(collectProcessTaskMaxRetry),
		asynq.Timeout(30 * time.Minute),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
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

func (s *sSysPublish) processCollectSourceWindowWithLock(ctx context.Context, payload collectProcessQueuePayload) error {
	enabled, err := collectProcessSourceEnabled(ctx, payload)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}
	key := fmt.Sprintf("youban_publish:collect:source:%d:%d:%d", payload.TenantId, payload.AccountId, payload.SourceId)
	distributedLock := lock.NewConfig(15*time.Minute, time.Second).Mutex(key)
	if err = distributedLock.TryLock(ctx); err != nil {
		return err
	}
	defer func() { _ = distributedLock.Unlock(context.Background()) }()
	g.Log().Infof(ctx, "采集窗口开始执行 sourceId:%d tenantId:%d accountId:%d", payload.SourceId, payload.TenantId, payload.AccountId)
	if err = s.processCollectSourceWindow(ctx, payload); err != nil {
		return gerror.Wrapf(err, "采集窗口执行失败 sourceId:%d", payload.SourceId)
	}
	return nil
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

func (s *sSysPublish) processCollectSourceTask(ctx context.Context, payload collectProcessQueuePayload) error {
	for run := 0; run < collectProcessTaskBatchRuns; run++ {
		if err := s.processCollectSourceWindowWithLock(ctx, payload); err != nil {
			return err
		}
		delay, pending, err := nextCollectProcessDelay(ctx, payload)
		if err != nil {
			return err
		}
		if !pending {
			return nil
		}
		if delay > time.Second {
			g.Log().Debugf(ctx, "采集源等待分组窗口 sourceId:%d retryAfter:%s", payload.SourceId, delay.Round(time.Second))
			return nil
		}
	}
	g.Log().Debugf(ctx, "采集源本轮批处理已完成，剩余事件由后续消息或恢复器继续 sourceId:%d", payload.SourceId)
	return nil
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
	verifyDeadline := now.Add(-collectMaterialVerifyRetryDelay)
	due, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields("id").
		Where("tenant_id", payload.TenantId).
		Where("account_id", payload.AccountId).
		Where("source_id", payload.SourceId).
		WhereIn("status", statuses).
		Where("material_role IS NULL OR material_role = '' OR material_role = ? OR (status = ? AND material_role = ? AND error_message = ?)", collectMaterialRolePending, sysin.CollectEventStatusIgnored, collectMaterialRoleVerify, collectMaterialVerifyUnmatchedMessage).
		Where("(processed_at IS NULL AND (received_at <= ? OR (received_at IS NULL AND created_at <= ?))) OR (status = ? AND material_role = ? AND error_message = ? AND updated_at <= ?)", groupDeadline, groupDeadline, sysin.CollectEventStatusIgnored, collectMaterialRoleVerify, collectMaterialVerifyUnmatchedMessage, verifyDeadline).
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
		WhereNull("processed_at").
		OrderAsc("ready_from").
		Limit(1).
		One()
	if err != nil {
		return 0, false, gerror.Wrap(err, "读取采集源分组等待任务失败")
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
