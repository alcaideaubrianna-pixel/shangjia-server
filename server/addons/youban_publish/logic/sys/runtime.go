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
}

func (s *sSysPublish) runPublishRuntime(ctx context.Context) {
	if err := ensureCollectSourceColumns(ctx); err != nil {
		g.Log().Warningf(ctx, "检查采集源字段失败：%+v", err)
	}
	config := loadPublishRuntimeConfig(ctx)
	if len(config.Roles) == 0 {
		g.Log().Warning(ctx, "上架插件未识别到有效运行角色，当前实例不会启动后台运行组件")
	}
	g.Log().Infof(ctx, "启动上架插件运行组件 roles:%v account:%t scheduler:%t pushWorker:%t mediaWorker:%t backgroundWorker:%t",
		config.Roles, config.Account, config.Scheduler, config.PushWorker, config.MediaWorker, config.BackgroundWorker)
	if config.PushWorker {
		s.startTelegramPushWorker(ctx)
	}
	if config.MediaWorker {
		s.startTelegramMediaWorker(ctx)
	}
	if config.BackgroundWorker {
		s.startTelegramBackgroundWorker(ctx)
	}
	if config.Scheduler {
		s.startPublishSchedulers(ctx)
	}
	if config.Account {
		go s.runAccountCollectSupervisor(ctx)
	}
	<-ctx.Done()
}

func (s *sSysPublish) startPublishSchedulers(ctx context.Context) {
	if err := ensurePublishChannelColumns(ctx); err != nil {
		g.Log().Errorf(ctx, "检查频道循环上架字段失败：%+v", err)
	}
	if err := s.backfillPublishSuccessRecords(ctx, 5000); err != nil {
		g.Log().Warningf(ctx, "补写成功发布记录失败：%+v", err)
	}
	go s.runProfileCycleStartupRecovery(ctx)
	go s.runScheduledPublishRuntime(ctx)
	go s.runMessagePushPlanScheduler(ctx)
	go s.runTelegramChannelScheduler(ctx)
	go s.runFullPushBatchScheduler(ctx)
	go s.runTelegramObserveStatsRefresher(ctx)
	go s.runTelegramJobRecovery(ctx)
	go s.runCollectRecovery(ctx)
	go s.runMaterialImportRecovery(ctx)
	go s.runPublishRecordRetentionCleaner(ctx)
	go func() {
		if err := s.recoverInterruptedTelegramJobs(ctx, 200); err != nil {
			g.Log().Warningf(ctx, "恢复中断的TG推送任务失败：%+v", err)
		}
	}()
}
