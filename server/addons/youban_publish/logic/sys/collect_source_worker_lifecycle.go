package sys

import (
	"context"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"

	"hotgo/internal/library/cache"
	"hotgo/utility/ownedlease"
)

type collectSourceWorkerControl struct {
	cancel context.CancelFunc
	done   chan struct{}
}

var collectSourceWorkerControls sync.Map

func stopCollectSourceQueueWorker(server *asynq.Server) {
	if value, ok := collectSourceWorkerControls.Load(server); ok {
		control := value.(*collectSourceWorkerControl)
		control.cancel()
		<-control.done
		return
	}
	server.Shutdown()
}

// collectSourceQueueHasWork uses only constant-time cardinalities. Completed
// and archived history must not keep an otherwise idle consumer alive.
func collectSourceQueueHasWork(ctx context.Context, queueName string) (bool, error) {
	return cache.AsynqQueueHasWork(ctx, queueName)
}

func (s *sSysPublish) ensureCollectSourceQueueWorker(ctx context.Context, queueName string) {
	s.tgQueueMu.Lock()
	_, exists := s.collectSourceQueueServers[queueName]
	stopped := s.collectSourceQueueServers == nil
	s.tgQueueMu.Unlock()
	if exists || stopped {
		return
	}
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	hasWork, err := collectSourceQueueHasWork(checkCtx, queueName)
	cancel()
	if err != nil || !hasWork {
		return
	}
	ttl := time.Duration(collectSourceLeaseSeconds(ctx)) * time.Second
	lease, err := cache.NewQueueLease("youban_publish:collect:source:lease:"+queueName, "collect_source_worker", ttl)
	if err != nil {
		return
	}
	claimCtx, cancelClaim := context.WithTimeout(ctx, 2*time.Second)
	claimed, err := lease.TryAcquire(claimCtx)
	cancelClaim()
	if err != nil || !claimed {
		return
	}
	execution, cancelExecution := context.WithCancel(ctx)
	server := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency: collectSourceWorkerConcurrency(ctx), Queues: map[string]int{queueName: 1},
		RetryDelayFunc:  telegramQueueRetryDelay,
		BaseContext:     func() context.Context { return execution },
		ShutdownTimeout: 5 * time.Second,
	})
	s.tgQueueMu.Lock()
	_, exists = s.collectSourceQueueServers[queueName]
	if exists || s.collectSourceQueueServers == nil || ctx.Err() != nil {
		s.tgQueueMu.Unlock()
		cancelExecution()
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = lease.Unlock(releaseCtx)
		releaseCancel()
		return
	}
	collectSourceWorkerControls.Store(server, &collectSourceWorkerControl{cancel: cancelExecution, done: make(chan struct{})})
	s.collectSourceQueueServers[queueName] = server
	s.tgQueueMu.Unlock()
	mux := asynq.NewServeMux()
	mux.HandleFunc(tgTaskTypeCollectProcess, s.handleCollectProcessTask)
	go s.runCollectSourceQueueWorker(execution, cancelExecution, queueName, server, mux, lease, ttl/3)
}

// Start does not register process-wide signal handlers. The collector lifecycle
// and ownership monitor own shutdown, including failed startup and lost leases.
func (s *sSysPublish) runCollectSourceQueueWorker(ctx context.Context, cancel context.CancelFunc, queueName string, server *asynq.Server, mux *asynq.ServeMux, lease *ownedlease.Lease, interval time.Duration) {
	defer func() {
		cancel()
		server.Shutdown()
		releaseCtx, releaseCancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = lease.Unlock(releaseCtx)
		releaseCancel()
		s.tgQueueMu.Lock()
		if s.collectSourceQueueServers[queueName] == server {
			delete(s.collectSourceQueueServers, queueName)
		}
		s.tgQueueMu.Unlock()
		if value, ok := collectSourceWorkerControls.LoadAndDelete(server); ok {
			close(value.(*collectSourceWorkerControl).done)
		}
	}()
	if err := server.Start(mux); err != nil {
		g.Log().Warningf(ctx, "采集来源消费者启动失败 queue:%s err:%+v", queueName, err)
		return
	}
	waitCollectSourceQueueWorker(ctx, interval, 30*time.Second, lease.Renew, func(checkCtx context.Context) (bool, error) {
		return collectSourceQueueHasWork(checkCtx, queueName)
	})
}
