package sys

import (
	"context"
	"errors"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"

	pdao "hotgo/addons/youban_publish/internal/dao"
)

type collectSourceQueueWorker struct {
	process *asynq.Server
	media   *asynq.Server
}

func (w *collectSourceQueueWorker) shutdown() {
	if w == nil {
		return
	}
	if w.process != nil {
		w.process.Shutdown()
	}
	if w.media != nil {
		w.media.Shutdown()
	}
}

func (s *sSysPublish) runCollectSourceQueueSupervisor(ctx context.Context) {
	s.syncCollectSourceQueueWorkers(ctx)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncCollectSourceQueueWorkers(ctx)
		}
	}
}

func (s *sSysPublish) syncCollectSourceQueueWorkers(ctx context.Context) {
	rows, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Fields("id").
		Where("collect_enabled", 1).
		Where("status", 1).
		WhereNull("deleted_at").
		All()
	if err != nil {
		g.Log().Warningf(ctx, "读取采集源独立队列失败：%+v", err)
		return
	}
	s.tgQueueMu.Lock()
	defer s.tgQueueMu.Unlock()
	active := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		sourceId := row["id"].Int64()
		if sourceId <= 0 {
			continue
		}
		active[sourceId] = struct{}{}
		if _, ok := s.collectQueueWorkers[sourceId]; ok {
			continue
		}
		s.collectQueueWorkers[sourceId] = s.startCollectSourceQueueWorker(ctx, sourceId)
	}
	for sourceId, worker := range s.collectQueueWorkers {
		if _, ok := active[sourceId]; ok {
			continue
		}
		worker.shutdown()
		delete(s.collectQueueWorkers, sourceId)
		g.Log().Infof(ctx, "移除采集源独立队列 sourceId:%d", sourceId)
	}
}

func (s *sSysPublish) startCollectSourceQueueWorker(ctx context.Context, sourceId int64) *collectSourceQueueWorker {
	processQueue := collectSourceQueueName(tgQueueNameCollect, sourceId)
	mediaQueue := collectSourceQueueName(tgQueueNameCollectMedia, sourceId)
	processServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    1,
		Queues:         map[string]int{processQueue: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	mediaServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    1,
		Queues:         map[string]int{mediaQueue: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	processMux := asynq.NewServeMux()
	processMux.HandleFunc(tgTaskTypeCollectProcess, s.handleCollectProcessTask)
	mediaMux := asynq.NewServeMux()
	mediaMux.HandleFunc(tgTaskTypeCollectMedia, s.handleCollectMediaCacheTask)
	go func() {
		if err := processServer.Run(processMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动采集源处理队列失败 sourceId:%d err:%+v", sourceId, err)
		}
	}()
	go func() {
		if err := mediaServer.Run(mediaMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动采集源媒体队列失败 sourceId:%d err:%+v", sourceId, err)
		}
	}()
	g.Log().Infof(ctx, "启动采集源独立队列 sourceId:%d process:%s media:%s", sourceId, processQueue, mediaQueue)
	return &collectSourceQueueWorker{process: processServer, media: mediaServer}
}
