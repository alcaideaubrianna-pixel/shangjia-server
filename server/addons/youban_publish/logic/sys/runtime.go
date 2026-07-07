package sys

import (
	"context"
	"fmt"
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
	s.startTelegramQueueWorker(ctx)
	go s.runTelegramRuntime(ctx)
	go s.runScheduledPublishRuntime(ctx)
	go s.runCyclePlanScheduler(ctx)
	<-ctx.Done()
}

func (s *sSysPublish) runTelegramRuntime(ctx context.Context) {
	time.Sleep(2 * time.Second)
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "读取上架插件Telegram配置失败：%+v", err)
		return
	}
	mode := conf.BotRuntimeMode
	if mode == "" || mode == "auto" {
		systemMode := g.Cfg().MustGet(ctx, "system.mode", "").String()
		if systemMode == "" || systemMode == "develop" || systemMode == "testing" || systemMode == "not-set" {
			mode = "pull"
		} else {
			mode = "webhook"
		}
	}
	switch mode {
	case "pull", "polling":
		s.runTelegramPolling(ctx)
	case "webhook":
		s.setupTelegramWebhooks(ctx)
		<-ctx.Done()
	default:
		g.Log().Warningf(ctx, "未知上架插件Bot运行模式：%s", mode)
	}
}

func (s *sSysPublish) runTelegramPolling(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	runningBots := map[string]struct{}{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			bots, err := s.enabledBots(ctx, -1)
			if err != nil {
				g.Log().Warningf(ctx, "读取上架插件Bot失败：%+v", err)
				continue
			}
			for _, item := range bots {
				if item == nil || item.BotToken == "" {
					continue
				}
				if _, ok := runningBots[item.BotToken]; ok {
					continue
				}
				bot, err := s.telegramBot(ctx, item.BotToken)
				if err != nil {
					g.Log().Warningf(ctx, "初始化上架插件Bot失败 bot:%d err:%+v", item.Id, err)
					continue
				}
				if err = s.telegramDeleteWebhook(ctx, item.BotToken); err != nil {
					g.Log().Warningf(ctx, "清理上架插件Bot webhook失败 bot:%d err:%+v", item.Id, err)
				}
				runningBots[item.BotToken] = struct{}{}
				go bot.Start(ctx)
			}
		}
	}
}

func (s *sSysPublish) setupTelegramWebhooks(ctx context.Context) {
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "读取上架插件Telegram配置失败：%+v", err)
		return
	}
	if conf.WebhookBaseUrl == "" {
		g.Log().Warning(ctx, "上架插件Webhook Base URL未配置，跳过自动setWebhook")
		return
	}
	bots, err := s.enabledBots(ctx, -1)
	if err != nil {
		g.Log().Warningf(ctx, "读取上架插件Bot失败：%+v", err)
		return
	}
	for _, item := range bots {
		if item == nil || item.BotToken == "" {
			continue
		}
		webhookURL := fmt.Sprintf("%s/api/youban_publish/telegram/webhook?botId=%d", conf.WebhookBaseUrl, item.Id)
		if err = s.telegramSetWebhook(ctx, item.BotToken, webhookURL); err != nil {
			g.Log().Warningf(ctx, "设置上架插件Bot webhook失败 bot:%d err:%+v", item.Id, err)
		}
	}
}
