package sys

import (
	"context"
	"strings"

	"github.com/go-telegram/bot/models"

	chatservice "hotgo/addons/youban_chat/service"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
)

type chatGatewayProvider struct {
	chat *sSysChat
}

func (p *chatGatewayProvider) Name() string { return "youban_chat" }

func (p *chatGatewayProvider) ListEnabledBots(ctx context.Context) ([]gatewayservice.BotBinding, error) {
	rows, err := p.chat.enabledBots(ctx)
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

func (p *chatGatewayProvider) HandleUpdate(ctx context.Context, binding gatewayservice.BotBinding, update *models.Update) error {
	in, err := telegramUpdateToWebhookInp(update)
	if err != nil || in == nil {
		return err
	}
	in.BotId = binding.ReferenceID
	return p.chat.TelegramWebhook(ctx, in)
}

func registerChatGateway(chat *sSysChat) {
	chatservice.RegisterSysChat(chat)
	gatewayservice.RegisterProvider(&chatGatewayProvider{chat: chat})
}
