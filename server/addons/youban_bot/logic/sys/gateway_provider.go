package sys

import (
	"context"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"

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
	if update != nil && update.InlineQuery != nil {
		g.Log().Infof(ctx, "TG链路 youban_bot_inline_provider_enter botId:%d updateId:%d queryId:%s query:%q", binding.ReferenceID, update.ID, update.InlineQuery.ID, strings.TrimSpace(update.InlineQuery.Query))
	}
	return p.bot.handleUpdate(ctx, binding.ReferenceID, update)
}
