package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
)

type collectMediaQueuePayload struct {
	EventId          int64  `json:"eventId"`
	TenantId         int64  `json:"tenantId"`
	AccountId        int64  `json:"accountId"`
	SourceId         int64  `json:"sourceId"`
	TgAccountId      int64  `json:"tgAccountId"`
	Bulk             bool   `json:"bulk,omitempty"`
	Reason           string `json:"reason,omitempty"`
	DeferredAttempts int    `json:"deferredAttempts,omitempty"`
}

const (
	collectMediaTaskUniqueTTL           = 24 * time.Hour
	collectMediaTaskTimeout             = 15 * time.Minute
	collectMediaRealtimeWindow          = 10 * time.Minute
	collectMediaQueueDefaultConcurrency = 16
	collectMediaQueueMaxConcurrency     = 32
	collectMediaBulkDefaultConcurrency  = 8
	collectMediaMaxDeferredAttempts     = 20
	collectMediaBulkMaxConcurrency      = 16
	mediaProcessDefaultConcurrency      = 2
	mediaProcessMaxConcurrency          = 8
)

func normalizeCollectMediaWorkerConcurrency(concurrency int, maximum int) int {
	if concurrency < 1 {
		return 1
	}
	if concurrency > maximum {
		return maximum
	}
	return concurrency
}

func collectMediaQueuePayloadFromEvent(event gdb.Record) collectMediaQueuePayload {
	payload := collectMediaQueuePayload{
		EventId:     event["id"].Int64(),
		TenantId:    event["tenant_id"].Int64(),
		AccountId:   event["account_id"].Int64(),
		SourceId:    event["source_id"].Int64(),
		TgAccountId: event["tg_account_id"].Int64(),
	}
	eventAt := collectMaterialEventAt(event)
	payload.Bulk = !eventAt.IsZero() && time.Since(eventAt) > collectMediaRealtimeWindow
	return payload
}

func collectMediaQueueName(ctx context.Context, payload collectMediaQueuePayload) string {
	shardKey := collectMediaQueueShardKey(payload)
	if !payload.Bulk {
		return collectMediaRealtimeQueueName(int(shardKey % int64(collectMediaMaxBulkQueueShards)))
	}
	return collectMediaBulkQueueName(int(shardKey % int64(collectMediaBulkQueueShards(ctx))))
}

func collectMediaQueueShardKey(payload collectMediaQueuePayload) int64 {
	// 账户采集优先按account_id隔离；只有没有账户归属的BOT采集才回退Bot/source分片。
	if payload.TenantId > 0 && payload.AccountId > 0 {
		return payload.AccountId
	}
	if payload.TgAccountId > 0 {
		return payload.TgAccountId
	}
	return payload.SourceId
}

func (s *sSysPublish) enqueueCollectMediaCache(ctx context.Context, payload collectMediaQueuePayload, delay time.Duration) error {
	_, err := s.enqueueCollectMediaCacheTask(ctx, payload, delay)
	return err
}

func (s *sSysPublish) enqueueMediaProcess(ctx context.Context, mediaId int64, delay time.Duration) error {
	if mediaId <= 0 {
		return nil
	}
	// PostgreSQL trigger also records first-time uploads in the media transaction.
	// Explicit requests cover repairs/projection retries and coalesce pending work.
	_, err := g.DB().GetValue(ctx, "SELECT ybp_request_media_process(?, ?)", mediaId, delay.Milliseconds())
	return err
}

func enqueueMediaProcessTask(ctx context.Context, client *asynq.Client, mediaId int64, taskID string, delay time.Duration) error {
	body, err := json.Marshal(mediaProcessQueuePayload{MediaId: mediaId, RetryPolicyVersion: 1})
	if err != nil {
		return err
	}
	options := []asynq.Option{
		asynq.Queue(tgQueueNameMediaProcess), asynq.MaxRetry(mediaProcessMaxAttempts - 1),
		asynq.Timeout(mediaProcessTaskTimeout), asynq.Retention(7 * 24 * time.Hour),
		asynq.TaskID(taskID),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeMediaProcess, body), options...)
	// Redelivery of the same intent never resets retry counts or revives archives.
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

func decodeMediaProcessQueuePayload(task *asynq.Task) (mediaProcessQueuePayload, error) {
	var payload mediaProcessQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析媒体处理任务失败: %w", err)
	}
	if payload.MediaId <= 0 {
		return payload, fmt.Errorf("媒体处理任务缺少mediaId")
	}
	return payload, nil
}

func (s *sSysPublish) enqueueCollectMediaCacheTask(ctx context.Context, payload collectMediaQueuePayload, delay time.Duration) (bool, error) {
	if payload.Reason == "" {
		payload.Reason = "pipeline"
	}
	return s.enqueueCollectMediaCacheTaskWithUnique(ctx, payload, delay, true)
}

func (s *sSysPublish) enqueueCollectMediaCacheDeferred(ctx context.Context, payload collectMediaQueuePayload, delay time.Duration) (bool, error) {
	if payload.Reason == "" {
		payload.Reason = "deferred"
	}
	return s.enqueueCollectMediaCacheTaskWithUnique(ctx, payload, delay, false)
}

func (s *sSysPublish) enqueueCollectMediaCacheTaskWithUnique(ctx context.Context, payload collectMediaQueuePayload, delay time.Duration, unique bool) (bool, error) {
	if payload.EventId <= 0 || payload.TenantId <= 0 || payload.AccountId <= 0 {
		return false, nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return false, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	task := asynq.NewTask(tgTaskTypeCollectMedia, body)
	queueName := collectMediaQueueName(ctx, payload)
	options := []asynq.Option{
		asynq.Queue(queueName),
		asynq.MaxRetry(10),
		asynq.Timeout(collectMediaTaskTimeout),
	}
	if unique {
		options = append(options, asynq.Unique(collectMediaTaskUniqueTTL))
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	var enqueued bool
	if unique {
		_, err = client.EnqueueContext(ctx, task, options...)
		enqueued = err == nil
	} else {
		inspector := asynq.NewInspector(telegramQueueRedisOpt(ctx))
		defer inspector.Close()
		taskKey := fmt.Sprintf("collect-media:%d:%s:%d", payload.EventId, payload.Reason, payload.DeferredAttempts)
		enqueued, err = enqueueCollectMediaCacheTaskWithID(ctx, client, inspector, task, queueName, options, taskKey, payload.EventId)
	}
	if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
		observeCollectMediaEnqueue(ctx, payload.Reason, "duplicate")
		return false, nil
	}
	if err != nil {
		observeCollectMediaEnqueue(ctx, payload.Reason, "error")
	} else {
		observeCollectMediaEnqueue(ctx, payload.Reason, "success")
	}
	return enqueued, err
}

func enqueueCollectMediaCacheTaskWithID(ctx context.Context, client *asynq.Client, inspector *asynq.Inspector, task *asynq.Task, queueName string, options []asynq.Option, baseID string, eventID int64) (bool, error) {
	for generation := 0; generation < 100; generation++ {
		taskID := baseID
		if generation > 0 {
			taskID = fmt.Sprintf("%s:repair:%d", baseID, generation)
		}
		taskOptions := append([]asynq.Option(nil), options...)
		taskOptions = append(taskOptions, asynq.TaskID(taskID))
		_, err := client.EnqueueContext(ctx, task, taskOptions...)
		if !errors.Is(err, asynq.ErrTaskIDConflict) {
			return err == nil, err
		}
		info, lookupErr := inspector.GetTaskInfo(queueName, taskID)
		if lookupErr != nil {
			return false, fmt.Errorf("读取采集媒体任务冲突状态失败: %w", lookupErr)
		}
		if info.State != asynq.TaskStateArchived && info.State != asynq.TaskStateCompleted {
			return false, nil
		}
	}
	return false, fmt.Errorf("采集事件 %d 的媒体任务历史轮次过多，需要人工检查", eventID)
}

func decodeCollectMediaQueuePayload(task *asynq.Task) (collectMediaQueuePayload, error) {
	var payload collectMediaQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析采集媒体缓存任务失败: %w", err)
	}
	if payload.EventId <= 0 {
		return payload, fmt.Errorf("采集媒体缓存任务缺少eventId")
	}
	return payload, nil
}

func collectMediaQueueConcurrency(ctx context.Context) int {
	concurrency := g.Cfg().MustGet(ctx, "youbanPublish.queue.mediaConcurrency", collectMediaQueueDefaultConcurrency).Int()
	return normalizeCollectMediaWorkerConcurrency(concurrency, collectMediaQueueMaxConcurrency)
}

func collectMediaBulkQueueShards(ctx context.Context) int {
	shards := g.Cfg().MustGet(ctx, "youbanPublish.queue.mediaBulkShards", collectMediaMaxBulkQueueShards).Int()
	if shards < 1 {
		return 1
	}
	if shards > collectMediaMaxBulkQueueShards {
		return collectMediaMaxBulkQueueShards
	}
	return shards
}

func collectMediaRealtimeWorkerQueues() map[string]int {
	queues := make(map[string]int, collectMediaMaxBulkQueueShards)
	for _, queue := range collectMediaRealtimeQueueNames() {
		queues[queue] = 1
	}
	return queues
}

func collectMediaBulkWorkerQueues(ctx context.Context) map[string]int {
	bulkWeight := g.Cfg().MustGet(ctx, "youbanPublish.queue.mediaBulkWeight", 1).Int()
	if bulkWeight < 1 {
		bulkWeight = 1
	}
	if bulkWeight > 20 {
		bulkWeight = 20
	}
	queues := make(map[string]int)
	for _, queue := range collectMediaBulkQueueNames() {
		queues[queue] = bulkWeight
	}
	return queues
}

func collectMediaBulkConcurrency(ctx context.Context) int {
	concurrency := g.Cfg().MustGet(ctx, "youbanPublish.queue.mediaBulkConcurrency", collectMediaBulkDefaultConcurrency).Int()
	return normalizeCollectMediaWorkerConcurrency(concurrency, collectMediaBulkMaxConcurrency)
}

func mediaProcessConcurrency(ctx context.Context) int {
	concurrency := g.Cfg().MustGet(ctx, "youbanPublish.queue.mediaProcessConcurrency", mediaProcessDefaultConcurrency).Int()
	return normalizeCollectMediaWorkerConcurrency(concurrency, mediaProcessMaxConcurrency)
}
