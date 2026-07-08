package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

func (s *sSysPublish) enqueueCollectHistoryTask(ctx context.Context, taskId int64, delay time.Duration) error {
	if taskId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(collectHistoryQueuePayload{TaskId: taskId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeCollectHistory, body)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameDefault),
		asynq.MaxRetry(0),
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

func decodeCollectHistoryQueuePayload(task *asynq.Task) (collectHistoryQueuePayload, error) {
	var payload collectHistoryQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析历史采集任务失败: %w", err)
	}
	if payload.TaskId <= 0 {
		return payload, fmt.Errorf("历史采集任务缺少taskId")
	}
	return payload, nil
}
