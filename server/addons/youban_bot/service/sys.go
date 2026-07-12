package service

import (
	"context"

	"hotgo/addons/youban_bot/model/input/sysin"
)

type ISysBot interface {
	StartRuntime(ctx context.Context)
	StopRuntime()
	AdminBotList(ctx context.Context, in *sysin.BotListInp) (list []*sysin.BotModel, totalCount int, err error)
	AdminBotSave(ctx context.Context, in *sysin.BotSaveInp) (err error)
	AdminBotDelete(ctx context.Context, in *sysin.BotDeleteInp) (err error)
	AdminBotRefresh(ctx context.Context, in *sysin.BotRefreshInp) (list []*sysin.BotRefreshModel, err error)
	AdminBotRestart(ctx context.Context, in *sysin.BotRefreshInp) (list []*sysin.BotRefreshModel, err error)
	AdminFeatureList(ctx context.Context, in *sysin.FeatureListInp) (list []*sysin.FeatureModel, totalCount int, err error)
	AdminFeatureSave(ctx context.Context, in *sysin.FeatureSaveInp) (err error)
	AdminUserList(ctx context.Context, in *sysin.UserListInp) (list []*sysin.UserModel, totalCount int, err error)
	AdminMessageList(ctx context.Context, in *sysin.MessageListInp) (list []*sysin.MessageModel, totalCount int, err error)
	AdminUserSwitchSuperAdmin(ctx context.Context, in *sysin.UserSwitchSuperAdminInp) (err error)
	AdminSendMessage(ctx context.Context, in *sysin.SendMessageInp) (err error)
	LoginCodeStart(ctx context.Context, in *sysin.CodeStartInp) (res *sysin.CodeStartModel, err error)
	LoginCodeStatus(ctx context.Context, in *sysin.CodeStatusInp) (res *sysin.CodeStatusModel, err error)
	BindCodeStart(ctx context.Context) (res *sysin.CodeStartModel, err error)
	BindCodeStatus(ctx context.Context, in *sysin.CodeStatusInp) (res *sysin.CodeStatusModel, err error)
	BindInfo(ctx context.Context) (res *sysin.BindInfoModel, err error)
	TelegramWebhookRaw(ctx context.Context, in *sysin.WebhookInp) (err error)
	Notify(ctx context.Context, in *sysin.NotifyInp) (err error)
}

var localSysBot ISysBot

func SysBot() ISysBot {
	if localSysBot == nil {
		panic("implement not found for interface ISysBot, forgot register?")
	}
	return localSysBot
}

func RegisterSysBot(i ISysBot) { localSysBot = i }
