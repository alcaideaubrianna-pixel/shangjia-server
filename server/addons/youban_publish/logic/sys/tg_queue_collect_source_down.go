package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

const tgTaskTypeCollectSourceDown = "youban_publish:collect:source_down"

type collectSourceDownQueuePayload struct {
	AccountId int64 `json:"accountId"`
	SourceId  int64 `json:"sourceId"`
	TenantId  int64 `json:"tenantId"`
}

func (s *sSysPublish) enqueueCollectSourceDown(ctx context.Context, payload collectSourceDownQueuePayload, delay time.Duration) error {
	if payload.SourceId <= 0 || payload.TenantId <= 0 || payload.AccountId <= 0 {
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
	task := asynq.NewTask(tgTaskTypeCollectSourceDown, body)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameBackground),
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

func decodeCollectSourceDownQueuePayload(task *asynq.Task) (collectSourceDownQueuePayload, error) {
	var payload collectSourceDownQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析采集源下架队列任务失败: %w", err)
	}
	if payload.SourceId <= 0 || payload.TenantId <= 0 || payload.AccountId <= 0 {
		return payload, fmt.Errorf("采集源下架队列任务缺少sourceId、tenantId或accountId")
	}
	return payload, nil
}
