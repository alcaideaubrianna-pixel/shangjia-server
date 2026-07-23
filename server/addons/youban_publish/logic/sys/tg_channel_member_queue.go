package sys

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/hibiken/asynq"
)

type channelMemberSyncQueuePayload struct {
	TaskId int64 `json:"taskId"`
}

func (s *sSysPublish) enqueueChannelMemberSyncTask(ctx context.Context, taskId int64, delay time.Duration) error {
	if taskId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(channelMemberSyncQueuePayload{TaskId: taskId})
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeChannelMemberSync, body)
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

func decodeChannelMemberSyncQueuePayload(task *asynq.Task) (channelMemberSyncQueuePayload, error) {
	var payload channelMemberSyncQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, err
	}
	if payload.TaskId <= 0 {
		return payload, gerror.New("频道成员同步任务缺少taskId")
	}
	return payload, nil
}
