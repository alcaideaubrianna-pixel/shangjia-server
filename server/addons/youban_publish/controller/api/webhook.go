package api

import (
	"context"

	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/api/api/webhook"
	"hotgo/addons/youban_publish/service"
)

var Webhook = cWebhook{}

type cWebhook struct{}

func (c *cWebhook) Telegram(ctx context.Context, req *webhook.TelegramReq) (res *webhook.TelegramRes, err error) {
	body := g.RequestFromCtx(ctx).GetBody()
	if len(body) == 0 {
		return nil, gerror.New("Webhook消息不能为空")
	}
	if !gjson.Valid(body) {
		return nil, gerror.New("Webhook消息格式不正确")
	}
	if err = service.SysPublish().TelegramWebhookRaw(ctx, req.BotId, body); err != nil {
		return nil, err
	}
	res = &webhook.TelegramRes{}
	return
}
