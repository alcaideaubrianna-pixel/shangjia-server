package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

type collectMediaQueuePayload struct {
	EventId     int64 `json:"eventId"`
	TenantId    int64 `json:"tenantId"`
	AccountId   int64 `json:"accountId"`
	SourceId    int64 `json:"sourceId"`
	TgAccountId int64 `json:"tgAccountId"`
}

const collectMediaTaskUniqueTTL = 30 * time.Minute

func (s *sSysPublish) enqueueCollectMediaCache(ctx context.Context, payload collectMediaQueuePayload, delay time.Duration) error {
	if payload.EventId <= 0 || payload.TenantId <= 0 || payload.AccountId <= 0 {
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
	task := asynq.NewTask(tgTaskTypeCollectMedia, body)
	options := []asynq.Option{
		asynq.Queue(collectSourceQueueName(tgQueueNameCollectMedia, payload.SourceId)),
		asynq.Unique(collectMediaTaskUniqueTTL),
		asynq.MaxRetry(10),
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
