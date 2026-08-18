package sys

import (
	"context"
	"strings"

	"github.com/go-telegram/bot/models"

	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
)

type botGatewayProvider struct {
	bot *sSysBot
}

func (p *botGatewayProvider) Name() string { return "youban_bot" }

func (p *botGatewayProvider) ListEnabledBots(ctx context.Context) ([]gatewayservice.BotBinding, error) {
	rows, err := p.bot.enabledBots(ctx)
	if err != nil {
		return nil, err
	}
	bindings := make([]gatewayservice.BotBinding, 0, len(rows))
	for _, row := range rows {
		if row == nil || strings.TrimSpace(row.BotToken) == "" {
			continue
		}
		bindings = append(bindings, gatewayservice.BotBinding{
			Owner:       p.Name(),
			ReferenceID: row.Id,
			Token:       strings.TrimSpace(row.BotToken),
		})
	}
	return bindings, nil
}

func (p *botGatewayProvider) HandleUpdate(ctx context.Context, binding gatewayservice.BotBinding, update *models.Update) error {
	return p.bot.handleUpdate(ctx, binding.ReferenceID, update)
}
