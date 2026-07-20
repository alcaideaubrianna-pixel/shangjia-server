package sys

import (
	"context"

	"hotgo/addons/youban_two_way_bot/api/admin/bot"
	"hotgo/addons/youban_two_way_bot/service"
)

var Bot = cBot{}

type cBot struct{}

func (c *cBot) List(ctx context.Context, req *bot.ListReq) (res *bot.ListRes, err error) {
	list, totalCount, err := service.SysTwoWayBot().AdminBotList(ctx, &req.BotListInp)
	if err != nil {
		return nil, err
	}
	return &bot.ListRes{List: list, TotalCount: totalCount}, nil
}

func (c *cBot) Save(ctx context.Context, req *bot.SaveReq) (res *bot.SaveRes, err error) {
	if err = service.SysTwoWayBot().AdminBotSave(ctx, &req.BotSaveInp); err != nil {
		return nil, err
	}
	return &bot.SaveRes{}, nil
}

func (c *cBot) Delete(ctx context.Context, req *bot.DeleteReq) (res *bot.DeleteRes, err error) {
	if err = service.SysTwoWayBot().AdminBotDelete(ctx, &req.BotDeleteInp); err != nil {
		return nil, err
	}
	return &bot.DeleteRes{}, nil
}

func (c *cBot) RefreshWebhook(ctx context.Context, req *bot.RefreshWebhookReq) (res *bot.RefreshWebhookRes, err error) {
	if err = service.SysTwoWayBot().AdminBotRefreshWebhook(ctx, &req.BotActionInp); err != nil {
		return nil, err
	}
	return &bot.RefreshWebhookRes{}, nil
}

func (c *cBot) Setup(ctx context.Context, req *bot.SetupReq) (res *bot.SetupRes, err error) {
	if err = service.SysTwoWayBot().AdminBotSetup(ctx, &req.BotActionInp); err != nil {
		return nil, err
	}
	return &bot.SetupRes{}, nil
}
