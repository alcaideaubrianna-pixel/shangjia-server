package service

import (
	"context"
	"sync"

	"hotgo/addons/youban_bot/model/input/sysin"
)

type AccountBoundEvent struct {
	App              string
	AccountId        int64
	BotId            int64
	TelegramUserId   string
	TelegramUsername string
}

type AccountBoundHook func(ctx context.Context, event *AccountBoundEvent) error

var accountBoundHooks struct {
	sync.RWMutex
	list []AccountBoundHook
}

func RegisterAccountBoundHook(hook AccountBoundHook) {
	if hook == nil {
		return
	}
	accountBoundHooks.Lock()
	accountBoundHooks.list = append(accountBoundHooks.list, hook)
	accountBoundHooks.Unlock()
}

func TriggerAccountBoundHooks(ctx context.Context, event *AccountBoundEvent) []error {
	accountBoundHooks.RLock()
	hooks := append([]AccountBoundHook(nil), accountBoundHooks.list...)
	accountBoundHooks.RUnlock()
	errs := make([]error, 0)
	for _, hook := range hooks {
		if err := hook(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

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
	AdminAccountBindList(ctx context.Context, in *sysin.AccountBindListInp) (list []*sysin.AccountBindModel, totalCount int, err error)
	AdminAccountBindUnbind(ctx context.Context, in *sysin.AccountBindUnbindInp) (err error)
	AdminMessageList(ctx context.Context, in *sysin.MessageListInp) (list []*sysin.MessageModel, totalCount int, err error)
	AdminBotChannelCacheList(ctx context.Context, in *sysin.BotChannelCacheListInp) (list []*sysin.BotChannelCacheModel, totalCount int, err error)
	AdminUserSwitchSuperAdmin(ctx context.Context, in *sysin.UserSwitchSuperAdminInp) (err error)
	AdminSendMessage(ctx context.Context, in *sysin.SendMessageInp) (err error)
	NotifySuperAdmins(ctx context.Context, botId int64, scene string, text string) (err error)
	LoginCodeStart(ctx context.Context, in *sysin.CodeStartInp) (res *sysin.CodeStartModel, err error)
	LoginCodeStatus(ctx context.Context, in *sysin.CodeStatusInp) (res *sysin.CodeStatusModel, err error)
	BindCodeStart(ctx context.Context) (res *sysin.CodeStartModel, err error)
	BindCodeStatus(ctx context.Context, in *sysin.CodeStatusInp) (res *sysin.CodeStatusModel, err error)
	BindInfo(ctx context.Context) (res *sysin.BindInfoModel, err error)
	ResolveCustomEmojis(ctx context.Context, in *sysin.CustomEmojiResolveInp) (list []*sysin.CustomEmojiModel, err error)
	MyInviteInfo(ctx context.Context) (res *sysin.InviteInfoModel, err error)
	MyInviteList(ctx context.Context, in *sysin.InviteListInp) (list []*sysin.InviteModel, totalCount int, err error)
	CreateInviteCode(ctx context.Context, in *sysin.InviteCreateInp) (res *sysin.InviteCreateModel, err error)
	TelegramWebhookRaw(ctx context.Context, in *sysin.WebhookInp) (err error)
	Notify(ctx context.Context, in *sysin.NotifyInp) (err error)
	NotifyAccount(ctx context.Context, in *sysin.NotifyAccountInp) (err error)
	NotifyAccounts(ctx context.Context, in *sysin.NotifyAccountsInp) (err error)
	NotifyRich(ctx context.Context, in *sysin.NotifyRichInp) (err error)
	CopyStoredMessages(ctx context.Context, in *sysin.StoredMessageCopyInp) (res *sysin.StoredMessageCopyModel, err error)
	StoredMessageSource(ctx context.Context, ids []int64) (res *sysin.StoredMessageSourceModel, err error)
	RetainStoredMessages(ctx context.Context, ids []int64) (err error)
	ReleaseStoredMessages(ctx context.Context, ids []int64) (err error)
	OfficialBotToken(ctx context.Context) (token string, err error)
}

var localSysBot ISysBot

func SysBot() ISysBot {
	if localSysBot == nil {
		panic("implement not found for interface ISysBot, forgot register?")
	}
	return localSysBot
}

func RegisterSysBot(i ISysBot) { localSysBot = i }
