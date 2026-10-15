package sys

import (
	"context"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/addons/youban_bot/model/input/sysin"
)

// Stable Bot rows also serialize edits before a relay configuration has been created.
func relayLockBot(ctx context.Context, botID int64) error {
	var bot *entity.YoubanBotRegistry
	if err := dao.YoubanBotRegistry.Ctx(ctx).WherePri(botID).LockUpdate().Scan(&bot); err != nil {
		return err
	}
	if bot == nil {
		return gerror.New("Bot不存在")
	}
	return nil
}
func relayMutate(ctx context.Context, botID int64, fn func(context.Context) error) error {
	return dao.YoubanBotRegistry.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := relayLockBot(ctx, botID); err != nil {
			return err
		}
		return fn(ctx)
	})
}
func (s *sSysBot) AdminRelaySaveSettings(ctx context.Context, in *sysin.RelaySettingsInp) error {
	if err := s.relayCheckAccess(ctx, in.BotId); err != nil {
		return err
	}
	return relayMutate(ctx, in.BotId, func(ctx context.Context) error { return s.adminRelaySaveSettings(ctx, in) })
}
func (s *sSysBot) AdminRelayBind(ctx context.Context, in *sysin.RelayBindingInp) error {
	if err := s.relayCheckAccess(ctx, in.BotId); err != nil {
		return err
	}
	return relayMutate(ctx, in.BotId, func(ctx context.Context) error { return s.adminRelayBind(ctx, in) })
}
func (s *sSysBot) AdminRelayUnbind(ctx context.Context, in *sysin.RelayDeleteInp) error {
	if err := s.relayCheckAccess(ctx, in.BotId); err != nil {
		return err
	}
	return relayMutate(ctx, in.BotId, func(ctx context.Context) error { return s.adminRelayUnbind(ctx, in) })
}
func (s *sSysBot) AdminRelaySaveRule(ctx context.Context, in *sysin.RelayRuleInp) error {
	if err := s.relayCheckAccess(ctx, in.BotId); err != nil {
		return err
	}
	return relayMutate(ctx, in.BotId, func(ctx context.Context) error { return s.adminRelaySaveRule(ctx, in) })
}
func (s *sSysBot) AdminRelayDeleteRule(ctx context.Context, in *sysin.RelayDeleteInp) error {
	if err := s.relayCheckAccess(ctx, in.BotId); err != nil {
		return err
	}
	return relayMutate(ctx, in.BotId, func(ctx context.Context) error { return s.adminRelayDeleteRule(ctx, in) })
}
func (s *sSysBot) AdminRelaySwitch(ctx context.Context, in *sysin.RelaySwitchInp) error {
	if err := s.relayCheckAccess(ctx, in.BotId); err != nil {
		return err
	}
	return relayMutate(ctx, in.BotId, func(ctx context.Context) error { return s.adminRelaySwitch(ctx, in) })
}
