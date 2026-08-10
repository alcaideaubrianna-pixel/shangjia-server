package sys

import (
	"context"
	"errors"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
)

func (s *sSysPublish) startTelegramPushWorker(ctx context.Context) {
	s.tgQueueMu.Lock()
	if s.tgQueueServer != nil {
		s.tgQueueMu.Unlock()
		return
	}
	server := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    g.Cfg().MustGet(ctx, "youbanPublish.queue.concurrency", 20).Int(),
		Queues:         telegramPublishQueueWeights(ctx),
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	g.Log().Info(ctx, "启动上架插件TG推送队列")
	s.tgQueueServer = server
	s.tgQueueMu.Unlock()

	mux := asynq.NewServeMux()
	mux.HandleFunc(tgTaskTypePublish, s.handleTelegramPublishTask)
	mux.HandleFunc(tgTaskTypeCleanup, s.handleTelegramCleanupTask)
	mux.HandleFunc(tgTaskTypeDown, s.handleProfileDownTask)

	go func() {
		if err := server.Run(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动上架插件TG发送队列失败：%+v", err)
		}
	}()
}

func (s *sSysPublish) startTelegramBackgroundWorker(ctx context.Context) {
	s.tgQueueMu.Lock()
	if s.backgroundQueueServer != nil {
		s.tgQueueMu.Unlock()
		return
	}
	server := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    g.Cfg().MustGet(ctx, "youbanPublish.queue.backgroundConcurrency", 4).Int(),
		Queues:         map[string]int{tgQueueNameBackground: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	g.Log().Info(ctx, "启动上架插件后台队列")
	s.backgroundQueueServer = server
	s.tgQueueMu.Unlock()

	backgroundMux := asynq.NewServeMux()
	backgroundMux.HandleFunc(tgTaskTypeImport, s.handleImportTask)
	backgroundMux.HandleFunc(tgTaskTypeRepair, s.handleTgMessageRepairTask)
	backgroundMux.HandleFunc(tgTaskTypeImportMatch, s.handleImportMatchTask)
	backgroundMux.HandleFunc(tgTaskTypeImportSync, s.handleImportTgSyncTask)
	backgroundMux.HandleFunc(tgTaskTypeMaterialImport, s.handleMaterialImportTask)
	backgroundMux.HandleFunc(tgTaskTypeDown, s.handleProfileDownTask)
	backgroundMux.HandleFunc(tgTaskTypeCycleRun, s.handleCycleRunTask)
	backgroundMux.HandleFunc(tgTaskTypeCycleReschedule, s.handleCycleRescheduleTask)
	backgroundMux.HandleFunc(tgTaskTypeCycleRefresh, s.handleCycleRefreshTask)
	backgroundMux.HandleFunc(tgTaskTypeCollectHistory, s.handleCollectHistoryTask)
	backgroundMux.HandleFunc(tgTaskTypeCollectProcess, s.handleCollectProcessTask)
	backgroundMux.HandleFunc(tgTaskTypeCollectTrigger, s.handleCollectTriggerTask)
	backgroundMux.HandleFunc(tgTaskTypeChannelMemberSync, s.handleChannelMemberSyncTask)
	backgroundMux.HandleFunc(tgTaskTypeCollectSourceDown, s.handleCollectSourceDownTask)
	go func() {
		if err := server.Run(backgroundMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动上架插件后台队列失败：%+v", err)
		}
	}()
}

func (s *sSysPublish) startTelegramMediaWorker(ctx context.Context) {
	s.tgQueueMu.Lock()
	if s.mediaQueueServer != nil {
		s.tgQueueMu.Unlock()
		return
	}
	server := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    collectMediaQueueConcurrency(ctx),
		Queues:         collectMediaWorkerQueues(ctx),
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	g.Log().Info(ctx, "启动上架插件采集媒体队列；按实时、历史账号分片和旧队列公平消费")
	s.mediaQueueServer = server
	s.tgQueueMu.Unlock()
	mediaMux := asynq.NewServeMux()
	mediaMux.HandleFunc(tgTaskTypeCollectMedia, s.handleCollectMediaCacheTask)
	mediaMux.HandleFunc(tgTaskTypeMediaProcess, s.handleMediaProcessTask)
	go s.recoverMediaProcessTasks(ctx)
	go func() {
		if err := server.Run(mediaMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动上架插件媒体缓存队列失败：%+v", err)
		}
	}()
}

func (s *sSysPublish) stopTelegramQueueWorker() {
	s.tgQueueMu.Lock()
	server := s.tgQueueServer
	mediaServer := s.mediaQueueServer
	backgroundServer := s.backgroundQueueServer
	client := s.tgQueueClient
	s.tgQueueServer = nil
	s.mediaQueueServer = nil
	s.backgroundQueueServer = nil
	s.tgQueueClient = nil
	s.tgQueueMu.Unlock()
	if server != nil {
		server.Shutdown()
	}
	if mediaServer != nil {
		mediaServer.Shutdown()
	}
	if backgroundServer != nil {
		backgroundServer.Shutdown()
	}
	if client != nil {
		_ = client.Close()
	}
}

func (s *sSysPublish) handleTelegramPublishTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeTelegramQueuePayload(task)
	if err != nil {
		return err
	}
	if delay, enabled := s.telegramPublishWindowDelay(ctx); enabled && delay > 0 {
		return &tgRetryAfterError{after: delay, err: errTelegramPublishWindowBlocked}
	}
	return s.SendTelegramJob(ctx, payload.JobId)
}

func (s *sSysPublish) handleTelegramCleanupTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeTelegramQueuePayload(task)
	if err != nil {
		return err
	}
	return s.CleanupTelegramJobMessages(ctx, payload.JobId)
}

func (s *sSysPublish) handleCycleRescheduleTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeCycleRescheduleQueuePayload(task)
	if err != nil {
		return err
	}
	return s.rescheduleChannelProfileCycles(ctx, payload.ChannelId)
}

func (s *sSysPublish) handleCycleRefreshTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeCycleRefreshQueuePayload(task)
	if err != nil {
		return err
	}
	return s.refreshChannelProfileCycleNextAt(ctx, payload.ChannelId)
}

func (s *sSysPublish) handleImportTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeImportQueuePayload(task)
	if err != nil {
		return err
	}
	if payload.RunId > 0 {
		return s.ExecuteImportRun(ctx, payload.RunId)
	}
	return s.ExecuteImportTask(ctx, payload.Id)
}

func (s *sSysPublish) handleTgMessageRepairTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeTgMessageRepairQueuePayload(task)
	if err != nil {
		return err
	}
	return s.ExecuteTgMessageRepairRun(ctx, payload.RunId)
}

func (s *sSysPublish) handleCollectHistoryTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeCollectHistoryQueuePayload(task)
	if err != nil {
		return err
	}
	return s.ExecuteCollectHistoryTask(ctx, payload.TaskId)
}

func (s *sSysPublish) handleCollectTriggerTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeCollectTriggerQueuePayload(task)
	if err != nil {
		return err
	}
	_, err = s.ExecuteCollectSourceTrigger(ctx, payload.SourceId, payload.TenantId, payload.AccountId)
	return err
}

func (s *sSysPublish) handleCollectSourceDownTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeCollectSourceDownQueuePayload(task)
	if err != nil {
		return err
	}
	_, err = s.ExecuteCollectSourceDown(ctx, payload.SourceId, payload.TenantId, payload.AccountId)
	return err
}

func (s *sSysPublish) handleCollectProcessTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeCollectProcessQueuePayload(task)
	if err != nil {
		return err
	}
	delay, pending, err := s.processCollectSourceTask(ctx, payload)
	if err != nil || !pending {
		if err == nil {
			removeCollectProcessSchedule(ctx, payload)
		}
		return err
	}
	if delay < collectProcessMinimumDelay {
		delay = collectProcessMinimumDelay
	}
	enqueued, err := s.enqueueCollectProcessDeferred(ctx, payload, delay)
	if err != nil {
		return err
	}
	if enqueued {
		g.Log().Debugf(ctx, "采集源单批处理完成并重新排队 sourceId:%d delay:%s", payload.SourceId, delay.Round(time.Second))
	}
	return nil
}

func (s *sSysPublish) handleImportMatchTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeImportMatchQueuePayload(task)
	if err != nil {
		return err
	}
	return s.ExecuteImportRunMatch(ctx, payload.MatchRunId)
}

func (s *sSysPublish) handleImportTgSyncTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeImportMatchQueuePayload(task)
	if err != nil {
		return err
	}
	return s.ExecuteImportRunTgSync(ctx, payload.MatchRunId)
}

func (s *sSysPublish) handleMaterialImportTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeMaterialImportQueuePayload(task)
	if err != nil {
		return err
	}
	return s.ExecuteMaterialImportTask(ctx, payload.TaskId)
}

func (s *sSysPublish) handleChannelMemberSyncTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeChannelMemberSyncQueuePayload(task)
	if err != nil {
		return err
	}
	return s.ExecuteChannelMemberSyncTask(ctx, payload.TaskId)
}

func (s *sSysPublish) handleProfileDownTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeProfileDownQueuePayload(task)
	if err != nil {
		return err
	}
	return s.handleProfilesDown(ctx, payload.ProfileIds, payload.TenantId, payload.DownAt, payload.OperationNo)
}

func (s *sSysPublish) handleCycleRunTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeCycleRunQueuePayload(task)
	if err != nil {
		return err
	}
	return s.ExecuteCycleRun(ctx, payload.RunId)
}

func (s *sSysPublish) handleCollectMediaCacheTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeCollectMediaQueuePayload(task)
	if err != nil {
		return err
	}
	err = s.ExecuteCollectMediaCache(ctx, payload)
	if retryErr := collectMediaRetryErrorFrom(err); retryErr != nil && retryErr.deferWithoutFailure {
		enqueued, enqueueErr := s.enqueueCollectMediaCacheDeferred(ctx, payload, retryErr.delay)
		if enqueueErr != nil {
			return enqueueErr
		}
		if enqueued {
			g.Log().Debugf(ctx, "采集媒体任务因账号公平调度延迟重新投递 eventId:%d delay:%s", payload.EventId, retryErr.delay)
		}
		return nil
	}
	return err
}
