package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
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
		delay = collectOrderRetryDelay
	}
	return &collectProcessRetryError{delay: delay, message: message}
}

type collectProcessQueuePayload struct {
	AccountId int64 `json:"accountId"`
	EventId   int64 `json:"eventId"`
	SourceId  int64 `json:"sourceId"`
	TenantId  int64 `json:"tenantId"`
}

const collectProcessTaskUniqueTTL = 10 * time.Minute

func (s *sSysPublish) enqueueCollectProcess(ctx context.Context, payload collectProcessQueuePayload, delay time.Duration) error {
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
	task := asynq.NewTask(tgTaskTypeCollectProcess, body)
	options := []asynq.Option{
		asynq.Queue(collectSourceQueueName(tgQueueNameCollect, payload.SourceId)),
		asynq.Unique(collectProcessTaskUniqueTTL),
		asynq.MaxRetry(10),
		asynq.Timeout(10 * time.Minute),
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

func decodeCollectProcessQueuePayload(task *asynq.Task) (collectProcessQueuePayload, error) {
	var payload collectProcessQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析采集事件处理任务失败: %w", err)
	}
	if payload.EventId <= 0 {
		return payload, fmt.Errorf("采集事件处理任务缺少eventId")
	}
	return payload, nil
}
