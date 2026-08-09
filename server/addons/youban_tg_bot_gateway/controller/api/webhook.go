package api

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_tg_bot_gateway/api/api/webhook"
	"hotgo/addons/youban_tg_bot_gateway/service"
)

var Webhook = cWebhook{}

type cWebhook struct{}

func (c *cWebhook) Telegram(ctx context.Context, req *webhook.TelegramReq) (*webhook.TelegramRes, error) {
	request := g.RequestFromCtx(ctx)
	body := request.GetBody()
	secret := request.Header.Get("X-Telegram-Bot-Api-Secret-Token")
	if err := service.Gateway().Webhook(ctx, req.Key, body, secret); err != nil {
		return nil, err
	}
	return &webhook.TelegramRes{}, nil
}
