package sys

import (
	"context"

	"hotgo/addons/youban_bot/internal/model/entity"
)

// Register the settings entry without enabling any runtime or global Bot menu.
func featureDefaultStatus(feature botFeature) int {
	if feature.Key() == (relayFeature{}).Key() {
		return 2
	}
	return 1
}

// A saved configuration determines identity even while switches are disabled.
func (s *sSysBot) relayIdentity(ctx context.Context, botID int64) (*entity.YoubanBotRelayConfig, error) {
	cfg, err := relayConfig(ctx, botID)
	if err != nil || cfg == nil {
		return cfg, err
	}
	bot, err := s.botById(ctx, botID)
	if err != nil {
		return nil, err
	}
	if bot.IsDefault == 1 {
		return nil, nil
	}
	return cfg, nil
}
func (s *sSysBot) relayRuntimeState(ctx context.Context, cfg *entity.YoubanBotRelayConfig) (bool, string, error) {
	if !relayOwned(cfg) {
		return false, "旧配置未归属租户账户，已隔离停用", nil
	}
	bot, err := s.botById(ctx, cfg.BotId)
	if err != nil {
		return false, "", err
	}
	if bot.IsDefault == 1 {
		return false, "默认主机器人不可用于双向聊天", nil
	}
	if cfg.SourceBotId > 0 {
		valid, err := s.relaySourceBotValid(ctx, cfg)
		if err != nil {
			return false, "", err
		}
		if !valid {
			return false, "后台Bot已停用、删除、变更归属或更换Token，请在后台检查", nil
		}
	}
	if bot.Status != 1 {
		return false, "Bot本身已停用", nil
	}
	enabled, err := s.relayPluginEnabled(ctx)
	if err != nil {
		return false, "", err
	}
	if !enabled {
		return false, "全局双向插件已停用", nil
	}
	if cfg.Enabled != 1 {
		return false, "此Bot的双向功能已停用", nil
	}
	return true, "运行中", nil
}
