package sys

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	publishconsts "hotgo/addons/youban_publish/consts"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"

	"hotgo/addons/youban_publish/model/input/sysin"
)

// telegramCleanupWorkerQueues 返回资料清理 Worker 负责消费的队列集合。
func telegramCleanupWorkerQueues() map[string]int {
	return map[string]int{
		tgQueueNameCleanup:                1,
		channelDispatchQueueDelete:        1,
		channelDispatchQueueDeleteManual:  100,
		channelDispatchQueueDeleteKeyword: 10,
		channelDispatchQueueDeleteCycle:   1,
	}
}

// telegramAutoDeleteWorkerQueues 返回关键字和循环删除 Worker 负责消费的队列集合。
func telegramAutoDeleteWorkerQueues() map[string]int {
	return map[string]int{
		tgQueueNameAutoDeleteRealtime:     1,
		channelDispatchQueueDeleteKeyword: 1,
		channelDispatchQueueDeleteCycle:   1,
	}
}

func (s *sSysPublish) startTelegramPushWorker(ctx context.Context) {
	s.tgQueueMu.Lock()
	if s.tgInteractiveQueueServer != nil || s.tgQueueServer != nil || s.tgCleanupQueueServer != nil {
		s.tgQueueMu.Unlock()
		return
	}
	var interactiveServer, bulkServer *asynq.Server
	delayedCheck, emptyCheck := telegramPublishQueueCheckIntervals(ctx)
	if telegramLegacyPublishQueuesEnabled(ctx) {
		interactiveServer = asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
			Concurrency:    telegramPublishInteractiveConcurrency(ctx),
			Queues:         telegramPublishInteractiveQueueWeights(ctx),
			RetryDelayFunc: telegramQueueRetryDelay,
		})
		bulkServer = asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
			Concurrency:    telegramPublishBulkConcurrency(ctx),
			Queues:         telegramPublishBulkQueueWeights(ctx),
			RetryDelayFunc: telegramQueueRetryDelay,
		})
	}
	server := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    g.Cfg().MustGet(ctx, "youbanPublish.queue.concurrency", 20).Int(),
		Queues:         telegramPublishExecutionQueues(ctx),
		RetryDelayFunc: telegramQueueRetryDelay,
		// 复用Asynq延迟任务提升器，避免1秒频道间隔被默认5秒轮询放大。
		DelayedTaskCheckInterval: delayedCheck,
		TaskCheckInterval:        emptyCheck,
	})
	cleanupServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    g.Cfg().MustGet(ctx, "youbanPublish.queue.cleanupConcurrency", 16).Int(),
		Queues:         telegramCleanupWorkerQueues(),
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	g.Log().Infof(ctx, "启动上架插件TG推送队列，兼容旧发送队列:%t", telegramLegacyPublishQueuesEnabled(ctx))
	s.tgInteractiveQueueServer = interactiveServer
	s.tgQueueServer = server
	s.tgBulkQueueServer = bulkServer
	s.tgCleanupQueueServer = cleanupServer
	s.tgQueueMu.Unlock()
	// The outbox dispatcher is part of the publish worker contract. Start it
	// from the same lifecycle entry point as the Asynq consumers so a role
	// parser or runtime branch cannot leave durable jobs stranded in pending.
	s.startTelegramOutboxDispatcher(ctx)
	go s.runTelegramStreamExecutionRecovery(ctx)
	go s.runTelegramCleanupIntents(ctx)

	mux := asynq.NewServeMux()
	mux.HandleFunc(tgTaskTypePublish, s.handleTelegramPublishTask)
	mux.HandleFunc(tgTaskTypeCleanup, s.handleTelegramCleanupTask)
	mux.HandleFunc(tgTaskTypeAutoDelete, s.handleTelegramAutoDeleteTask)
	mux.HandleFunc(tgTaskTypeDown, s.handleProfileDownTask)

	if interactiveServer != nil {
		go func() {
			if err := interactiveServer.Run(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
				g.Log().Errorf(ctx, "启动上架插件TG人工发送队列失败：%+v", err)
			}
		}()
	}
	go func() {
		if err := cleanupServer.Run(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动上架插件TG清理队列失败：%+v", err)
		}
	}()
	go func() {
		if err := server.Run(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动上架插件TG发送队列失败：%+v", err)
		}
	}()
	if bulkServer != nil {
		go func() {
			if err := bulkServer.Run(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
				g.Log().Errorf(ctx, "启动上架插件TG批量发送队列失败：%+v", err)
			}
		}()
	}
}

func (s *sSysPublish) startCollectProcessWorker(ctx context.Context) {
	s.tgQueueMu.Lock()
	if s.collectRealtimeServer != nil || s.collectProcessQueueServer != nil {
		s.tgQueueMu.Unlock()
		return
	}
	realtimeServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    g.Cfg().MustGet(ctx, "youbanPublish.queue.collectRealtimeConcurrency", 16).Int(),
		Queues:         map[string]int{tgQueueNameCollectProcessRealtimeV2: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	processServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    g.Cfg().MustGet(ctx, "youbanPublish.queue.backgroundConcurrency", 4).Int(),
		Queues:         collectProcessWorkerQueues(),
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	s.collectRealtimeServer = realtimeServer
	s.collectProcessQueueServer = processServer
	s.tgQueueMu.Unlock()

	mux := asynq.NewServeMux()
	mux.HandleFunc(tgTaskTypeCollectProcess, s.handleCollectProcessTask)
	g.Log().Infof(ctx, "启动采集处理队列 queues:%v", collectProcessWorkerQueues())
	go func() {
		if err := realtimeServer.Run(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动实时采集处理队列失败：%+v", err)
		}
	}()
	go func() {
		if err := processServer.Run(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动采集处理队列失败：%+v", err)
		}
	}()
}

func (s *sSysPublish) startTelegramBackgroundWorker(ctx context.Context) {
	s.tgQueueMu.Lock()
	if s.autoDeleteQueueServer != nil || s.autoDeleteBackfillServer != nil || s.attemptTimeoutServer != nil || s.backgroundQueueServer != nil || s.accountScheduleQueueServer != nil || s.materialImportPullServer != nil || s.cycleQueueServer != nil {
		s.tgQueueMu.Unlock()
		return
	}
	autoDeleteServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    telegramAutoDeleteConcurrency(ctx),
		Queues:         telegramAutoDeleteWorkerQueues(),
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
	workerQueues := backgroundWorkerQueues(false, true)
	server := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    g.Cfg().MustGet(ctx, "youbanPublish.queue.backgroundConcurrency", 4).Int(),
		Queues:         workerQueues,
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	accountScheduleServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    g.Cfg().MustGet(ctx, "youbanPublish.queue.accountScheduleConcurrency", 4).Int(),
		Queues:         map[string]int{tgQueueNameAccountSchedule: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	materialImportPullServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    g.Cfg().MustGet(ctx, "youbanPublish.queue.materialImportPullConcurrency", 8).Int(),
		Queues:         map[string]int{tgQueueNameMaterialImportPull: 1},
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
	g.Log().Infof(ctx, "启动上架插件后台队列，自动删除独立并发：%d queues:%v", telegramAutoDeleteConcurrency(ctx), workerQueues)
	s.autoDeleteQueueServer = autoDeleteServer
	s.autoDeleteBackfillServer = autoDeleteBackfillServer
	s.attemptTimeoutServer = attemptTimeoutServer
	s.backgroundQueueServer = server
	s.accountScheduleQueueServer = accountScheduleServer
	s.materialImportPullServer = materialImportPullServer
	s.cycleQueueServer = cycleServer
	s.profileQueueServer = profileServer
	s.duplicateQueueServer = duplicateServer
	s.historyQueueServer = historyServer
	s.mattingQueueServer = mattingServer
	s.relayQueueServer = relayServer
	s.tgQueueMu.Unlock()

	backgroundMux := asynq.NewServeMux()
	backgroundMux.HandleFunc(publishconsts.QueueAccountScheduleTask, s.handleAccountScheduleTask)
	backgroundMux.HandleFunc(publishconsts.QueueProfileCyclePrepareTask, s.handleProfileCyclePreparationTask)
	backgroundMux.HandleFunc(tgTaskTypeProfileProjection, s.handleStreamProfileProjection)
	backgroundMux.HandleFunc(tgTaskTypeCollectSourceStats, s.handleCollectSourceStatsTask)
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
	backgroundMux.HandleFunc(tgTaskTypeCollectTrigger, s.handleCollectTriggerTask)
	backgroundMux.HandleFunc(tgTaskTypeChannelMemberSync, s.handleChannelMemberSyncTask)
	backgroundMux.HandleFunc(tgTaskTypeCollectSourceDown, s.handleCollectSourceDownTask)
	backgroundMux.HandleFunc(tgTaskTypeCollectChannelPurge, s.handleCollectChannelPurgeTask)
	backgroundMux.HandleFunc(tgTaskTypeCollectNotification, s.handleCollectNotificationTask)
	backgroundMux.HandleFunc(tgTaskTypeCollectSourceDelete, s.handleCollectSourceDeleteTask)
	backgroundMux.HandleFunc(tgTaskTypeBotMediaRepair, s.handleBotMediaRepairTask)
	backgroundMux.HandleFunc(tgTaskTypeProfileMedia, s.handleProfileMediaTask)
	go s.recoverProfileMediaOperations(ctx)
	go s.profileMediaRecoveryLoop(ctx)
	backgroundMux.HandleFunc(tgTaskTypeAttemptTimeout, s.handleTelegramAttemptTimeoutTask)
	accountScheduleMux := asynq.NewServeMux()
	accountScheduleMux.HandleFunc(publishconsts.QueueAccountScheduleTask, s.handleAccountScheduleTask)
	materialImportPullMux := asynq.NewServeMux()
	materialImportPullMux.HandleFunc(tgTaskTypeMaterialImport, s.handleMaterialImportTask)
	relayMux := asynq.NewServeMux()
	relayMux.HandleFunc(tgTaskTypeRelayMedia, s.handleRelayMediaTask)
	relayMux.HandleFunc(tgTaskTypeRelayMediaGroup, s.handleRelayMediaGroupTask)
	go func() {
		if err := relayServer.Run(relayMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动 relay 媒体队列失败：%+v", err)
		}
	}()
	cycleMux := asynq.NewServeMux()
	cycleMux.HandleFunc(publishconsts.QueueAccountScheduleTask, s.handleAccountScheduleTask)
	cycleMux.HandleFunc(tgTaskTypeCycleRun, s.handleCycleRunTask)
	cycleMux.HandleFunc(tgTaskTypeCycleReschedule, s.handleCycleRescheduleTask)
	cycleMux.HandleFunc(tgTaskTypeCycleRefresh, s.handleCycleRefreshTask)
	autoDeleteMux := asynq.NewServeMux()
	autoDeleteMux.HandleFunc(tgTaskTypeAutoDelete, s.handleTelegramAutoDeleteTask)
	autoDeleteMux.HandleFunc(tgTaskTypeCleanup, s.handleTelegramCleanupTask)
	autoDeleteBackfillMux := asynq.NewServeMux()
	autoDeleteBackfillMux.HandleFunc(tgTaskTypeAutoDelete, s.handleTelegramAutoDeleteTask)
	autoDeleteBackfillMux.HandleFunc(tgTaskTypeAutoDeleteBackfill, s.handleTelegramAutoDeleteBackfillTask)
	attemptTimeoutMux := asynq.NewServeMux()
	attemptTimeoutMux.HandleFunc(tgTaskTypeAttemptTimeout, s.handleTelegramAttemptTimeoutTask)
	profileMux := asynq.NewServeMux()
	profileMux.HandleFunc(publishconsts.QueueProfileCyclePrepareTask, s.handleProfileCyclePreparationTask)
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
		if err := server.Run(backgroundMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动上架插件后台队列失败：%+v", err)
		}
	}()
	go func() {
		if err := accountScheduleServer.Run(accountScheduleMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动账户循环调度队列失败：%+v", err)
		}
	}()
	go func() {
		if err := materialImportPullServer.Run(materialImportPullMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动资料导入拉取队列失败：%+v", err)
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
	if s.mediaQueueServer != nil || s.mediaProcessServer != nil || s.materialImportMediaServer != nil {
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
	materialImportMediaServer := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    4,
		Queues:         map[string]int{tgQueueNameMaterialImportMedia: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	g.Log().Info(ctx, "启动上架插件媒体队列；采集下载与资料媒体处理隔离消费")
	s.mediaQueueServer = server
	s.mediaBulkQueueServer = bulkServer
	s.mediaProcessServer = processServer
	s.materialImportMediaServer = materialImportMediaServer
	s.tgQueueMu.Unlock()
	mediaMux := asynq.NewServeMux()
	mediaMux.HandleFunc(tgTaskTypeCollectMedia, s.handleCollectMediaCacheTask)
	// Drain tasks enqueued to the legacy shared media queue before this version.
	mediaMux.HandleFunc(tgTaskTypeMediaProcess, s.handleMediaProcessTask)
	processMux := asynq.NewServeMux()
	processMux.HandleFunc(tgTaskTypeMediaProcess, s.handleMediaProcessTask)
	materialImportMux := asynq.NewServeMux()
	materialImportMux.HandleFunc(tgTaskTypeMaterialImport, s.handleMaterialImportTask)
	go s.runMediaProcessOutbox(ctx)
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
	go func() {
		if err := materialImportMediaServer.Run(materialImportMux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动资料导入媒体队列失败：%+v", err)
		}
	}()
}

func (s *sSysPublish) stopTelegramQueueWorker() {
	s.tgQueueMu.Lock()
	interactiveServer := s.tgInteractiveQueueServer
	server := s.tgQueueServer
	bulkServer := s.tgBulkQueueServer
	cleanupServer := s.tgCleanupQueueServer
	mediaServer := s.mediaQueueServer
	mediaBulkServer := s.mediaBulkQueueServer
	mediaProcessServer := s.mediaProcessServer
	autoDeleteServer := s.autoDeleteQueueServer
	autoDeleteBackfillServer := s.autoDeleteBackfillServer
	attemptTimeoutServer := s.attemptTimeoutServer
	collectRealtimeServer := s.collectRealtimeServer
	collectProcessServer := s.collectProcessQueueServer
	backgroundServer := s.backgroundQueueServer
	accountScheduleServer := s.accountScheduleQueueServer
	relayServer := s.relayQueueServer
	materialImportPullServer := s.materialImportPullServer
	materialImportMediaServer := s.materialImportMediaServer
	cycleServer := s.cycleQueueServer
	profileServer := s.profileQueueServer
	duplicateServer := s.duplicateQueueServer
	historyServer := s.historyQueueServer
	mattingServer := s.mattingQueueServer
	client := s.tgQueueClient
	s.tgInteractiveQueueServer = nil
	s.tgQueueServer = nil
	s.tgBulkQueueServer = nil
	s.tgCleanupQueueServer = nil
	s.mediaQueueServer = nil
	s.mediaBulkQueueServer = nil
	s.mediaProcessServer = nil
	s.autoDeleteQueueServer = nil
	s.autoDeleteBackfillServer = nil
	s.attemptTimeoutServer = nil
	s.collectRealtimeServer = nil
	s.collectProcessQueueServer = nil
	s.backgroundQueueServer = nil
	s.accountScheduleQueueServer = nil
	s.relayQueueServer = nil
	s.materialImportPullServer = nil
	s.materialImportMediaServer = nil
	s.cycleQueueServer = nil
	s.profileQueueServer = nil
	s.duplicateQueueServer = nil
	s.historyQueueServer = nil
	s.mattingQueueServer = nil
	s.tgQueueClient = nil
	s.tgQueueMu.Unlock()
	if interactiveServer != nil {
		interactiveServer.Shutdown()
	}
	if server != nil {
		server.Shutdown()
	}
	if bulkServer != nil {
		bulkServer.Shutdown()
	}
	if cleanupServer != nil {
		cleanupServer.Shutdown()
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
	if collectProcessServer != nil {
		collectProcessServer.Shutdown()
	}
	if backgroundServer != nil {
		backgroundServer.Shutdown()
	}
	if accountScheduleServer != nil {
		accountScheduleServer.Shutdown()
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
	if relayServer != nil {
		relayServer.Shutdown()
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
		if isTelegramMissingJobError(jobErr) {
			// 旧 Asynq 任务可能晚于数据库 Job 清理到达。此类任务没有可执行事实，
			// 必须在当前消费中收口，不能把不存在的 Job 交给 Asynq 无限重试。
			s.settleMissingTelegramPublishTask(ctx, payload.JobId, jobErr)
			beginTelegramPublishStage(ctx, "queue.stale_job").End(nil, g.Map{
				"jobId": payload.JobId, "stale": true,
				"decision": "ack_and_stop_retry",
			})
			return nil
		}
		return jobErr
	}
	ctx = telegramPublishJobTraceContext(ctx, job)
	queueWait := int64(0)
	if job.CreatedAt != nil {
		queueWait = time.Since(job.CreatedAt.Time).Milliseconds()
	}
	dispatchLag := int64(0)
	if job.DispatchedAt != nil && job.CreatedAt != nil {
		dispatchLag = job.DispatchedAt.Time.Sub(job.CreatedAt.Time).Milliseconds()
	}
	beginTelegramPublishStage(ctx, "queue.wait_observed").End(nil, g.Map{
		"queueWaitMs": queueWait, "dispatchLagMs": dispatchLag,
		"queueName": job.QueueName, "operationNo": job.OperationNo,
		"channelId": job.ChannelId, "botId": job.BotId, "targetChatId": job.TargetChatId,
	})
	if delay, enabled := s.telegramPublishWindowDelay(ctx, job.TenantId, job.AccountId); !telegramJobBypassesAutomaticSchedule(job) && enabled && delay > 0 {
		beginTelegramPublishStage(ctx, "queue.deferred").End(nil, g.Map{"reason": "publish_window", "delayMs": delay.Milliseconds()})
		return &tgRetryAfterError{after: delay, err: errTelegramPublishWindowBlocked}
	}
	return s.SendTelegramJob(ctx, payload.JobId)
}

// isTelegramMissingJobError 识别数据库空结果和 GoFrame 包装后的空结果。
// 旧队列任务对应的 Job 已被清理时，消费端必须把它当作幂等终态。
func isTelegramMissingJobError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sql.ErrNoRows) || errors.Is(gerror.Cause(err), sql.ErrNoRows) {
		return true
	}
	cause := gerror.Cause(err)
	return cause != nil && strings.Contains(cause.Error(), "TG任务不存在")
}

// settleMissingTelegramPublishTask 收口不存在 Job 的遗留队列任务。
// Job 已不存在时无法恢复频道身份，只能按 job_id 有界清理 Outbox 和 Stream 去重键；
// 频道索引由 Outbox 恢复/重建流程按数据库事实维护，避免用不完整信息误删其他频道任务。
func (s *sSysPublish) settleMissingTelegramPublishTask(ctx context.Context, jobID int64, cause error) {
	if jobID <= 0 {
		return
	}
	clearTelegramReadyStreamDedupe(ctx, jobID)
	_, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("job_id", jobID).
		Where("event_type", telegramOutboxEventPublish).
		WhereIn("status", []string{telegramOutboxPending, telegramOutboxProcessing}).
		Data(g.Map{
			"status":           telegramOutboxDelivered,
			"lease_owner":      "",
			"lease_expires_at": nil,
			"delivered_at":     gtime.Now(),
			"last_error":       "关联TG任务已不存在，遗留队列任务已收口",
			"updated_at":       gtime.Now(),
		}).Update()
	if err != nil {
		// 收口失败不能重新抛回 Asynq，否则旧任务会形成无限重试；持久化 Outbox
		// 后续仍可由恢复扫描再次处理，当前只记录结构化告警。
		g.Log().Warning(ctx, "不存在TG任务的Outbox收口失败，已停止队列重试", g.Map{
			"jobId": jobID, "stale": true, "error": err.Error(),
		})
		return
	}
	g.Log().Warning(ctx, "发现不存在TG任务，已幂等停止遗留队列重试", g.Map{
		"jobId": jobID, "stale": true,
		"error": cause.Error(),
	})
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
	if err = s.rescheduleChannelProfileCycles(ctx, payload.ChannelId); err != nil {
		// The scheduler advances cycle_next_run_at before enqueueing so that
		// duplicate rebuilds are suppressed. Release that claim when the
		// worker fails; otherwise one transient DB/Redis error hides the
		// channel for the full claim lease and its cycle remains stalled.
		s.releaseChannelProfileCycleReschedule(context.Background(), payload.ChannelId)
		g.Log().Warningf(ctx, "频道循环重算失败，已释放检查点等待重试 channel:%d err:%+v", payload.ChannelId, err)
		return err
	}
	return nil
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
	ctx = s.collectProgressContext(ctx, payload.EventId, 0)
	identity := currentCollectWorkerIdentity(ctx)
	g.Log().Info(ctx, "采集处理任务开始", g.Map{
		"eventId": payload.EventId, "tenantId": payload.TenantId, "accountId": payload.AccountId,
		"sourceId": payload.SourceId, "outboxOwner": payload.OutboxOwner, "recovery": payload.Recovery,
		"workerRole": identity.Role, "workerInstanceId": identity.InstanceID,
		"workerRevision": identity.Revision, "workerImageDigest": identity.ImageDigest,
	})
	if payload.EventId > 0 {
		_, _ = g.DB().Model(collectProcessOutboxTable).Ctx(ctx).Where("event_id", payload.EventId).Data(g.Map{
			"worker_role": identity.Role, "worker_instance_id": identity.InstanceID,
			"worker_revision": identity.Revision, "worker_image_digest": identity.ImageDigest,
			"worker_started_at": gtime.Now(), "updated_at": gtime.Now(),
		}).Update()
	}
	unlock, lockErr := lockCollectProcessEvent(ctx, payload)
	if lockErr != nil {
		reportCollectProgress(ctx, "waiting_lock", "已有工作线程持有事件处理锁，等待重试")
		return newCollectProcessRetryError(10*time.Second, "采集事件正在处理，稍后重试")
	}
	defer unlock()
	if payload.OutboxOwner != "" {
		current, readErr := g.DB().Model(collectProcessOutboxTable).Ctx(ctx).Where("event_id", payload.EventId).Where("status", "processing").Where("lease_owner", payload.OutboxOwner).Count()
		if readErr != nil {
			return readErr
		}
		if current == 0 {
			return nil
		} // Superseded by a newer durable schedule.
	}
	reportCollectProgress(ctx, "processing", "工作线程已开始执行，检查资料分组、验证配对与规则")
	delay, pending, removeSchedule, err := s.processCollectSourceTask(ctx, payload)
	if err == nil {
		clearCollectSourceCircuit(ctx, payload)
	}
	if err != nil {
		openCircuit, circuitErr := collectSourceCircuitFailure(ctx, payload)
		if circuitErr != nil {
			return circuitErr
		}
		if openCircuit {
			reportCollectProgress(ctx, "source_circuit_open", "采集源连续失败，暂时熔断并让出恢复资源")
			return newCollectProcessRetryError(time.Minute, "采集源连续失败，进入熔断冷却")
		}
		if isCollectProcessRedisTransientError(err) {
			reportCollectProgress(ctx, "waiting", "缓存服务正在恢复，采集任务将在稍后自动重试")
			return newCollectProcessRetryError(collectProcessRedisRetryDelay, "缓存服务暂时不可用，采集任务稍后重试")
		}
		reportCollectProgress(ctx, "failed", err.Error())
	}
	if err != nil || !pending {
		errorMessage := ""
		if err != nil {
			errorMessage = err.Error()
		}
		g.Log().Info(ctx, "采集处理任务结束", g.Map{
			"eventId": payload.EventId, "pending": pending, "removeSchedule": removeSchedule,
			"error": errorMessage, "workerRole": identity.Role, "workerInstanceId": identity.InstanceID,
			"workerRevision": identity.Revision, "workerImageDigest": identity.ImageDigest,
		})
		if err != nil && payload.EventId > 0 {
			_, _ = g.DB().Model(collectProcessOutboxTable).Ctx(ctx).Where("event_id", payload.EventId).Data(g.Map{
				"last_error": errorMessage, "worker_finished_at": gtime.Now(), "updated_at": gtime.Now(),
			}).Update()
		}
		if err == nil && removeSchedule {
			err = s.reconcileCollectProcessSchedule(ctx, payload)
			if err == nil {
				completeCollectProcessOutbox(ctx, payload)
			}
			return err
		}
		if err == nil {
			completeCollectProcessOutbox(ctx, payload)
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
		g.Log().Info(ctx, "采集处理任务等待后续批次", g.Map{
			"eventId": payload.EventId, "sourceId": payload.SourceId, "delay": delay.Round(time.Second).String(),
			"workerRole": identity.Role, "workerInstanceId": identity.InstanceID,
			"workerRevision": identity.Revision, "workerImageDigest": identity.ImageDigest,
		})
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

// handleCycleRunTask 执行批次生产，超时释放运行状态以便依据已保存游标幂等恢复。
func (s *sSysPublish) handleCycleRunTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeCycleRunQueuePayload(task)
	if err != nil {
		return err
	}
	err = s.ExecuteCycleRun(ctx, payload.RunId)
	if err != nil && ctx.Err() != nil {
		// 生产阶段超时必须释放running状态，保留游标和已落库任务供幂等续跑。
		recoveryCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, recoveryErr := g.DB().Model(publishCycleRunTable).Ctx(recoveryCtx).Where("id", payload.RunId).Where("status", cycleRunStatusRunning).Data(g.Map{"status": cycleRunStatusFailed, "error_message": "循环生产超时，等待幂等恢复"}).Update()
		if recoveryErr != nil {
			g.Log().Warningf(recoveryCtx, "释放超时循环生产状态失败 run:%d err:%+v", payload.RunId, recoveryErr)
		}
	}
	return err
}

func (s *sSysPublish) handleCollectMediaCacheTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeCollectMediaQueuePayload(task)
	if err != nil {
		return err
	}
	err = s.ExecuteCollectMediaCache(ctx, payload)
	if retryErr := collectMediaRetryErrorFrom(err); retryErr != nil && retryErr.deferWithoutFailure {
		if payload.DeferredAttempts >= collectMediaMaxDeferredAttempts {
			// Owner-slot contention used to create an unbounded stream of new
			// deferred tasks. Keep the event pending for recovery, but stop
			// multiplying Redis work until a later recovery pass retries it.
			_ = s.markCollectEvent(ctx, payload.EventId, sysin.CollectEventStatusMediaPending, "媒体处理配额持续繁忙，已暂停本轮自动重试")
			g.Log().Warningf(ctx, "采集媒体任务达到公平调度重试上限，暂停本轮重试 eventId:%d attempts:%d", payload.EventId, payload.DeferredAttempts)
			return nil
		}
		payload.DeferredAttempts++
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
