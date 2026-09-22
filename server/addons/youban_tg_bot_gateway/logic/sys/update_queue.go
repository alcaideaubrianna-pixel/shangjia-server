package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"hotgo/addons/youban_tg_bot_gateway/service"
)

const (
	tgGatewayQueueName      = "youban_tg_bot_gateway_update"
	tgGatewayTaskType       = "youban_tg_bot_gateway:update"
	tgGatewayEnqueueTimeout = 5 * time.Second
)

type gatewayUpdatePayload struct {
	Key  string `json:"key"`
	Body []byte `json:"body"`
}

func (s *sGateway) startUpdateQueue(ctx context.Context) {
	s.queueMu.Lock()
	if s.queue != nil {
		s.queueMu.Unlock()
		return
	}
	redisOpt := gatewayRedisOption(ctx)
	server := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: gatewayQueueConcurrency(ctx),
		Queues:      map[string]int{tgGatewayQueueName: 1},
	})
	if s.queueCli == nil {
		s.queueCli = asynq.NewClient(redisOpt)
	}
	mux := asynq.NewServeMux()
	mux.HandleFunc(tgGatewayTaskType, s.handleUpdateTask)
	s.queue = server
	s.queueMu.Unlock()
	g.Log().Infof(ctx, "启动TG Bot Gateway异步更新队列 concurrency:%d", gatewayQueueConcurrency(ctx))
	go func() {
		if err := server.Run(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "TG Bot Gateway异步更新队列停止：%+v", err)
		}
	}()
}

func gatewayRedisOption(ctx context.Context) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr:     g.Cfg().MustGet(ctx, "redis.default.address", "127.0.0.1:6379").String(),
		Password: g.Cfg().MustGet(ctx, "redis.default.pass", "").String(),
		DB:       g.Cfg().MustGet(ctx, "redis.default.db", 0).Int(),
	}
}

func (s *sGateway) updateQueueClient(ctx context.Context) *asynq.Client {
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	if s.queueCli == nil {
		s.queueCli = asynq.NewClient(gatewayRedisOption(ctx))
	}
	return s.queueCli
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
	client := s.updateQueueClient(ctx)
	enqueueCtx, cancel := context.WithTimeout(ctx, tgGatewayEnqueueTimeout)
	defer cancel()
	_, err = client.EnqueueContext(enqueueCtx, asynq.NewTask(tgGatewayTaskType, payload),
		asynq.Queue(tgGatewayQueueName),
		asynq.MaxRetry(10),
		asynq.Timeout(2*time.Minute),
		asynq.Unique(5*time.Minute),
	)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		var update models.Update
		_ = json.Unmarshal(body, &update)
		s.observeDuplicateUpdate(ctx, key, update.ID)
		return nil
	}
	return err
}

func (s *sGateway) observeDuplicateUpdate(ctx context.Context, key string, updateID int64) {
	s.mu.Lock()
	bindings := append([]service.BotBinding(nil), s.bindings[key]...)
	s.mu.Unlock()
	owners := make(map[string]struct{})
	references := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		owners[binding.Owner] = struct{}{}
		references = append(references, binding.Owner+":"+strconv.FormatInt(binding.ReferenceID, 10))
	}
	if len(owners) == 0 {
		owners["unknown"] = struct{}{}
	}
	counter, _ := gatewayObserveMeter.Int64Counter("xiaohuiji.tg.gateway_duplicate_updates")
	for owner := range owners {
		counter.Add(ctx, 1, metric.WithAttributes(attribute.String("owner", owner)))
	}
	g.Log().Warningf(ctx, "TG Bot Gateway重复Update已拦截 updateId:%d bindings:%s", updateID, strings.Join(references, ","))
}

func (s *sGateway) handleUpdateTask(ctx context.Context, task *asynq.Task) error {
	startedAt := time.Now()
	var payload gatewayUpdatePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("解析Telegram队列任务失败: %w", err)
	}
	var update models.Update
	if err := json.Unmarshal(payload.Body, &update); err != nil {
		return fmt.Errorf("解析Telegram更新失败: %w", err)
	}
	if err := s.dispatch(ctx, payload.Key, &update); err != nil {
		g.Log().Warningf(ctx, "TG Bot Gateway分发失败 key:%s updateId:%d duration:%s err:%+v", payload.Key, update.ID, time.Since(startedAt), err)
		return err
	}
	g.Log().Infof(ctx, "TG链路 gateway_task_complete key:%s updateId:%d duration:%s", payload.Key, update.ID, time.Since(startedAt))
	return nil
}
