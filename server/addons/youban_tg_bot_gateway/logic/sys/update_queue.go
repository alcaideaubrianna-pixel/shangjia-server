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
	"hotgo/utility/scanconfig"
)

const (
	tgGatewayInteractiveQueueName = "youban_tg_bot_gateway_interactive"
	tgGatewayBulkQueueName        = "youban_tg_bot_gateway_bulk"
	tgGatewayLegacyQueueName      = "youban_tg_bot_gateway_update"
	tgGatewayTaskType             = "youban_tg_bot_gateway:update"
	tgGatewayEnqueueTimeout       = 5 * time.Second
)

type gatewayUpdatePayload struct {
	Key        string `json:"key"`
	Body       []byte `json:"body"`
	EnqueuedAt int64  `json:"enqueuedAt,omitempty"`
}

func (s *sGateway) startUpdateQueue(ctx context.Context) {
	s.queueMu.Lock()
	if s.interactiveQueue != nil || s.bulkQueue != nil {
		s.queueMu.Unlock()
		return
	}
	redisOpt := gatewayRedisOption(ctx)
	interactiveServer := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: gatewayInteractiveQueueConcurrency(ctx),
		Queues:      map[string]int{tgGatewayInteractiveQueueName: 1},
	})
	bulkServer := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: gatewayBulkQueueConcurrency(ctx),
		Queues: map[string]int{
			tgGatewayBulkQueueName:   10,
			tgGatewayLegacyQueueName: 1,
		},
	})
	if s.queueCli == nil {
		s.queueCli = asynq.NewClient(redisOpt)
	}
	mux := asynq.NewServeMux()
	mux.HandleFunc(tgGatewayTaskType, s.handleUpdateTask)
	s.interactiveQueue = interactiveServer
	s.bulkQueue = bulkServer
	s.queueMu.Unlock()
	g.Log().Infof(ctx, "启动TG Bot Gateway异步更新队列 interactiveConcurrency:%d bulkConcurrency:%d", gatewayInteractiveQueueConcurrency(ctx), gatewayBulkQueueConcurrency(ctx))
	go func() {
		if err := interactiveServer.Run(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "TG Bot Gateway交互更新队列停止：%+v", err)
		}
	}()
	go func() {
		if err := bulkServer.Run(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "TG Bot Gateway批量更新队列停止：%+v", err)
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

func gatewayBulkQueueConcurrency(ctx context.Context) int {
	concurrency := g.Cfg().MustGet(ctx, "youbanTgBotGateway.queue.concurrency", 8).Int()
	if concurrency < 1 {
		return 1
	}
	return concurrency
}

func gatewayInteractiveQueueConcurrency(ctx context.Context) int {
	concurrency := g.Cfg().MustGet(ctx, "youbanTgBotGateway.queue.interactiveConcurrency", 16).Int()
	if concurrency < 1 {
		return 1
	}
	return concurrency
}

func (s *sGateway) stopUpdateQueue() {
	s.queueMu.Lock()
	interactiveServer, bulkServer, client := s.interactiveQueue, s.bulkQueue, s.queueCli
	s.interactiveQueue, s.bulkQueue, s.queueCli = nil, nil, nil
	s.queueMu.Unlock()
	if interactiveServer != nil {
		interactiveServer.Shutdown()
	}
	if bulkServer != nil {
		bulkServer.Shutdown()
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
	enqueuedAt := time.Now()
	payload, err := json.Marshal(gatewayUpdatePayload{Key: key, Body: body, EnqueuedAt: enqueuedAt.UnixNano()})
	if err != nil {
		return fmt.Errorf("序列化Telegram队列任务失败: %w", err)
	}
	client := s.updateQueueClient(ctx)
	enqueueCtx, cancel := context.WithTimeout(ctx, tgGatewayEnqueueTimeout)
	defer cancel()
	queueName := tgGatewayBulkQueueName
	taskTimeout := 2 * time.Minute
	var update models.Update
	if unmarshalErr := json.Unmarshal(body, &update); unmarshalErr == nil && updateNeedsInteractiveQueue(&update) {
		queueName = tgGatewayInteractiveQueueName
		// Interactive updates must not occupy a worker while a downstream
		// operation is stuck. A short lease keeps private chat and callbacks
		// responsive; the task is retried by Asynq when the handler times out.
		taskTimeout = 20 * time.Second
		taskTimeout = gatewayMediaSearchTaskTimeout(ctx, &update, taskTimeout)
	}
	_, err = client.EnqueueContext(enqueueCtx, asynq.NewTask(tgGatewayTaskType, payload),
		asynq.Queue(queueName),
		asynq.MaxRetry(10),
		asynq.Timeout(taskTimeout),
		asynq.Unique(5*time.Minute),
	)
	if err == nil {
		g.Log().Infof(ctx, "TG链路 gateway_interactive_enqueue updateId:%d queue:%s enqueue:%s", update.ID, queueName, time.Since(enqueuedAt))
	}
	if errors.Is(err, asynq.ErrDuplicateTask) {
		var update models.Update
		_ = json.Unmarshal(body, &update)
		s.observeDuplicateUpdate(ctx, key, update.ID)
		return nil
	}
	return err
}

// Only private photo/video updates can enter media scan; keep text, callbacks
// and channel ingestion on their existing short leases.
func gatewayMediaSearchTaskTimeout(ctx context.Context, update *models.Update, fallback time.Duration) time.Duration {
	if update == nil {
		return fallback
	}
	message := update.Message
	if message == nil {
		message = update.EditedMessage
	}
	if message == nil || message.Chat.Type != "private" || (len(message.Photo) == 0 && message.Video == nil) {
		return fallback
	}
	if budget := scanconfig.QueryTimeout(ctx); budget > fallback {
		return scanconfig.SearchTimeout(ctx) + 30*time.Second
	}
	return fallback
}

func updateNeedsInteractiveQueue(update *models.Update) bool {
	if update == nil {
		return false
	}
	if update.CallbackQuery != nil || update.InlineQuery != nil {
		return true
	}
	for _, message := range []*models.Message{update.Message, update.EditedMessage} {
		if message != nil && message.Chat.Type == "private" {
			return true
		}
	}
	return false
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
	queueWait := time.Duration(0)
	if payload.EnqueuedAt > 0 {
		queueWait = time.Since(time.Unix(0, payload.EnqueuedAt))
	}
	g.Log().Infof(ctx, "TG链路 gateway_interactive_start updateId:%d queueWait:%s", update.ID, queueWait)
	if err := s.dispatch(ctx, payload.Key, &update); err != nil {
		g.Log().Warningf(ctx, "TG Bot Gateway分发失败 key:%s updateId:%d queueWait:%s duration:%s err:%+v", payload.Key, update.ID, queueWait, time.Since(startedAt), err)
		return err
	}
	g.Log().Infof(ctx, "TG链路 gateway_interactive_complete key:%s updateId:%d queueWait:%s handler:%s total:%s", payload.Key, update.ID, queueWait, time.Since(startedAt), queueWait+time.Since(startedAt))
	return nil
}
