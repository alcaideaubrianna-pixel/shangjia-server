package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

func (s *sSysPublish) enqueueCollectSourceDelete(ctx context.Context, payload collectSourceDeleteQueuePayload) error {
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
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeCollectSourceDelete, body),
		asynq.Queue(tgQueueNameBackground),
		asynq.MaxRetry(10),
		asynq.Timeout(30*time.Minute),
		asynq.Unique(5*time.Minute),
	)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func decodeCollectSourceDeleteQueuePayload(task *asynq.Task) (collectSourceDeleteQueuePayload, error) {
	var payload collectSourceDeleteQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析采集源删除清理任务失败: %w", err)
	}
	if payload.SourceId <= 0 || payload.TenantId <= 0 || payload.AccountId <= 0 {
		return payload, fmt.Errorf("采集源删除清理任务参数不完整")
	}
	return payload, nil
}
