package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	publishmodel "hotgo/addons/youban_publish/model"
	twdao "hotgo/addons/youban_two_way_bot/internal/dao"
	"hotgo/addons/youban_two_way_bot/internal/model/entity"
	"hotgo/addons/youban_two_way_bot/model/input/sysin"
)

func (s *sSysTwoWayBot) StartRuntime(ctx context.Context) {
	s.runtimeMu.Lock()
	defer s.runtimeMu.Unlock()
	if s.runtimeCancel != nil {
		return
	}
	runtimeCtx, cancel := context.WithCancel(context.Background())
	s.runtimeCancel = cancel
	s.runtimeCtx = runtimeCtx
	s.runtimeDone = make(chan struct{})
	go func() {
		defer close(s.runtimeDone)
		s.runRuntime(runtimeCtx)
	}()
}

func (s *sSysTwoWayBot) StopRuntime() {
	s.runtimeMu.Lock()
	cancel := s.runtimeCancel
	done := s.runtimeDone
	s.runtimeCancel = nil
	s.runtimeCtx = nil
	s.runtimeDone = nil
	s.pollingBots = nil
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
}

func (s *sSysTwoWayBot) runRuntime(ctx context.Context) {
	time.Sleep(2 * time.Second)
	mode, conf, err := s.twoWayBotRuntimeMode(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "读取双向机器人运行模式失败：%+v", err)
		return
	}
	switch mode {
	case "pull", "polling":
		s.runPolling(ctx)
	case "webhook":
		s.setupWebhooks(ctx, conf)
		<-ctx.Done()
	default:
		g.Log().Warningf(ctx, "未知双向机器人运行模式：%s", mode)
	}
}

func (s *sSysTwoWayBot) twoWayBotRuntimeMode(ctx context.Context) (string, *publishmodel.TelegramConfig, error) {
	conf, err := publishTelegramConfig(ctx)
	if err != nil {
		return "", nil, err
	}
	mode := strings.TrimSpace(conf.BotRuntimeMode)
	if mode == "" || mode == "auto" {
		if isPublicWebhookBaseUrl(conf.WebhookBaseUrl) {
			mode = "webhook"
		} else {
			mode = "pull"
		}
	}
	if mode == "polling" {
		mode = "pull"
	}
	return mode, conf, nil
}

func isPublicWebhookBaseUrl(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	if !strings.HasPrefix(value, "https://") {
		return false
	}
	return !strings.Contains(value, "localhost") &&
		!strings.Contains(value, "127.0.0.1") &&
		!strings.Contains(value, "0.0.0.0") &&
		!strings.Contains(value, "[::1]")
}

func (s *sSysTwoWayBot) runPolling(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	runningBots := map[string]struct{}{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			bots, err := s.enabledBots(ctx)
			if err != nil {
				g.Log().Warningf(ctx, "读取双向机器人失败：%+v", err)
				continue
			}
			for _, item := range bots {
				if item == nil || strings.TrimSpace(item.BotToken) == "" {
					continue
				}
				if _, ok := runningBots[item.BotToken]; ok {
					continue
				}
				if err = s.enablePollingForBot(ctx, item); err != nil {
					g.Log().Warningf(ctx, "启动双向机器人Polling失败 bot:%d err:%+v", item.Id, err)
					continue
				}
				runningBots[item.BotToken] = struct{}{}
			}
		}
	}
}

func (s *sSysTwoWayBot) setupWebhooks(ctx context.Context, conf *publishmodel.TelegramConfig) {
	if conf == nil || strings.TrimSpace(conf.WebhookBaseUrl) == "" {
		g.Log().Warning(ctx, "双向机器人Webhook Base URL未配置，跳过自动setWebhook")
		return
	}
	bots, err := s.enabledBots(ctx)
	if err != nil {
		g.Log().Warningf(ctx, "读取双向机器人失败：%+v", err)
		return
	}
	for _, item := range bots {
		if item == nil || strings.TrimSpace(item.BotToken) == "" || strings.TrimSpace(item.SupergroupId) == "" {
			continue
		}
		if err = s.enableWebhookForBot(ctx, item, conf); err != nil {
			g.Log().Warningf(ctx, "设置双向机器人Webhook失败 bot:%d err:%+v", item.Id, err)
		}
	}
}

func (s *sSysTwoWayBot) enablePollingForBot(ctx context.Context, row *entity.YoubanTwoWayBotBot) error {
	bot, err := s.telegramBot(ctx, row.BotToken)
	if err != nil {
		return err
	}
	if _, err = bot.DeleteWebhook(ctx, &tgbot.DeleteWebhookParams{DropPendingUpdates: false}); err != nil {
		g.Log().Infof(ctx, "清理双向机器人Webhook失败，继续Polling bot:%d err:%+v", row.Id, err)
	}
	if err = s.startPollingBotIfNeeded(ctx, row); err != nil {
		return err
	}
	return s.updateWebhookStatus(ctx, row.Id, sysin.TwoWayBotWebhookPolling, "")
}

func (s *sSysTwoWayBot) startPollingBotIfNeeded(ctx context.Context, row *entity.YoubanTwoWayBotBot) error {
	if row == nil || strings.TrimSpace(row.BotToken) == "" {
		return nil
	}
	s.runtimeMu.Lock()
	runCtx := s.runtimeCtx
	if runCtx == nil {
		runCtx = context.Background()
	}
	if s.pollingBots == nil {
		s.pollingBots = map[string]struct{}{}
	}
	if _, ok := s.pollingBots[row.BotToken]; ok {
		s.runtimeMu.Unlock()
		return nil
	}
	s.pollingBots[row.BotToken] = struct{}{}
	s.runtimeMu.Unlock()

	bot, err := s.telegramBot(ctx, row.BotToken)
	if err != nil {
		s.runtimeMu.Lock()
		delete(s.pollingBots, row.BotToken)
		s.runtimeMu.Unlock()
		return err
	}
	go bot.Start(runCtx)
	g.Log().Infof(ctx, "双向机器人Polling已启动 bot:%d username:%s", row.Id, row.BotUsername)
	return nil
}

func (s *sSysTwoWayBot) enableWebhookForBot(ctx context.Context, row *entity.YoubanTwoWayBotBot, conf *publishmodel.TelegramConfig) error {
	if conf == nil || strings.TrimSpace(conf.WebhookBaseUrl) == "" {
		return gerror.New("请先配置官方域名或Telegram Webhook Base URL")
	}
	bot, err := s.telegramBot(ctx, row.BotToken)
	if err != nil {
		return err
	}
	webhookURL := fmt.Sprintf("%s/api/youban_two_way_bot/telegram/webhook?botId=%d", strings.TrimRight(conf.WebhookBaseUrl, "/"), row.Id)
	params := &tgbot.SetWebhookParams{URL: webhookURL, AllowedUpdates: []string{"message", "edited_message"}}
	if strings.TrimSpace(conf.WebhookSecret) != "" {
		params.SecretToken = strings.TrimSpace(conf.WebhookSecret)
	}
	_, err = bot.SetWebhook(ctx, params)
	if err != nil {
		_ = s.updateWebhookStatus(ctx, row.Id, sysin.TwoWayBotWebhookFailed, err.Error())
		return gerror.Wrap(err, "设置Telegram Webhook失败")
	}
	return s.updateWebhookStatus(ctx, row.Id, sysin.TwoWayBotWebhookReady, "")
}

func (s *sSysTwoWayBot) updateWebhookStatus(ctx context.Context, id int64, status string, message string) error {
	_, err := twdao.YoubanTwoWayBotBot.Ctx(ctx).WherePri(id).Data(g.Map{
		"webhook_status":  status,
		"error_message":   message,
		"last_webhook_at": gtime.Now(),
		"updated_at":      gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "保存Webhook状态失败")
	}
	return nil
}

func (s *sSysTwoWayBot) enabledBots(ctx context.Context) ([]*entity.YoubanTwoWayBotBot, error) {
	columns := twdao.YoubanTwoWayBotBot.Columns()
	var rows []*entity.YoubanTwoWayBotBot
	err := twdao.YoubanTwoWayBotBot.Ctx(ctx).
		Where(columns.Status, sysin.TwoWayBotStatusEnabled).
		WhereNull(columns.DeletedAt).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取双向机器人失败")
	}
	return rows, nil
}

func (s *sSysTwoWayBot) botByToken(ctx context.Context, token string) (*entity.YoubanTwoWayBotBot, error) {
	columns := twdao.YoubanTwoWayBotBot.Columns()
	var row *entity.YoubanTwoWayBotBot
	err := twdao.YoubanTwoWayBotBot.Ctx(ctx).
		Where(columns.BotToken, strings.TrimSpace(token)).
		Where(columns.Status, sysin.TwoWayBotStatusEnabled).
		WhereNull(columns.DeletedAt).
		OrderDesc(columns.Id).
		Scan(&row)
	if err != nil {
		return nil, gerror.Wrap(err, "读取双向机器人失败")
	}
	return row, nil
}
