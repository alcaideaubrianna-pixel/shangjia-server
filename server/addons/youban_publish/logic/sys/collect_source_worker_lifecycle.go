package sys

import (
	"context"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
	"hotgo/internal/library/cache"
	"hotgo/utility/sourcequeue"
	"sync"
	"time"
)

// Only lifecycle state lives here. Work and ownership always live in Redis.
type collectSourceRuntimeControl struct {
	cancel context.CancelFunc
	done   chan struct{}
}

var collectSourceRuntimes sync.Map

func (s *sSysPublish) startCollectSourceRuntime(ctx context.Context) {
	s.tgQueueMu.Lock()
	if _, exists := collectSourceRuntimes.Load(s); exists {
		s.tgQueueMu.Unlock()
		return
	}
	execution, cancel := context.WithCancel(ctx)
	control := &collectSourceRuntimeControl{cancel: cancel, done: make(chan struct{})}
	collectSourceRuntimes.Store(s, control)
	s.tgQueueMu.Unlock()
	go func() {
		defer close(control.done)
		defer cancel()
		defer collectSourceRuntimes.CompareAndDelete(s, control)
		s.runCollectSourceRuntime(execution)
	}()
}

func (s *sSysPublish) stopCollectSourceRuntime() {
	s.tgQueueMu.Lock()
	value, ok := collectSourceRuntimes.Load(s)
	if ok {
		value.(*collectSourceRuntimeControl).cancel()
	}
	s.tgQueueMu.Unlock()
	if ok {
		<-value.(*collectSourceRuntimeControl).done
	}
}

func (s *sSysPublish) runCollectSourceRuntime(ctx context.Context) {
	var queue *sourcequeue.Queue
	for ctx.Err() == nil {
		var err error
		queue, err = sourcequeue.New(collectSourceRuntimeNamespace)
		if err == nil {
			break
		}
		g.Log().Warningf(ctx, "初始化采集来源队列失败，保留任务等待重连 err:%+v", err)
		if !waitCollectSourceRuntime(ctx, time.Second) {
			return
		}
	}
	if ctx.Err() != nil {
		return
	}
	var workers sync.WaitGroup
	for _, lane := range []sourcequeue.Lane{sourcequeue.Realtime, sourcequeue.History} {
		for i := 0; i < collectSourceRuntimeConcurrency(ctx, lane); i++ {
			workers.Add(1)
			go func(lane sourcequeue.Lane) { defer workers.Done(); s.runCollectSourceRuntimeWorker(ctx, queue, lane) }(lane)
		}
		workers.Add(1)
		go func(lane sourcequeue.Lane) { defer workers.Done(); runCollectSourceRuntimeRecovery(ctx, queue, lane) }(lane)
	}
	workers.Add(1)
	go func() { defer workers.Done(); s.runCollectSourceLegacyReader(ctx) }()
	g.Log().Info(ctx, "启动有界来源采集处理池", g.Map{
		"event": "collect.source.runtime.started", "namespace": collectSourceRuntimeNamespace,
		"realtimeConcurrency": collectSourceRuntimeConcurrency(ctx, sourcequeue.Realtime),
		"historyConcurrency":  collectSourceRuntimeConcurrency(ctx, sourcequeue.History),
	})
	workers.Wait()
}

// One bounded migration reader. No source enumeration, automatic backfill, or
// new producer writes to Asynq. Empty explicit configuration disables migration.
func (s *sSysPublish) runCollectSourceLegacyReader(ctx context.Context) {
	names := g.Cfg().MustGet(ctx, "youbanPublish.collect.sourceLegacyQueues", []string{}).Strings()
	queues, err := collectSourceLegacyQueues(names)
	if err != nil {
		g.Log().Warningf(ctx, "旧采集队列迁移配置无效 err:%+v", err)
		return
	}
	if len(queues) == 0 {
		return
	}
	server := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency: collectSourceRuntimeConfig(ctx, "youbanPublish.queue.collectSourceLegacyConcurrency", 1, 1, 4),
		Queues:      queues, RetryDelayFunc: telegramQueueRetryDelay,
		BaseContext: func() context.Context { return ctx }, ShutdownTimeout: 5 * time.Second,
	})
	mux := asynq.NewServeMux()
	mux.HandleFunc(tgTaskTypeCollectProcess, func(taskCtx context.Context, task *asynq.Task) error {
		payload, err := decodeCollectProcessQueuePayload(task)
		if err != nil {
			return err
		}
		// ACK old work only after reliable handoff into the history budget.
		payload.Recovery = true
		return enqueueCollectSourceProcess(taskCtx, payload, 0)
	})
	if ctx.Err() != nil {
		return
	}
	if err := server.Start(mux); err != nil {
		g.Log().Warningf(ctx, "旧采集队列迁移消费者启动失败 err:%+v", err)
		return
	}
	<-ctx.Done()
	server.Shutdown()
}

// Compatibility callers cannot allocate a source-specific server.
func (s *sSysPublish) ensureCollectSourceQueueWorker(ctx context.Context, _ string) {
	s.startCollectSourceRuntime(ctx)
}
func stopCollectSourceQueueWorker(server *asynq.Server) {
	if server != nil {
		server.Shutdown()
	}
}
func collectSourceQueueHasWork(ctx context.Context, queueName string) (bool, error) {
	return cache.AsynqQueueHasWork(ctx, queueName)
}
