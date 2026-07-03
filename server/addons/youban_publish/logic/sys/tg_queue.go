package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
)

const (
	tgQueueNameDefault = "youban_publish_tg"
	tgTaskTypePublish  = "youban_publish:tg:publish"
	tgTaskTypeDelete   = "youban_publish:tg:delete"
)

type tgQueuePayload struct {
	JobId int64 `json:"jobId"`
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
	return s.enqueueTelegramTask(ctx, tgTaskTypePublish, jobId, delay)
}

func (s *sSysPublish) enqueueTelegramDeleteJob(ctx context.Context, jobId int64, delay time.Duration) error {
	return s.enqueueTelegramTask(ctx, tgTaskTypeDelete, jobId, delay)
}

func (s *sSysPublish) enqueueTelegramTask(ctx context.Context, taskType string, jobId int64, delay time.Duration) error {
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
	options := []asynq.Option{
		asynq.Queue(tgQueueNameDefault),
		asynq.MaxRetry(10),
		asynq.Timeout(5 * time.Minute),
		asynq.Unique(30 * time.Second),
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
