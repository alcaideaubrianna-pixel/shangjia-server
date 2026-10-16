package sys

import (
	"context"
	"encoding/json"
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
		Queues:         telegramPublishForegroundQueueWeights(ctx),
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	bulkServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    telegramPublishBulkConcurrency(ctx),
		Queues:         telegramPublishBulkQueueWeights(ctx),
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	g.Log().Info(ctx, "启动上架插件TG推送队列")
	s.tgQueueServer = server
	s.tgBulkQueueServer = bulkServer
	s.tgQueueMu.Unlock()
	// The outbox dispatcher is part of the publish worker contract. Start it
	// from the same lifecycle entry point as the Asynq consumers so a role
	// parser or runtime branch cannot leave durable jobs stranded in pending.
	s.startTelegramOutboxDispatcher(ctx)

	mux := asynq.NewServeMux()
	mux.HandleFunc(tgTaskTypePublish, s.handleTelegramPublishTask)
	mux.HandleFunc(tgTaskTypeCleanup, s.handleTelegramCleanupTask)
	mux.HandleFunc(tgTaskTypeDown, s.handleProfileDownTask)

	go func() {
		if err := server.Run(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动上架插件TG发送队列失败：%+v", err)
		}
	}()
	go func() {
		if err := bulkServer.Run(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动上架插件TG批量发送队列失败：%+v", err)
		}
	}()
}

func (s *sSysPublish) startTelegramBackgroundWorker(ctx context.Context) {
	s.tgQueueMu.Lock()
	if s.autoDeleteQueueServer != nil || s.autoDeleteBackfillServer != nil || s.attemptTimeoutServer != nil || s.collectRealtimeServer != nil || s.backgroundQueueServer != nil || s.materialImportPullServer != nil || s.materialImportMediaServer != nil || s.cycleQueueServer != nil {
		s.tgQueueMu.Unlock()
		return
	}
	autoDeleteServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    telegramAutoDeleteConcurrency(ctx),
		Queues:         map[string]int{tgQueueNameAutoDeleteRealtime: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	autoDeleteBackfillServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    1,
		Queues:         map[string]int{tgQueueNameAutoDeleteBackfill: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	attemptTimeoutServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    4,
		Queues:         map[string]int{tgQueueNameAttemptTimeout: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	collectRealtimeServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    g.Cfg().MustGet(ctx, "youbanPublish.queue.collectRealtimeConcurrency", 16).Int(),
		Queues:         map[string]int{tgQueueNameCollectProcessRealtime: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	server := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    g.Cfg().MustGet(ctx, "youbanPublish.queue.backgroundConcurrency", 4).Int(),
		Queues:         collectProcessWorkerQueues(),
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	materialImportPullServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    g.Cfg().MustGet(ctx, "youbanPublish.queue.materialImportPullConcurrency", 8).Int(),
		Queues:         map[string]int{tgQueueNameMaterialImportPull: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	materialImportMediaServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    g.Cfg().MustGet(ctx, "youbanPublish.queue.materialImportMediaConcurrency", 4).Int(),
		Queues:         map[string]int{tgQueueNameMaterialImportMedia: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	cycleServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    normalizeCycleQueueConcurrency(g.Cfg().MustGet(ctx, "youbanPublish.queue.cycleConcurrency", 1).Int()),
		Queues:         map[string]int{tgQueueNameCycle: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	profileServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    g.Cfg().MustGet(ctx, "youbanPublish.queue.profileConcurrency", 4).Int(),
		Queues:         map[string]int{tgQueueNameProfileMaintenance: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	duplicateServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    g.Cfg().MustGet(ctx, "youbanPublish.queue.duplicateScanConcurrency", 1).Int(),
		Queues:         map[string]int{tgQueueNameDuplicateScan: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	historyServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    collectHistoryQueueConcurrency(ctx),
		Queues:         map[string]int{tgQueueNameHistory: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	mattingServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    normalizeAntiScanMattingConcurrency(g.Cfg().MustGet(ctx, "youbanPublish.queue.mattingConcurrency", 64).Int()),
		Queues:         map[string]int{tgQueueNameMatting: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	relayServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{Concurrency: g.Cfg().MustGet(ctx, "youbanPublish.queue.relayConcurrency", 4).Int(), Queues: map[string]int{tgQueueNameRelay: 1}, RetryDelayFunc: telegramQueueRetryDelay})
	g.Log().Infof(ctx, "启动上架插件后台队列，自动删除独立并发：%d", telegramAutoDeleteConcurrency(ctx))
	s.autoDeleteQueueServer = autoDeleteServer
	s.autoDeleteBackfillServer = autoDeleteBackfillServer
	s.attemptTimeoutServer = attemptTimeoutServer
	s.collectRealtimeServer = collectRealtimeServer
	s.backgroundQueueServer = server
	s.materialImportPullServer = materialImportPullServer
	s.materialImportMediaServer = materialImportMediaServer
	s.cycleQueueServer = cycleServer
	s.profileQueueServer = profileServer
	s.duplicateQueueServer = duplicateServer
	s.historyQueueServer = historyServer
	s.mattingQueueServer = mattingServer
	s.relayQueueServer = relayServer
	s.tgQueueMu.Unlock()

	backgroundMux := asynq.NewServeMux()
	backgroundMux.HandleFunc(tgTaskTypeImport, s.handleImportTask)
	backgroundMux.HandleFunc(tgTaskTypeRepair, s.handleTgMessageRepairTask)
	backgroundMux.HandleFunc(tgTaskTypeImportMatch, s.handleImportMatchTask)
	backgroundMux.HandleFunc(tgTaskTypeImportSync, s.handleImportTgSyncTask)
	backgroundMux.HandleFunc(tgTaskTypeMaterialImport, s.handleMaterialImportTask)
	backgroundMux.HandleFunc(tgTaskTypeCollectHistory, s.handleCollectHistoryTask)
	backgroundMux.HandleFunc(tgTaskTypeDown, s.handleProfileDownTask)
	backgroundMux.HandleFunc(tgTaskTypeCycleRun, s.handleCycleRunTask)
	backgroundMux.HandleFunc(tgTaskTypeCycleReschedule, s.handleCycleRescheduleTask)
	backgroundMux.HandleFunc(tgTaskTypeCycleRefresh, s.handleCycleRefreshTask)
	backgroundMux.HandleFunc(tgTaskTypeCollectProcess, s.handleCollectProcessTask)
	backgroundMux.HandleFunc(tgTaskTypeCollectTrigger, s.handleCollectTriggerTask)
	backgroundMux.HandleFunc(tgTaskTypeChannelMemberSync, s.handleChannelMemberSyncTask)
	backgroundMux.HandleFunc(tgTaskTypeCollectSourceDown, s.handleCollectSourceDownTask)
	backgroundMux.HandleFunc(tgTaskTypeCollectChannelPurge, s.handleCollectChannelPurgeTask)
	backgroundMux.HandleFunc(tgTaskTypeCollectSourceDelete, s.handleCollectSourceDeleteTask)
	backgroundMux.HandleFunc(tgTaskTypeBotMediaRepair, s.handleBotMediaRepairTask)
	backgroundMux.HandleFunc(tgTaskTypeAttemptTimeout, s.handleTelegramAttemptTimeoutTask)
	materialImportMux := asynq.NewServeMux()
	materialImportMux.HandleFunc(tgTaskTypeMaterialImport, s.handleMaterialImportTask)
	relayMux := asynq.NewServeMux()
	relayMux.HandleFunc(tgTaskTypeRelayMedia, s.handleRelayMediaTask)
	relayMux.HandleFunc(tgTaskTypeRelayMediaGroup, s.handleRelayMediaGroupTask)
	go func() {
		if err := relayServer.Run(relayMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动 relay 媒体队列失败：%+v", err)
		}
	}()
	cycleMux := asynq.NewServeMux()
	cycleMux.HandleFunc(tgTaskTypeCycleRun, s.handleCycleRunTask)
	cycleMux.HandleFunc(tgTaskTypeCycleReschedule, s.handleCycleRescheduleTask)
	cycleMux.HandleFunc(tgTaskTypeCycleRefresh, s.handleCycleRefreshTask)
	autoDeleteMux := asynq.NewServeMux()
	autoDeleteMux.HandleFunc(tgTaskTypeAutoDelete, s.handleTelegramAutoDeleteTask)
	autoDeleteBackfillMux := asynq.NewServeMux()
	autoDeleteBackfillMux.HandleFunc(tgTaskTypeAutoDelete, s.handleTelegramAutoDeleteTask)
	autoDeleteBackfillMux.HandleFunc(tgTaskTypeAutoDeleteBackfill, s.handleTelegramAutoDeleteBackfillTask)
	attemptTimeoutMux := asynq.NewServeMux()
	attemptTimeoutMux.HandleFunc(tgTaskTypeAttemptTimeout, s.handleTelegramAttemptTimeoutTask)
	collectRealtimeMux := asynq.NewServeMux()
	collectRealtimeMux.HandleFunc(tgTaskTypeCollectProcess, s.handleCollectProcessTask)
	profileMux := asynq.NewServeMux()
	profileMux.HandleFunc(tgTaskTypeProfileMaintenance, s.handleProfileMaintenanceTask)
	profileMux.HandleFunc(tgTaskTypeProfileSubmit, s.handleProfileSubmitTask)
	profileMux.HandleFunc(tgTaskTypePublishRecovery, s.handleTerminalPublishRecoveryTask)
	duplicateMux := asynq.NewServeMux()
	duplicateMux.HandleFunc(tgTaskTypeDuplicateScan, s.handleDuplicateScanTask)
	go func() {
		if err := autoDeleteServer.Run(autoDeleteMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动上架插件TG自动删除队列失败：%+v", err)
		}
	}()
	go func() {
		if err := autoDeleteBackfillServer.Run(autoDeleteBackfillMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动历史自动删除低优先级队列失败：%+v", err)
		}
	}()
	go func() {
		if err := attemptTimeoutServer.Run(attemptTimeoutMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动TG Attempt超时队列失败：%+v", err)
		}
	}()
	go func() {
		if err := collectRealtimeServer.Run(collectRealtimeMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动实时采集处理队列失败：%+v", err)
		}
	}()
	go func() {
		if err := server.Run(backgroundMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动上架插件后台队列失败：%+v", err)
		}
	}()
	go func() {
		if err := materialImportPullServer.Run(materialImportMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动资料导入拉取队列失败：%+v", err)
		}
	}()
	go func() {
		if err := materialImportMediaServer.Run(materialImportMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动资料导入媒体队列失败：%+v", err)
		}
	}()
	go func() {
		if err := cycleServer.Run(cycleMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动循环上架低优先级队列失败：%+v", err)
		}
	}()
	go func() {
		if err := profileServer.Run(profileMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动资料后台维护队列失败：%+v", err)
		}
	}()
	go func() {
		if err := duplicateServer.Run(duplicateMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动重复资料扫描队列失败：%+v", err)
		}
	}()
	historyMux := asynq.NewServeMux()
	historyMux.HandleFunc(tgTaskTypeCollectHistory, s.handleCollectHistoryTask)
	go func() {
		if err := historyServer.Run(historyMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动历史采集队列失败：%+v", err)
		}
	}()
	mattingMux := asynq.NewServeMux()
	mattingMux.HandleFunc(tgTaskTypeMatting, s.handleAntiScanMattingTask)
	go func() {
		if err := mattingServer.Run(mattingMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动人像抠图队列失败：%+v", err)
		}
	}()
}

func telegramAutoDeleteConcurrency(ctx context.Context) int {
	return normalizeTelegramAutoDeleteConcurrency(
		g.Cfg().MustGet(ctx, "youbanPublish.queue.autoDeleteConcurrency", 32).Int(),
	)
}

func normalizeTelegramAutoDeleteConcurrency(concurrency int) int {
	if concurrency < 1 {
		return 1
	}
	return concurrency
}

func normalizeCycleQueueConcurrency(concurrency int) int {
	if concurrency < 1 {
		return 1
	}
	if concurrency > 4 {
		return 4
	}
	return concurrency
}

func normalizeAntiScanMattingConcurrency(concurrency int) int {
	if concurrency < 1 {
		return 128
	}
	if concurrency > 800 {
		return 800
	}
	return concurrency
}

func (s *sSysPublish) startTelegramMediaWorker(ctx context.Context) {
	s.tgQueueMu.Lock()
	if s.mediaQueueServer != nil || s.mediaProcessServer != nil {
		s.tgQueueMu.Unlock()
		return
	}
	server := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    collectMediaQueueConcurrency(ctx),
		Queues:         collectMediaRealtimeWorkerQueues(),
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	bulkServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    collectMediaBulkConcurrency(ctx),
		Queues:         collectMediaBulkWorkerQueues(ctx),
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	processServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    mediaProcessConcurrency(ctx),
		Queues:         map[string]int{tgQueueNameMediaProcess: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	g.Log().Info(ctx, "启动上架插件媒体队列；采集下载与资料媒体处理隔离消费")
	s.mediaQueueServer = server
	s.mediaBulkQueueServer = bulkServer
	s.mediaProcessServer = processServer
	s.tgQueueMu.Unlock()
	mediaMux := asynq.NewServeMux()
	mediaMux.HandleFunc(tgTaskTypeCollectMedia, s.handleCollectMediaCacheTask)
	// Drain tasks enqueued to the legacy shared media queue before this version.
	mediaMux.HandleFunc(tgTaskTypeMediaProcess, s.handleMediaProcessTask)
	processMux := asynq.NewServeMux()
	processMux.HandleFunc(tgTaskTypeMediaProcess, s.handleMediaProcessTask)
	go s.recoverMediaProcessTasks(ctx)
	go func() {
		if err := server.Run(mediaMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动上架插件媒体缓存队列失败：%+v", err)
		}
	}()
	go func() {
		if err := bulkServer.Run(mediaMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动上架插件历史媒体队列失败：%+v", err)
		}
	}()
	go func() {
		if err := processServer.Run(processMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动资料媒体处理队列失败：%+v", err)
		}
	}()
}

func (s *sSysPublish) stopTelegramQueueWorker() {
	s.tgQueueMu.Lock()
	server := s.tgQueueServer
	bulkServer := s.tgBulkQueueServer
	mediaServer := s.mediaQueueServer
	mediaBulkServer := s.mediaBulkQueueServer
	mediaProcessServer := s.mediaProcessServer
	autoDeleteServer := s.autoDeleteQueueServer
	autoDeleteBackfillServer := s.autoDeleteBackfillServer
	attemptTimeoutServer := s.attemptTimeoutServer
	collectRealtimeServer := s.collectRealtimeServer
	backgroundServer := s.backgroundQueueServer
	materialImportPullServer := s.materialImportPullServer
	materialImportMediaServer := s.materialImportMediaServer
	cycleServer := s.cycleQueueServer
	profileServer := s.profileQueueServer
	duplicateServer := s.duplicateQueueServer
	historyServer := s.historyQueueServer
	mattingServer := s.mattingQueueServer
	client := s.tgQueueClient
	s.tgQueueServer = nil
	s.tgBulkQueueServer = nil
	s.mediaQueueServer = nil
	s.mediaBulkQueueServer = nil
	s.mediaProcessServer = nil
	s.autoDeleteQueueServer = nil
	s.autoDeleteBackfillServer = nil
	s.attemptTimeoutServer = nil
	s.collectRealtimeServer = nil
	s.backgroundQueueServer = nil
	s.materialImportPullServer = nil
	s.materialImportMediaServer = nil
	s.cycleQueueServer = nil
	s.profileQueueServer = nil
	s.duplicateQueueServer = nil
	s.historyQueueServer = nil
	s.mattingQueueServer = nil
	s.tgQueueClient = nil
	s.tgQueueMu.Unlock()
	if server != nil {
		server.Shutdown()
	}
	if bulkServer != nil {
		bulkServer.Shutdown()
	}
	if mediaServer != nil {
		mediaServer.Shutdown()
	}
	if mediaBulkServer != nil {
		mediaBulkServer.Shutdown()
	}
	if mediaProcessServer != nil {
		mediaProcessServer.Shutdown()
	}
	if autoDeleteServer != nil {
		autoDeleteServer.Shutdown()
	}
	if autoDeleteBackfillServer != nil {
		autoDeleteBackfillServer.Shutdown()
	}
	if attemptTimeoutServer != nil {
		attemptTimeoutServer.Shutdown()
	}
	if collectRealtimeServer != nil {
		collectRealtimeServer.Shutdown()
	}
	if backgroundServer != nil {
		backgroundServer.Shutdown()
	}
	if materialImportPullServer != nil {
		materialImportPullServer.Shutdown()
	}
	if materialImportMediaServer != nil {
		materialImportMediaServer.Shutdown()
	}
	if cycleServer != nil {
		cycleServer.Shutdown()
	}
	if profileServer != nil {
		profileServer.Shutdown()
	}
	if duplicateServer != nil {
		duplicateServer.Shutdown()
	}
	if historyServer != nil {
		historyServer.Shutdown()
	}
	if mattingServer != nil {
		mattingServer.Shutdown()
	}
	if client != nil {
		_ = client.Close()
	}
}

func (s *sSysPublish) handleTelegramPublishTask(ctx context.Context, task *asynq.Task) (err error) {
	ctx = telegramPublishTraceContext(ctx, 0, g.Map{"taskType": task.Type()})
	payload, err := decodeTelegramQueuePayload(task)
	if err != nil {
		beginTelegramPublishStage(ctx, "queue.decode_payload").End(err)
		return err
	}
	ctx = telegramPublishTraceContext(ctx, payload.JobId)
	stage := beginTelegramPublishStage(ctx, "queue.consume")
	defer stage.Finish(&err)
	jobStage := beginTelegramPublishStage(ctx, "queue.load_job")
	job, jobErr := s.telegramJobById(ctx, payload.JobId)
	jobStage.End(jobErr)
	if jobErr != nil {
		return jobErr
	}
	ctx = telegramPublishJobTraceContext(ctx, job)
	if delay, enabled := s.telegramPublishWindowDelay(ctx, job.TenantId, job.AccountId); enabled && delay > 0 {
		beginTelegramPublishStage(ctx, "queue.deferred").End(nil, g.Map{"reason": "publish_window", "delayMs": delay.Milliseconds()})
		return &tgRetryAfterError{after: delay, err: errTelegramPublishWindowBlocked}
	}
	return s.SendTelegramJob(ctx, payload.JobId)
}

func (s *sSysPublish) handleTelegramAttemptTimeoutTask(ctx context.Context, task *asynq.Task) error {
	var payload tgAttemptQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	return s.handleTelegramAttemptTimeout(ctx, payload.AttemptId)
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
	_, err = s.ExecuteCollectSourceDown(ctx, payload.SourceId, payload.TenantId, payload.AccountId, payload.DeleteProfiles)
	return err
}

func (s *sSysPublish) handleCollectChannelPurgeTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeCollectChannelPurgeQueuePayload(task)
	if err != nil {
		return err
	}
	return s.ExecuteCollectChannelPurge(ctx, payload)
}

func (s *sSysPublish) handleCollectSourceDeleteTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeCollectSourceDeleteQueuePayload(task)
	if err != nil {
		return err
	}
	return s.executeCollectSourceDeleteCleanup(ctx, payload.SourceId, payload.TenantId, payload.AccountId)
}

func (s *sSysPublish) handleCollectProcessTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeCollectProcessQueuePayload(task)
	if err != nil {
		return err
	}
	delay, pending, removeSchedule, err := s.processCollectSourceTask(ctx, payload)
	if err != nil || !pending {
		if err == nil && removeSchedule {
			err = s.reconcileCollectProcessSchedule(ctx, payload)
			if err == nil {
				completeCollectProcessOutbox(ctx, payload.EventId)
			}
			return err
		}
		if err == nil {
			completeCollectProcessOutbox(ctx, payload.EventId)
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
