package sys

import (
	"context"
	"encoding/json"
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

const collectProcessTaskUniqueTTL = 10 * time.Second

func (s *sSysPublish) enqueueCollectProcess(ctx context.Context, payload collectProcessQueuePayload, _ time.Duration) error {
	if payload.TenantId <= 0 || payload.AccountId <= 0 || payload.SourceId <= 0 {
		return nil
	}
	if s == nil || s.collectProcessRefresh == nil {
		return nil
	}
	select {
	case s.collectProcessRefresh <- struct{}{}:
	default:
	}
	return nil
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
