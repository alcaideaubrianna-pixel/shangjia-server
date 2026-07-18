// Package api
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package api

import (
	"context"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/lazysheep_tggo/api/api/webhook"
	"hotgo/addons/lazysheep_tggo/service"
)

var Webhook = cWebhook{}

type cWebhook struct{}

func (c *cWebhook) Update(ctx context.Context, req *webhook.UpdateReq) (res *webhook.UpdateRes, err error) {
	request := g.RequestFromCtx(ctx)
	body := request.GetBody()
	secretToken := request.GetHeader("X-Telegram-Bot-Api-Secret-Token")
	err = service.SysLazysheepTggo().HandleWebhook(ctx, req.BotKey, body, secretToken)
	if err != nil {
		return
	}
	res = &webhook.UpdateRes{Ok: true}
	return
}
