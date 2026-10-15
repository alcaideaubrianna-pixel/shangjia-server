package sys

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"hotgo/addons/youban_bot/consts"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/internal/model/entity"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"
)

const relaySendTimeout = 20 * time.Second
const relayLeaseTTL = time.Minute

func (s *sSysBot) startRelayRuntime(parent context.Context) {
	s.relayMu.Lock()
	defer s.relayMu.Unlock()
	if s.relayCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	s.relayCancel = cancel
	s.relayDone = done
	go func() {
		defer close(done)
		var workers sync.WaitGroup
		for i := 0; i < 4; i++ {
			workers.Add(1)
			go func() { defer workers.Done(); s.relayDeliveryLoop(ctx) }()
		}
		workers.Wait()
	}()
}
func (s *sSysBot) stopRelayRuntime() {
	s.relayMu.Lock()
	defer s.relayMu.Unlock()
	if s.relayCancel == nil {
		return
	}
	s.relayCancel()
	select {
	case <-s.relayDone:
	case <-time.After(25 * time.Second):
	}
	s.relayCancel = nil
	s.relayDone = nil
}

func (s *sSysBot) relayDeliveryTick(ctx context.Context) error {
	c := dao.YoubanBotRelayDelivery.Columns()
	// An expired send may already have reached Telegram. Do not blindly send it again.
	if err := recoverRelayDeliveries(ctx); err != nil {
		return err
	}
	enabled, err := s.relayPluginEnabled(ctx)
	if err != nil || !enabled {
		return err
	}
	var next *entity.YoubanBotRelayDelivery
	if err = dao.YoubanBotRelayDelivery.Ctx(ctx).Where(c.Status, consts.RelayPending).WhereLTE(c.AvailableAt, gtime.Now()).OrderAsc(c.Id).Scan(&next); err != nil || next == nil {
		return err
	}
	rows, token, err := claimRelayDeliveries(ctx, next)
	if err != nil || len(rows) == 0 {
		return err
	}
	sendCtx, cancel := context.WithTimeout(ctx, relaySendTimeout)
	defer cancel()
	ids, status, sendErr := s.sendRelayDeliveries(sendCtx, rows)
	available := gtime.Now()
	if status == consts.RelayPending {
		available = available.Add(time.Duration(min(rows[0].Attempts*rows[0].Attempts+1, 300)) * time.Second)
	}
	var flood *tgbot.TooManyRequestsError
	if errors.As(sendErr, &flood) {
		available = gtime.Now().Add(time.Duration(flood.RetryAfter+1) * time.Second)
	}
	// Persist outcomes even if the send deadline or process context has been cancelled.
	saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer saveCancel()
	for i, row := range rows {
		id := int64(0)
		if i < len(ids) {
			id = int64(ids[i])
		}
		message := ""
		if sendErr != nil {
			message = relayErrorText(sendErr)
		}
		data := do.YoubanBotRelayDelivery{Status: status, TargetMessageId: id, ErrorMessage: message, AvailableAt: available, LeaseToken: ""}
		if status == consts.RelaySent || status == consts.RelayCancelled {
			data.RawJson = ""
		}
		_, err = dao.YoubanBotRelayDelivery.Ctx(saveCtx).WherePri(row.Id).Where(c.Status, consts.RelaySending).Where(c.LeaseToken, token).Data(data).Update()
		if err != nil {
			return gerror.Wrap(err, "保存聊天投递结果失败")
		}
	}
	s.relayDeliveryNotice(saveCtx, rows[0], status)
	return nil
}

func claimRelayDeliveries(ctx context.Context, next *entity.YoubanBotRelayDelivery) ([]*entity.YoubanBotRelayDelivery, string, error) {
	rows := make([]*entity.YoubanBotRelayDelivery, 0)
	token := guid.S()
	err := dao.YoubanBotRelayDelivery.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		c := dao.YoubanBotRelayDelivery.Columns()
		mod := dao.YoubanBotRelayDelivery.Ctx(ctx).Where(c.Status, consts.RelayPending)
		if next.MediaGroupId == "" {
			mod = mod.WherePri(next.Id)
		} else {
			mod = mod.Where(do.YoubanBotRelayDelivery{BotId: next.BotId, SourceChatId: next.SourceChatId, MediaGroupId: next.MediaGroupId, TargetChatId: next.TargetChatId, Kind: next.Kind})
		}
		if err := mod.OrderAsc(c.SourceMessageId).Limit(100).LockUpdate().Scan(&rows); err != nil {
			return err
		}
		for _, row := range rows {
			row.Attempts++
			result, err := dao.YoubanBotRelayDelivery.Ctx(ctx).WherePri(row.Id).Where(c.Status, consts.RelayPending).Data(do.YoubanBotRelayDelivery{Status: consts.RelaySending, Attempts: row.Attempts, LeaseToken: token, AvailableAt: gtime.Now().Add(relayLeaseTTL)}).Update()
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if n != 1 {
				return gerror.New("消息已由其他实例抢占")
			}
		}
		return nil
	})
	return rows, token, gerror.Wrap(err, "抢占聊天投递任务失败")
}

func (s *sSysBot) sendRelayDeliveries(ctx context.Context, rows []*entity.YoubanBotRelayDelivery) ([]int, string, error) {
	first := rows[0]
	cfg, err := relayConfig(ctx, first.BotId)
	if err != nil {
		return nil, relayRetryStatus(first.Attempts), err
	}
	botRow, err := s.botById(ctx, first.BotId)
	if err != nil {
		return nil, consts.RelayCancelled, err
	}
	if cfg == nil || cfg.Enabled != 1 || botRow.Status != 1 {
		return nil, consts.RelayCancelled, nil
	}
	if first.Kind == consts.RelayKindAudit && cfg.AuditGroupId != first.TargetChatId {
		return nil, consts.RelayCancelled, nil
	}
	if first.Kind == consts.RelayKindChat {
		var binding *entity.YoubanBotRelayBinding
		if err := dao.YoubanBotRelayBinding.Ctx(ctx).Where(do.YoubanBotRelayBinding{Id: first.BindingId, BotId: first.BotId}).Scan(&binding); err != nil {
			return nil, relayRetryStatus(first.Attempts), err
		}
		if binding == nil {
			return nil, consts.RelayCancelled, nil
		}
		if reason, err := relayPendingFilter(ctx, cfg, rows); err != nil {
			return nil, relayRetryStatus(first.Attempts), err
		} else if reason != "" {
			return nil, consts.RelayCancelled, gerror.New(reason)
		}

	}
	bot, err := s.relayBot(ctx, botRow.BotToken)
	if err != nil {
		return nil, relayRetryStatus(first.Attempts), err
	}
	threadID := 0
	if first.Kind == consts.RelayKindAudit {
		threadID, err = s.relayAuditThread(ctx, bot, first)
		if err != nil {
			return nil, relayRetryStatus(first.Attempts), err
		}
	}
	messageIDs := make([]int, 0, len(rows))
	for _, row := range rows {
		messageIDs = append(messageIDs, int(row.SourceMessageId))
	}
	// CopyMessages preserves albums and entities without exposing the original sender.
	// Audit uses forwardMessages to retain the source attribution.
	// Each source message is durably claimed before the single external operation.
	var result []models.MessageID
	if first.Kind == consts.RelayKindAudit {
		result, err = bot.ForwardMessages(ctx, &tgbot.ForwardMessagesParams{ChatID: first.TargetChatId, MessageThreadID: threadID, FromChatID: first.SourceChatId, MessageIDs: messageIDs})
	} else {
		result, err = bot.CopyMessages(ctx, &tgbot.CopyMessagesParams{ChatID: first.TargetChatId, MessageThreadID: threadID, FromChatID: first.SourceChatId, MessageIDs: messageIDs})
	}
	if err != nil {
		if first.Kind == consts.RelayKindAudit && relayTopicMissing(err) {
			if resetErr := invalidateRelayAuditThread(ctx, first); resetErr != nil {
				return nil, consts.RelayFailed, resetErr
			}
			return nil, relayRetryStatus(first.Attempts), err
		}
		return nil, relaySendErrorStatus(err, first.Attempts), err
	}
	ids := make([]int, 0, len(result))
	for _, id := range result {
		ids = append(ids, id.ID)
	}
	if len(ids) != len(rows) {
		if len(ids) == 0 {
			return nil, consts.RelayFailed, gerror.New("消息无法复制或已删除，未发送到目标聊天")
		}
		return nil, consts.RelayUncertain, gerror.New("Telegram跳过了部分消息，目标ID：" + fmt.Sprint(ids) + "，请人工核对，避免重复发送")
	}
	return ids, consts.RelaySent, nil
}
func relayRetryStatus(attempts int) string {
	if attempts >= 10 {
		return consts.RelayFailed
	}
	return consts.RelayPending
}
func relaySendErrorStatus(err error, attempts int) string {
	var flood *tgbot.TooManyRequestsError
	if errors.As(err, &flood) || errors.Is(err, tgbot.ErrorTooManyRequests) {
		return relayRetryStatus(attempts)
	}
	if errors.Is(err, tgbot.ErrorBadRequest) || errors.Is(err, tgbot.ErrorForbidden) || errors.Is(err, tgbot.ErrorUnauthorized) {
		return consts.RelayFailed
	}
	// Network failures and cancellation may occur after Telegram accepted the request.
	return consts.RelayUncertain
}
func relayErrorText(err error) string {
	runes := []rune(fmt.Sprint(err))
	if len(runes) > 1000 {
		runes = runes[:1000]
	}
	return string(runes)
}

func recoverRelayDeliveries(ctx context.Context) error {
	c := dao.YoubanBotRelayDelivery.Columns()
	_, err := dao.YoubanBotRelayDelivery.Ctx(ctx).Where(c.Status, consts.RelaySending).WhereLT(c.AvailableAt, gtime.Now()).Data(do.YoubanBotRelayDelivery{Status: consts.RelayUncertain, ErrorMessage: "发送进程中断，结果不确定，请核对目标消息"}).Update()
	return gerror.Wrap(err, "恢复中断的聊天投递失败")
}

func (s *sSysBot) relayDeliveryLoop(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.relayDeliveryTick(ctx); err != nil && ctx.Err() == nil {
				g.Log().Warningf(ctx, "双向机器人投递失败 err:%+v", err)
			}
		}
	}
}
