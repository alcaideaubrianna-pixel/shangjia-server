package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
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
}

func (s *sSysTwoWayBot) runRuntime(ctx context.Context) {
	if err := gatewayservice.Gateway().Refresh(ctx); err != nil {
		g.Log().Warningf(ctx, "刷新统一TG Bot Gateway失败：%+v", err)
	}
	<-ctx.Done()
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
