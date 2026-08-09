package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
)

const (
	tgGatewayQueueName = "youban_tg_bot_gateway_update"
	tgGatewayTaskType  = "youban_tg_bot_gateway:update"
)

type gatewayUpdatePayload struct {
	Key  string `json:"key"`
	Body []byte `json:"body"`
}

func (s *sGateway) startUpdateQueue(ctx context.Context) {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	if s.queue != nil || s.queueCli != nil {
		return
	}
	redisOpt := asynq.RedisClientOpt{
		Addr:     g.Cfg().MustGet(ctx, "redis.default.address", "127.0.0.1:6379").String(),
		Password: g.Cfg().MustGet(ctx, "redis.default.pass", "").String(),
		DB:       g.Cfg().MustGet(ctx, "redis.default.db", 0).Int(),
	}
	server := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: gatewayQueueConcurrency(ctx),
		Queues:      map[string]int{tgGatewayQueueName: 1},
	})
	client := asynq.NewClient(redisOpt)
	mux := asynq.NewServeMux()
	mux.HandleFunc(tgGatewayTaskType, s.handleUpdateTask)
	s.queue, s.queueCli = server, client
	g.Log().Infof(ctx, "启动TG Bot Gateway异步更新队列 concurrency:%d", gatewayQueueConcurrency(ctx))
	go func() {
		if err := server.Run(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "TG Bot Gateway异步更新队列停止：%+v", err)
		}
	}()
}

func gatewayQueueConcurrency(ctx context.Context) int {
	concurrency := g.Cfg().MustGet(ctx, "youbanTgBotGateway.queue.concurrency", 8).Int()
	if concurrency < 1 {
		return 1
	}
	return concurrency
}

func (s *sGateway) stopUpdateQueue() {
	s.queueMu.Lock()
	server, client := s.queue, s.queueCli
	s.queue, s.queueCli = nil, nil
	s.queueMu.Unlock()
	if server != nil {
		server.Shutdown()
	}
	if client != nil {
		_ = client.Close()
	}
}

func (s *sGateway) enqueueUpdate(ctx context.Context, key string, update *models.Update) error {
	body, err := json.Marshal(update)
	if err != nil {
		return fmt.Errorf("序列化Telegram更新失败: %w", err)
	}
	return s.enqueueUpdateBody(ctx, key, body)
}

func (s *sGateway) enqueueUpdateBody(ctx context.Context, key string, body []byte) error {
	if key == "" {
		return fmt.Errorf("Telegram Bot Key为空")
	}
	if len(body) == 0 {
		return fmt.Errorf("Telegram更新内容为空")
	}
	payload, err := json.Marshal(gatewayUpdatePayload{Key: key, Body: body})
	if err != nil {
		return fmt.Errorf("序列化Telegram队列任务失败: %w", err)
	}
	s.queueMu.Lock()
	client := s.queueCli
	s.queueMu.Unlock()
	if client == nil {
		return fmt.Errorf("TG Bot Gateway异步队列未启动")
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgGatewayTaskType, payload),
		asynq.Queue(tgGatewayQueueName),
		asynq.MaxRetry(10),
		asynq.Timeout(2*time.Minute),
		asynq.Unique(5*time.Minute),
	)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

func (s *sGateway) handleUpdateTask(ctx context.Context, task *asynq.Task) error {
	var payload gatewayUpdatePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("解析Telegram队列任务失败: %w", err)
	}
	var update models.Update
	if err := json.Unmarshal(payload.Body, &update); err != nil {
		return fmt.Errorf("解析Telegram更新失败: %w", err)
	}
	if err := s.dispatch(ctx, payload.Key, &update); err != nil {
		g.Log().Warningf(ctx, "TG Bot Gateway分发失败 key:%s err:%+v", payload.Key, err)
		return err
	}
	return nil
}
