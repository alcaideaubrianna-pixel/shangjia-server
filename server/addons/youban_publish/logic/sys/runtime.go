package sys

import (
	"context"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

type publishRuntimeMutex struct {
	sync.Mutex
}

func (s *sSysPublish) StartRuntime(ctx context.Context) {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	if s.runtimeCancel != nil {
		return
	}
	runtimeCtx, cancel := context.WithCancel(context.Background())
	s.runtimeCancel = cancel
	s.runtimeDone = make(chan struct{})
	go func() {
		defer close(s.runtimeDone)
		s.runPublishRuntime(runtimeCtx)
	}()
}

func (s *sSysPublish) StopRuntime() {
	s.runtimeMu.Lock()
	cancel := s.runtimeCancel
	done := s.runtimeDone
	s.runtimeCancel = nil
	s.runtimeDone = nil
	s.runtimeMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	}
	s.stopCollectGroupedEventTimers()
	s.stopTelegramQueueWorker()
	s.tgQueueMu.Lock()
	s.telegramOutboxStarted = false
	s.tgQueueMu.Unlock()
}

func (s *sSysPublish) runPublishRuntime(ctx context.Context) {
	config := loadPublishRuntimeConfig(ctx)
	if len(config.Roles) == 0 {
		g.Log().Warning(ctx, "上架插件未识别到有效运行角色，当前实例不会启动后台运行组件")
	}
	g.Log().Infof(ctx, "启动上架插件运行组件 roles:%v account:%t scheduler:%t pushWorker:%t mediaWorker:%t backgroundWorker:%t collectorWorker:%t collectProcessWorker:%t",
		config.Roles, config.Account, config.Scheduler, config.PushWorker, config.MediaWorker, config.BackgroundWorker, config.CollectorWorker, config.CollectProcessWorker)
	startRuntimeResourceObserver(ctx, config)
	observePublishRuntimeHeartbeat(ctx, config)
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()
	if config.PushWorker {
		s.startTelegramPushWorker(ctx)
		if telegramChannelQueueEnabled() {
			go s.runTelegramChannelQueueWorker(ctx)
		}
		if config.RedisStreamGray {
			go s.runRedisStreamGrayConsumer(ctx)
		}
	}
	if telegramDeliveryRuntimeEnabled(config) && !telegramChannelQueueEnabled() {
		// Telegram recovery belongs with the worker that owns delivery state.
		// Keeping it on Scheduler made every channel unrecoverable whenever the
		// scheduler role was stopped independently of the push workers.
		go s.runTelegramJobRecovery(ctx)
		go s.runTelegramSendWatchdog(ctx)
	}
	if config.MediaWorker {
		s.startTelegramMediaWorker(ctx)
	}
	// Collection processing is owned exclusively by Collector. Do not let a
	// stale or over-broad role configuration make Media/Background consume the
	// durable collect outbox and persist text with a different rule pipeline.
	if config.CollectorWorker {
		s.startCollectProcessWorker(ctx)
		go s.runCollectProcessDispatcher(ctx)
		// Collect processing has two halves: the collector worker consumes
		// queue tasks, while the recovery loop dispatches durable process_outbox
		// rows into that queue. Keep the dispatcher available with the consumer
		// when Scheduler is deployed separately or temporarily unavailable.
		if !config.Scheduler {
			go s.runCollectRecovery(ctx)
		}
	}
	if config.BackgroundQueue {
		s.startTelegramBackgroundWorker(ctx)
	}
	// 资料指纹回填是低优先级维护任务，只允许 Background 角色运行，不能和 Publish 调度/发送共用实例。
	if config.BackgroundWorker {
		go s.runProfileFingerprintBackfill(ctx)
	}
	if config.PushWorker || config.MediaWorker || config.BackgroundWorker || config.CollectorWorker || config.CollectProcessWorker {
		go s.runAsynqObserveMetrics(ctx)
	}
	if config.Scheduler {
		s.startPublishSchedulers(ctx)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeatTicker.C:
			observePublishRuntimeHeartbeat(ctx, config)
		}
	}
}

func telegramDeliveryRuntimeEnabled(config publishRuntimeConfig) bool {
	return config.PushWorker
}

func (s *sSysPublish) startPublishSchedulers(ctx context.Context) {
	go s.runAccountNotices(ctx)
	if err := s.backfillPublishSuccessRecords(ctx, 5000); err != nil {
		g.Log().Warningf(ctx, "补写成功发布记录失败：%+v", err)
	}
	go s.runProfileCycleStartupRecovery(ctx)
	go s.runScheduledPublishRuntime(ctx)
	go s.runMessagePushPlanScheduler(ctx)
	go s.runFullPushBatchScheduler(ctx)
	go s.runTelegramObserveStatsRefresher(ctx)
	go s.runCollectRecovery(ctx)
	go s.runMaterialImportRecovery(ctx)
	go s.runPublishRecordRetentionCleaner(ctx)
	go func() {
		if err := s.recoverInterruptedTelegramJobs(ctx, 200); err != nil {
			g.Log().Warningf(ctx, "恢复中断的TG推送任务失败：%+v", err)
		}
	}()
}
