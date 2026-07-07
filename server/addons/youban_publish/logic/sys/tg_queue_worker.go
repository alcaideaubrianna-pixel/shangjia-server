package sys

import (
	"context"
	"errors"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
)

func (s *sSysPublish) startTelegramQueueWorker(ctx context.Context) {
	s.tgQueueMu.Lock()
	if s.tgQueueServer != nil {
		s.tgQueueMu.Unlock()
		return
	}
	server := asynq.NewServer(telegramQueueRedisOpt(ctx), asynq.Config{
		Concurrency:    g.Cfg().MustGet(ctx, "youbanPublish.queue.concurrency", 20).Int(),
		Queues:         map[string]int{tgQueueNameDefault: 1},
		RetryDelayFunc: telegramQueueRetryDelay,
	})
	s.tgQueueServer = server
	s.tgQueueMu.Unlock()

	mux := asynq.NewServeMux()
	mux.HandleFunc(tgTaskTypePublish, s.handleTelegramPublishTask)
	mux.HandleFunc(tgTaskTypeDelete, s.handleTelegramDeleteTask)
	mux.HandleFunc(tgTaskTypeCleanup, s.handleTelegramCleanupTask)
	mux.HandleFunc(tgTaskTypeImport, s.handleImportTask)
	mux.HandleFunc(tgTaskTypeRepair, s.handleTgMessageRepairTask)
	mux.HandleFunc(tgTaskTypeDown, s.handleProfileDownTask)
	mux.HandleFunc(tgTaskTypeCycleRun, s.handleCycleRunTask)

	go func() {
		if err := server.Run(mux); err != nil && !errors.Is(err, asynq.ErrServerClosed) {
			g.Log().Errorf(ctx, "启动上架插件TG队列失败：%+v", err)
		}
	}()
}

func (s *sSysPublish) stopTelegramQueueWorker() {
	s.tgQueueMu.Lock()
	server := s.tgQueueServer
	client := s.tgQueueClient
	s.tgQueueServer = nil
	s.tgQueueClient = nil
	s.tgQueueMu.Unlock()
	if server != nil {
		server.Shutdown()
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

func (s *sSysPublish) handleTelegramDeleteTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeTelegramQueuePayload(task)
	if err != nil {
		return err
	}
	return s.DeleteTelegramJobMessages(ctx, payload.JobId)
}

func (s *sSysPublish) handleTelegramCleanupTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeTelegramQueuePayload(task)
	if err != nil {
		return err
	}
	return s.CleanupTelegramJobMessages(ctx, payload.JobId)
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

func (s *sSysPublish) handleProfileDownTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeProfileDownQueuePayload(task)
	if err != nil {
		return err
	}
	return s.handleProfilesDown(ctx, payload.ProfileIds, payload.TenantId)
}

func (s *sSysPublish) handleCycleRunTask(ctx context.Context, task *asynq.Task) error {
	payload, err := decodeCycleRunQueuePayload(task)
	if err != nil {
		return err
	}
	return s.ExecuteCycleRun(ctx, payload.RunId)
}
