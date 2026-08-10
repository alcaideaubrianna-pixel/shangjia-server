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
	EventId     int64 `json:"eventId"`
	TenantId    int64 `json:"tenantId"`
	AccountId   int64 `json:"accountId"`
	SourceId    int64 `json:"sourceId"`
	TgAccountId int64 `json:"tgAccountId"`
	Bulk        bool  `json:"bulk,omitempty"`
}

const (
	collectMediaTaskUniqueTTL  = 24 * time.Hour
	collectMediaRealtimeWindow = 10 * time.Minute
)

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
	if !payload.Bulk {
		return tgQueueNameMediaRealtime
	}
	return collectMediaBulkQueueName(int(payload.TgAccountId % int64(collectMediaBulkQueueShards(ctx))))
}

func (s *sSysPublish) enqueueCollectMediaCache(ctx context.Context, payload collectMediaQueuePayload, delay time.Duration) error {
	_, err := s.enqueueCollectMediaCacheTask(ctx, payload, delay)
	return err
}

func (s *sSysPublish) enqueueMediaProcess(ctx context.Context, mediaId int64, delay time.Duration) error {
	if mediaId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(mediaProcessQueuePayload{MediaId: mediaId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeMediaProcess, body)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameMediaRealtime),
		asynq.MaxRetry(8),
		asynq.Timeout(30 * time.Minute),
		asynq.Unique(24 * time.Hour),
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
	return s.enqueueCollectMediaCacheTaskWithUnique(ctx, payload, delay, true)
}

func (s *sSysPublish) enqueueCollectMediaCacheDeferred(ctx context.Context, payload collectMediaQueuePayload, delay time.Duration) (bool, error) {
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
	options := []asynq.Option{
		asynq.Queue(collectMediaQueueName(ctx, payload)),
		asynq.MaxRetry(10),
		asynq.Timeout(30 * time.Minute),
	}
	if unique {
		options = append(options, asynq.Unique(collectMediaTaskUniqueTTL))
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
		return false, nil
	}
	return err == nil, err
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
	concurrency := g.Cfg().MustGet(ctx, "youbanPublish.queue.mediaConcurrency", 3).Int()
	if concurrency < 1 {
		return 1
	}
	if concurrency > 8 {
		return 8
	}
	return concurrency
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

func collectMediaWorkerQueues(ctx context.Context) map[string]int {
	realtimeWeight := g.Cfg().MustGet(ctx, "youbanPublish.queue.mediaRealtimeWeight", 32).Int()
	bulkWeight := g.Cfg().MustGet(ctx, "youbanPublish.queue.mediaBulkWeight", 1).Int()
	legacyWeight := g.Cfg().MustGet(ctx, "youbanPublish.queue.mediaLegacyWeight", 1).Int()
	if realtimeWeight < 1 {
		realtimeWeight = 1
	}
	if realtimeWeight > 100 {
		realtimeWeight = 100
	}
	if bulkWeight < 1 {
		bulkWeight = 1
	}
	if bulkWeight > 20 {
		bulkWeight = 20
	}
	if legacyWeight < 1 {
		legacyWeight = 1
	}
	if legacyWeight > 20 {
		legacyWeight = 20
	}
	queues := map[string]int{
		tgQueueNameMediaRealtime: realtimeWeight,
		tgQueueNameMedia:         legacyWeight,
	}
	for _, queue := range collectMediaBulkQueueNames() {
		queues[queue] = bulkWeight
	}
	return queues
}
