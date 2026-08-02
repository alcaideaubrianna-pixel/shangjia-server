package api

import (
	"context"

	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	botapi "hotgo/addons/youban_bot/api/api/bot"
	"hotgo/addons/youban_bot/model/input/sysin"
	"hotgo/addons/youban_bot/service"
)

var BotPublic = cBotPublic{}
var BotAuth = cBotAuth{}

type cBotPublic struct{}
type cBotAuth struct{}

func (c *cBotPublic) LoginStart(ctx context.Context, req *botapi.LoginStartReq) (res *botapi.LoginStartRes, err error) {
	out, err := service.SysBot().LoginCodeStart(ctx, &req.CodeStartInp)
	if err != nil {
		return
	}
	res = &botapi.LoginStartRes{CodeStartModel: out}
	return
}

func (c *cBotPublic) LoginStatus(ctx context.Context, req *botapi.LoginStatusReq) (res *botapi.LoginStatusRes, err error) {
	out, err := service.SysBot().LoginCodeStatus(ctx, &req.CodeStatusInp)
	if err != nil {
		return
	}
	res = &botapi.LoginStatusRes{CodeStatusModel: out}
	return
}

func (c *cBotPublic) TelegramWebhook(ctx context.Context, req *botapi.TelegramWebhookReq) (res *botapi.TelegramWebhookRes, err error) {
	body := g.RequestFromCtx(ctx).GetBody()
	if len(body) == 0 {
		return nil, gerror.New("Webhook消息不能为空")
	}
	if !gjson.Valid(body) {
		return nil, gerror.New("Webhook消息格式不正确")
	}
	botId := req.BotId
	if botId <= 0 {
		botId = g.RequestFromCtx(ctx).Get("botId").Int64()
	}
	g.Log().Infof(ctx, "收到Telegram Webhook botId:%d bodyLen:%d", botId, len(body))
	if err = service.SysBot().TelegramWebhookRaw(ctx, &sysin.WebhookInp{BotId: botId, Body: body}); err != nil {
		return nil, err
	}
	res = &botapi.TelegramWebhookRes{}
	return
}

func (c *cBotAuth) BindStart(ctx context.Context, req *botapi.BindStartReq) (res *botapi.BindStartRes, err error) {
	out, err := service.SysBot().BindCodeStart(ctx)
	if err != nil {
		return
	}
	res = &botapi.BindStartRes{CodeStartModel: out}
	return
}

func (c *cBotAuth) BindStatus(ctx context.Context, req *botapi.BindStatusReq) (res *botapi.BindStatusRes, err error) {
	out, err := service.SysBot().BindCodeStatus(ctx, &req.CodeStatusInp)
	if err != nil {
		return
	}
	res = &botapi.BindStatusRes{CodeStatusModel: out}
	return
}

func (c *cBotAuth) BindInfo(ctx context.Context, req *botapi.BindInfoReq) (res *botapi.BindInfoRes, err error) {
	out, err := service.SysBot().BindInfo(ctx)
	if err != nil {
		return
	}
	res = &botapi.BindInfoRes{BindInfoModel: out}
	return
}

func (c *cBotAuth) CustomEmojiResolve(ctx context.Context, req *botapi.CustomEmojiResolveReq) (res *botapi.CustomEmojiResolveRes, err error) {
	list, err := service.SysBot().ResolveCustomEmojis(ctx, &req.CustomEmojiResolveInp)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []*sysin.CustomEmojiModel{}
	}
	return &botapi.CustomEmojiResolveRes{List: list}, nil
}
