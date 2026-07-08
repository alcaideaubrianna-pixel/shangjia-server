package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"
)

const (
	tgQueueNameUrgent        = "youban_publish_tg_urgent"
	tgQueueNameDefault       = "youban_publish_tg"
	tgQueueNameBulk          = "youban_publish_tg_bulk"
	tgQueueNameMedia         = "youban_publish_media"
	tgTaskTypeSubmit         = "youban_publish:tg:submit"
	tgTaskTypePublish        = "youban_publish:tg:publish"
	tgTaskTypeDelete         = "youban_publish:tg:delete"
	tgTaskTypeCleanup        = "youban_publish:tg:cleanup"
	tgTaskTypeImport         = "youban_publish:import:legacy"
	tgTaskTypeRepair         = "youban_publish:tg:message_repair"
	tgTaskTypeImportMatch    = "youban_publish:import:tg_match"
	tgTaskTypeImportSync     = "youban_publish:import:tg_sync"
	tgTaskTypeDown           = "youban_publish:profile:down"
	tgTaskTypeCycleRun       = "youban_publish:cycle:run"
	tgTaskTypeCollectMedia   = "youban_publish:collect:media_cache"
	tgTaskTypeCollectHistory = "youban_publish:collect:history"
)

const (
	tgJobPriorityUrgent  = 10
	tgJobPriorityDefault = 50
	tgJobPriorityBulk    = 90
)

type tgQueuePayload struct {
	JobId int64 `json:"jobId"`
}

type publishSubmitQueuePayload struct {
	TaskId               int64   `json:"taskId"`
	TenantId             int64   `json:"tenantId"`
	AccountId            int64   `json:"accountId"`
	OperatorId           int64   `json:"operatorId"`
	OperationNo          string  `json:"operationNo"`
	ChannelIds           []int64 `json:"channelIds"`
	OnlySelectedChannels bool    `json:"onlySelectedChannels"`
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
	TenantId   int64   `json:"tenantId"`
	ProfileIds []int64 `json:"profileIds"`
	DownAt     string  `json:"downAt"`
}

type cycleRunQueuePayload struct {
	RunId int64 `json:"runId"`
}

type collectHistoryQueuePayload struct {
	TaskId int64 `json:"taskId"`
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
	if delay <= 0 {
		if windowDelay, enabled := s.telegramPublishWindowDelay(ctx); enabled && windowDelay > 0 {
			delay = windowDelay
		}
	}
	return s.scheduleTelegramJob(ctx, jobId, delay)
}

func (s *sSysPublish) enqueueTelegramDeleteJob(ctx context.Context, jobId int64, delay time.Duration) error {
	return s.enqueueTelegramTask(ctx, tgTaskTypeDelete, jobId, delay, true)
}

func (s *sSysPublish) enqueueTelegramCleanupJob(ctx context.Context, jobId int64, delay time.Duration) error {
	return s.enqueueTelegramTask(ctx, tgTaskTypeCleanup, jobId, delay, true)
}

func (s *sSysPublish) requeueTelegramJob(ctx context.Context, taskType string, jobId int64, delay time.Duration) error {
	if taskType == tgTaskTypePublish {
		return s.scheduleTelegramJob(ctx, jobId, delay)
	}
	return s.enqueueTelegramTask(ctx, taskType, jobId, delay, false)
}

func (s *sSysPublish) enqueuePublishSubmitTask(ctx context.Context, payload publishSubmitQueuePayload, delay time.Duration) error {
	if payload.TaskId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeSubmit, body)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameUrgent),
		asynq.MaxRetry(3),
		asynq.Timeout(10 * time.Minute),
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
		asynq.Queue(tgQueueNameBulk),
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
		asynq.Queue(tgQueueNameBulk),
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
		asynq.Queue(tgQueueNameBulk),
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
		asynq.Queue(tgQueueNameBulk),
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
	payload, err := json.Marshal(profileDownQueuePayload{TenantId: tenantId, ProfileIds: profileIds, DownAt: gtime.Now().String()})
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
		asynq.Queue(tgQueueNameBulk),
		asynq.MaxRetry(3),
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

func decodeCycleRunQueuePayload(task *asynq.Task) (cycleRunQueuePayload, error) {
	var payload cycleRunQueuePayload
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

func decodePublishSubmitQueuePayload(task *asynq.Task) (publishSubmitQueuePayload, error) {
	var payload publishSubmitQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析上架提交队列任务失败: %w", err)
	}
	if payload.TaskId <= 0 {
		return payload, fmt.Errorf("上架提交队列任务缺少taskId")
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
