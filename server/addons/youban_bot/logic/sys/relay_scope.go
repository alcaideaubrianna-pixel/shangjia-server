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

// Optional relay failures must not stop the main Bot. No message is claimed
// until a shared, enabled configuration positively identifies this Bot.
func (s *sSysBot) relayScope(ctx context.Context, botID int64) (*entity.YoubanBotRelayConfig, error) {
	enabled, err := s.relayPluginEnabled(ctx)
	if err != nil || !enabled {
		return nil, err
	}
	cfg, err := relayConfig(ctx, botID)
	if err != nil {
		return nil, err
	}
	if cfg == nil || cfg.Enabled != 1 {
		return nil, nil
	}
	return cfg, nil
}
