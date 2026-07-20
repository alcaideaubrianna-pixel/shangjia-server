package service

import (
	"context"

	"hotgo/addons/youban_two_way_bot/model/input/sysin"
)

type ISysTwoWayBot interface {
	StartRuntime(ctx context.Context)
	StopRuntime()
	AdminBotList(ctx context.Context, in *sysin.BotListInp) (list []*sysin.BotModel, totalCount int, err error)
	AdminBotSave(ctx context.Context, in *sysin.BotSaveInp) (err error)
	AdminBotDelete(ctx context.Context, in *sysin.BotDeleteInp) (err error)
	AdminBotRefreshWebhook(ctx context.Context, in *sysin.BotActionInp) (err error)
	AdminBotSetup(ctx context.Context, in *sysin.BotActionInp) (err error)
	TelegramWebhookRaw(ctx context.Context, in *sysin.WebhookInp) (err error)
}

var localSysTwoWayBot ISysTwoWayBot

func SysTwoWayBot() ISysTwoWayBot {
	if localSysTwoWayBot == nil {
		panic("implement not found for interface ISysTwoWayBot, forgot register?")
	}
	return localSysTwoWayBot
}

func RegisterSysTwoWayBot(i ISysTwoWayBot) {
	localSysTwoWayBot = i
}
