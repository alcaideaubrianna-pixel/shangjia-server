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
)

const (
	tgQueueNameUrgent           = "youban_publish_tg_urgent"
	tgQueueNameDefault          = "youban_publish_tg"
	tgQueueNameBulk             = "youban_publish_tg_bulk"
	tgQueueNameMedia            = "youban_publish_media"
	tgQueueNameMediaRealtime    = "youban_publish_media_realtime"
	tgQueueNameMediaBulkPrefix  = "youban_publish_media_bulk_"
	tgQueueNameBackground       = "youban_publish_background"
	tgTaskTypePublish           = "youban_publish:tg:publish"
	tgTaskTypeCleanup           = "youban_publish:tg:cleanup"
	tgTaskTypeImport            = "youban_publish:import:legacy"
	tgTaskTypeRepair            = "youban_publish:tg:message_repair"
	tgTaskTypeImportMatch       = "youban_publish:import:tg_match"
	tgTaskTypeImportSync        = "youban_publish:import:tg_sync"
	tgTaskTypeDown              = "youban_publish:profile:down"
	tgTaskTypeCycleRun          = "youban_publish:cycle:run"
	tgTaskTypeCycleReschedule   = "youban_publish:cycle:reschedule"
	tgTaskTypeCycleRefresh      = "youban_publish:cycle:refresh"
	tgTaskTypeCollectMedia      = "youban_publish:collect:media_cache"
	tgTaskTypeCollectProcess    = "youban_publish:collect:process"
	tgTaskTypeCollectHistory    = "youban_publish:collect:history"
	tgTaskTypeCollectTrigger    = "youban_publish:collect:trigger"
	tgTaskTypeChannelMemberSync = "youban_publish:tg:channel_member_sync"
)

const collectMediaMaxBulkQueueShards = 16

func collectMediaBulkQueueName(shard int) string {
	if shard < 0 {
		shard = -shard
	}
	return fmt.Sprintf("%s%d", tgQueueNameMediaBulkPrefix, shard%collectMediaMaxBulkQueueShards)
}

func collectMediaBulkQueueNames() []string {
	queues := make([]string, 0, collectMediaMaxBulkQueueShards)
	for shard := 0; shard < collectMediaMaxBulkQueueShards; shard++ {
		queues = append(queues, collectMediaBulkQueueName(shard))
	}
	return queues
}

const (
	tgJobPriorityUrgent  = 10
	tgJobPriorityDefault = 50
	tgJobPriorityBulk    = 90
)

type tgQueuePayload struct {
	JobId int64 `json:"jobId"`
}

type importQueuePayload struct {
	Id    int64 `json:"id"`
	RunId int64 `json:"runId"`
}

type tgMessageRepairQueuePayload struct {
	RunId int64 `json:"runId"`
}

type importMatchQueuePayload struct {
	MatchRunId int64 `json:"matchRunId"`
}

type profileDownQueuePayload struct {
	TenantId    int64   `json:"tenantId"`
	ProfileIds  []int64 `json:"profileIds"`
	DownAt      string  `json:"downAt"`
	OperationNo string  `json:"operationNo"`
}

type cycleRunQueuePayload struct {
	RunId int64 `json:"runId"`
}

type cycleRescheduleQueuePayload struct {
	ChannelId int64 `json:"channelId"`
}

type cycleRefreshQueuePayload struct {
	ChannelId int64 `json:"channelId"`
}

type collectHistoryQueuePayload struct {
	TaskId int64 `json:"taskId"`
}

type collectTriggerQueuePayload struct {
	AccountId int64 `json:"accountId"`
	SourceId  int64 `json:"sourceId"`
	TenantId  int64 `json:"tenantId"`
}

type tgRetryAfterError struct {
	after time.Duration
	err   error
}

func (e *tgRetryAfterError) Error() string {
	return e.err.Error()
}

func (e *tgRetryAfterError) Unwrap() error {
	return e.err
}

func (s *sSysPublish) enqueueTelegramJob(ctx context.Context, jobId int64, delay time.Duration) error {
	return s.enqueueTelegramJobDirect(ctx, jobId, delay)
}

func (s *sSysPublish) enqueueTelegramJobDeferred(ctx context.Context, jobId int64, delay time.Duration) error {
	return s.enqueueTelegramJobDirect(ctx, jobId, delay)
}

func (s *sSysPublish) enqueueTelegramJobDirect(ctx context.Context, jobId int64, delay time.Duration) error {
	return s.enqueueTelegramJobDirectWithUnique(ctx, jobId, delay, true)
}

func (s *sSysPublish) enqueueTelegramJobDirectWithUnique(ctx context.Context, jobId int64, delay time.Duration, unique bool) error {
	if jobId <= 0 {
		return nil
	}
	if delay <= 0 {
		if windowDelay, enabled := s.telegramPublishWindowDelay(ctx); enabled && windowDelay > 0 {
			delay = windowDelay
		}
	}
	job, err := s.telegramJobById(ctx, jobId)
	if err != nil {
		return err
	}
	if active, checkErr := s.telegramJobChannelIsActive(ctx, job); checkErr != nil {
		return checkErr
	} else if !active {
		_, updateErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("id", jobId).
			WhereIn("status", []string{"pending", "failed_retry", "unknown", "sending"}).
			Data(g.Map{
				"status":              "superseded",
				"dispatch_status":     tgDispatchStatusDone,
				"next_retry_at":       nil,
				"error_message":       "目标频道已删除或禁用，任务自动终止",
				"last_dispatch_error": "目标频道已删除或禁用，任务自动终止",
				"updated_at":          gtime.Now(),
			}).Update()
		if updateErr != nil {
			return gerror.Wrap(updateErr, "终止无效频道TG任务失败")
		}
		return nil
	}
	priority := s.telegramJobPriority(job)
	queueName := telegramQueueNameByPriorityAndChannel(ctx, priority, job.ChannelId)
	data := g.Map{
		"priority":            priority,
		"queue_name":          queueName,
		"dispatch_status":     tgDispatchStatusQueued,
		"dispatched_at":       gtime.Now(),
		"dispatch_count":      gdb.Raw("dispatch_count + 1"),
		"last_dispatch_error": "",
		"updated_at":          gtime.Now(),
	}
	if delay > 0 {
		data["next_retry_at"] = gtime.Now().Add(delay)
	} else {
		data["next_retry_at"] = nil
	}
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).
		WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
		Where("(dispatch_status = ? OR dispatch_status = '')", tgDispatchStatusIdle).
		Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "锁定TG任务入队状态失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	if err = s.enqueueTelegramTaskWithQueue(ctx, tgTaskTypePublish, jobId, delay, unique, queueName); err != nil {
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobId).
			Where("dispatch_status", tgDispatchStatusQueued).
			Data(g.Map{
				"dispatch_status":     tgDispatchStatusIdle,
				"last_dispatch_error": err.Error(),
				"updated_at":          gtime.Now(),
			}).Update()
		return gerror.Wrap(err, "TG任务直接入队失败")
	}
	return nil
}

func (s *sSysPublish) enqueueTelegramCleanupJob(ctx context.Context, jobId int64, delay time.Duration) error {
	return s.enqueueTelegramTask(ctx, tgTaskTypeCleanup, jobId, delay, true)
}

func (s *sSysPublish) requeueTelegramJob(ctx context.Context, taskType string, jobId int64, delay time.Duration) error {
	if taskType == tgTaskTypePublish {
		return s.enqueueTelegramJobDirect(ctx, jobId, delay)
	}
	return s.enqueueTelegramTask(ctx, taskType, jobId, delay, false)
}

func (s *sSysPublish) enqueueImportTask(ctx context.Context, id int64, delay time.Duration) error {
	if id <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(importQueuePayload{Id: id})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeImport, payload)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameBackground),
		asynq.MaxRetry(0),
		asynq.Timeout(2 * time.Hour),
		asynq.Unique(30 * time.Second),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) enqueueImportRun(ctx context.Context, runId int64, delay time.Duration) error {
	if runId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(importQueuePayload{RunId: runId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeImport, payload)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameBackground),
		asynq.MaxRetry(0),
		asynq.Timeout(2 * time.Hour),
		asynq.Unique(30 * time.Second),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) enqueueTgMessageRepairRun(ctx context.Context, runId int64, delay time.Duration) error {
	if runId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(tgMessageRepairQueuePayload{RunId: runId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeRepair, payload)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameBackground),
		asynq.MaxRetry(0),
		asynq.Timeout(2 * time.Hour),
		asynq.Unique(30 * time.Second),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) enqueueImportMatchRun(ctx context.Context, matchRunId int64, delay time.Duration) error {
	return s.enqueueImportRunMatchTask(ctx, tgTaskTypeImportMatch, matchRunId, delay)
}

func (s *sSysPublish) enqueueImportTgSyncRun(ctx context.Context, matchRunId int64, delay time.Duration) error {
	return s.enqueueImportRunMatchTask(ctx, tgTaskTypeImportSync, matchRunId, delay)
}

func (s *sSysPublish) enqueueImportRunMatchTask(ctx context.Context, taskType string, matchRunId int64, delay time.Duration) error {
	if matchRunId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(importMatchQueuePayload{MatchRunId: matchRunId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(taskType, payload)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameBackground),
		asynq.MaxRetry(0),
		asynq.Timeout(2 * time.Hour),
		asynq.Unique(30 * time.Second),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) enqueueProfileDownRun(ctx context.Context, tenantId int64, profileIds []int64, delay time.Duration) error {
	profileIds = uniqueIds(profileIds)
	if tenantId <= 0 || len(profileIds) == 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(profileDownQueuePayload{
		TenantId: tenantId, ProfileIds: profileIds, DownAt: gtime.Now().String(),
		OperationNo: newTelegramOperationNo("down", profileIds[0]),
	})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeDown, payload)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameUrgent),
		asynq.MaxRetry(10),
		asynq.Timeout(30 * time.Minute),
		asynq.Unique(30 * time.Second),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) enqueueCycleRun(ctx context.Context, runId int64, delay time.Duration) error {
	if runId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(cycleRunQueuePayload{RunId: runId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeCycleRun, payload)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameBackground),
		asynq.MaxRetry(3),
		asynq.Timeout(30 * time.Minute),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sSysPublish) enqueueCycleReschedule(ctx context.Context, channelId int64, delay time.Duration) error {
	if channelId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(cycleRescheduleQueuePayload{ChannelId: channelId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeCycleReschedule, payload)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameBackground),
		asynq.MaxRetry(5),
		asynq.Timeout(2 * time.Hour),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	return err
}

func (s *sSysPublish) enqueueCycleSummaryRefresh(ctx context.Context, channelId int64, delay time.Duration) error {
	if channelId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(cycleRefreshQueuePayload{ChannelId: channelId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeCycleRefresh, payload)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameBackground),
		asynq.MaxRetry(5),
		asynq.Timeout(time.Minute),
		asynq.Unique(2 * time.Minute),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func decodeCycleRunQueuePayload(task *asynq.Task) (cycleRunQueuePayload, error) {
	var payload cycleRunQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func decodeCycleRescheduleQueuePayload(task *asynq.Task) (cycleRescheduleQueuePayload, error) {
	var payload cycleRescheduleQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func decodeCycleRefreshQueuePayload(task *asynq.Task) (cycleRefreshQueuePayload, error) {
	var payload cycleRefreshQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, err
	}
	return payload, nil
}

func (s *sSysPublish) enqueueTelegramTask(ctx context.Context, taskType string, jobId int64, delay time.Duration, unique bool) error {
	return s.enqueueTelegramTaskWithQueue(ctx, taskType, jobId, delay, unique, tgQueueNameDefault)
}

func (s *sSysPublish) enqueueTelegramTaskWithQueue(ctx context.Context, taskType string, jobId int64, delay time.Duration, unique bool, queueName string) error {
	if jobId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(tgQueuePayload{JobId: jobId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(taskType, payload)
	if queueName == "" {
		queueName = tgQueueNameDefault
	}
	options := []asynq.Option{
		asynq.Queue(queueName),
		asynq.MaxRetry(10),
		asynq.Timeout(5 * time.Minute),
	}
	if unique {
		options = append(options, asynq.Unique(30*time.Second))
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	info, err := client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	if err != nil {
		return err
	}
	_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobId).Data(g.Map{
		"asynq_task_id": info.ID,
		"queue_name":    queueName,
		"updated_at":    time.Now(),
	}).Update()
	return nil
}

func (s *sSysPublish) telegramQueueClient(ctx context.Context) (*asynq.Client, error) {
	s.tgQueueMu.Lock()
	defer s.tgQueueMu.Unlock()
	if s.tgQueueClient != nil {
		return s.tgQueueClient, nil
	}
	client := asynq.NewClient(telegramQueueRedisOpt(ctx))
	s.tgQueueClient = client
	return client, nil
}

func telegramQueueRedisOpt(ctx context.Context) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr:     g.Cfg().MustGet(ctx, "redis.default.address", "127.0.0.1:6379").String(),
		Password: g.Cfg().MustGet(ctx, "redis.default.pass", "").String(),
		DB:       g.Cfg().MustGet(ctx, "redis.default.db", 0).Int(),
	}
}

func telegramQueueRetryDelay(n int, err error, task *asynq.Task) time.Duration {
	var retryErr *tgRetryAfterError
	if errors.As(err, &retryErr) && retryErr.after > 0 {
		return retryErr.after + time.Duration(n)*time.Second
	}
	var collectRetryErr *collectMediaRetryError
	if errors.As(err, &collectRetryErr) && collectRetryErr.delay > 0 {
		return collectRetryErr.delay + time.Duration(n)*time.Second
	}
	var collectProcessRetryErr *collectProcessRetryError
	if errors.As(err, &collectProcessRetryErr) && collectProcessRetryErr.delay > 0 {
		return collectProcessRetryErr.delay + time.Duration(n)*time.Second
	}
	return asynq.DefaultRetryDelayFunc(n, err, task)
}

func decodeTelegramQueuePayload(task *asynq.Task) (tgQueuePayload, error) {
	var payload tgQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析TG队列任务失败: %w", err)
	}
	if payload.JobId <= 0 {
		return payload, fmt.Errorf("TG队列任务缺少jobId")
	}
	return payload, nil
}

func decodeImportQueuePayload(task *asynq.Task) (importQueuePayload, error) {
	var payload importQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析导入队列任务失败: %w", err)
	}
	if payload.Id <= 0 && payload.RunId <= 0 {
		return payload, fmt.Errorf("导入队列任务缺少id")
	}
	return payload, nil
}

func decodeTgMessageRepairQueuePayload(task *asynq.Task) (tgMessageRepairQueuePayload, error) {
	var payload tgMessageRepairQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析TG消息修复队列任务失败: %w", err)
	}
	if payload.RunId <= 0 {
		return payload, fmt.Errorf("TG消息修复队列任务缺少runId")
	}
	return payload, nil
}

func decodeImportMatchQueuePayload(task *asynq.Task) (importMatchQueuePayload, error) {
	var payload importMatchQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析导入TG匹配队列任务失败: %w", err)
	}
	if payload.MatchRunId <= 0 {
		return payload, fmt.Errorf("导入TG匹配队列任务缺少matchRunId")
	}
	return payload, nil
}

func decodeProfileDownQueuePayload(task *asynq.Task) (profileDownQueuePayload, error) {
	var payload profileDownQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析资料下架队列任务失败: %w", err)
	}
	payload.ProfileIds = uniqueIds(payload.ProfileIds)
	if payload.TenantId <= 0 || len(payload.ProfileIds) == 0 {
		return payload, fmt.Errorf("资料下架队列任务缺少tenantId或profileIds")
	}
	return payload, nil
}
