package sys

import (
	"context"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/entity"
	"hotgo/addons/youban_bot/model/input/sysin"
)

// Lock the relay configuration itself. The management Bot ID may be either the
// current source Bot ID or a legacy relay Bot ID during migration.
func relayLockConfig(ctx context.Context, botID int64) error {
	columns := dao.YoubanBotRelayConfig.Columns()
	var config *entity.YoubanBotRelayConfig
	if err := dao.YoubanBotRelayConfig.Ctx(ctx).Where(columns.BotId, botID).LockUpdate().Scan(&config); err != nil {
		return err
	}
	if config == nil {
		if err := dao.YoubanBotRelayConfig.Ctx(ctx).Where(columns.SourceBotId, botID).LockUpdate().Scan(&config); err != nil {
			return err
		}
	}
	if config == nil {
		return gerror.New("双向机器人配置不存在")
	}
	return nil
}
func relayMutate(ctx context.Context, botID int64, fn func(context.Context) error) error {
	return dao.YoubanBotRelayConfig.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := relayLockConfig(ctx, botID); err != nil {
			return err
		}
		return fn(ctx)
	})
}
func (s *sSysBot) AdminRelaySaveSettings(ctx context.Context, in *sysin.RelaySettingsInp) error {
	botID, err := s.relayCanonicalBotID(ctx, in.BotId)
	if err != nil {
		return err
	}
	in.BotId = botID
	return relayMutate(ctx, botID, func(ctx context.Context) error { return s.adminRelaySaveSettings(ctx, in) })
}
func (s *sSysBot) AdminRelayBind(ctx context.Context, in *sysin.RelayBindingInp) error {
	botID, err := s.relayCanonicalBotID(ctx, in.BotId)
	if err != nil {
		return err
	}
	in.BotId = botID
	return relayMutate(ctx, botID, func(ctx context.Context) error { return s.adminRelayBind(ctx, in) })
}
func (s *sSysBot) AdminRelayUnbind(ctx context.Context, in *sysin.RelayDeleteInp) error {
	botID, err := s.relayCanonicalBotID(ctx, in.BotId)
	if err != nil {
		return err
	}
	in.BotId = botID
	return relayMutate(ctx, botID, func(ctx context.Context) error { return s.adminRelayUnbind(ctx, in) })
}
func (s *sSysBot) AdminRelaySaveRule(ctx context.Context, in *sysin.RelayRuleInp) error {
	botID, err := s.relayCanonicalBotID(ctx, in.BotId)
	if err != nil {
		return err
	}
	in.BotId = botID
	return relayMutate(ctx, botID, func(ctx context.Context) error { return s.adminRelaySaveRule(ctx, in) })
}
func (s *sSysBot) AdminRelayDeleteRule(ctx context.Context, in *sysin.RelayDeleteInp) error {
	botID, err := s.relayCanonicalBotID(ctx, in.BotId)
	if err != nil {
		return err
	}
	in.BotId = botID
	return relayMutate(ctx, botID, func(ctx context.Context) error { return s.adminRelayDeleteRule(ctx, in) })
}
func (s *sSysBot) AdminRelaySwitch(ctx context.Context, in *sysin.RelaySwitchInp) error {
	botID, err := s.relayCanonicalBotID(ctx, in.BotId)
	if err != nil {
		return err
	}
	in.BotId = botID
	return relayMutate(ctx, botID, func(ctx context.Context) error { return s.adminRelaySwitch(ctx, in) })
}
