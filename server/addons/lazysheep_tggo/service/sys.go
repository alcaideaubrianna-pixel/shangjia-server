// ====================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// ====================================================================

package service

import (
	"context"
	"hotgo/addons/lazysheep_tggo/model"
	"hotgo/addons/lazysheep_tggo/model/input/sysin"
)

type (
	ILazySheepTGGo interface {
		Install(ctx context.Context) error
		Upgrade(ctx context.Context) error
		UnInstall(ctx context.Context) error
		BootBots(ctx context.Context) error
		StartAutoPullLoop(ctx context.Context)
		StartPullMonitorAggregator(ctx context.Context)
		StartPushQueueLoop(ctx context.Context)
		HandleAutoPullTask(ctx context.Context, task *sysin.AutoPullTask) error
		DispatchPushNoteTask(ctx context.Context, task *sysin.PushNoteTask)
		HandlePushNoteTask(ctx context.Context, task *sysin.PushNoteTask) error

		GetState(ctx context.Context) (res *model.State, err error)
		SaveState(ctx context.Context, state *model.State) error
		SaveConfig(ctx context.Context, in *sysin.UpdateConfigInp) error
		ChannelList(ctx context.Context, in *sysin.ChannelListInp) (res *sysin.ChannelListModel, err error)
		InspectBot(ctx context.Context, in *sysin.BotInspectInp) (res *sysin.BotInspectModel, err error)
		DeleteBot(ctx context.Context, in *sysin.BotDeleteInp) error
		StartBot(ctx context.Context, in *sysin.BotStartInp) error
		BotUsers(ctx context.Context, in *sysin.BotUserListInp) (list []*sysin.BotUserListModel, err error)
		UpdateBotUser(ctx context.Context, in *sysin.BotUserEditInp) error
		TestTelegramProxy(ctx context.Context, in *sysin.TelegramProxyTestInp) (res *sysin.TelegramProxyTestModel, err error)
		PullMonitor(ctx context.Context, in *sysin.PullMonitorInp) (res *sysin.PullMonitorModel, err error)
		PushQueueMonitor(ctx context.Context, in *sysin.PushQueueMonitorInp) (res *sysin.PushQueueMonitorModel, err error)
		UpdatePushQueueControl(ctx context.Context, in *sysin.PushQueueControlInp) error
		UpdateBindingAutoPull(ctx context.Context, in *sysin.BindingAutoPullControlInp) error
		TouchUser(ctx context.Context, in *sysin.TouchUserInp) error
		IsBotAdmin(ctx context.Context, botKey string, telegramID int64) (bool, error)
		UpsertBot(ctx context.Context, in *sysin.BotUpsertInp) (key string, err error)
		BindSource(ctx context.Context, in *sysin.BindSourceInp) error
		PullNow(ctx context.Context, in *sysin.PullInp) (message string, err error)
		PauseBindingWork(ctx context.Context, botKey string, chatID int64) (message string, err error)
		ResetBindingPull(ctx context.Context, botKey string, chatID int64) (message string, err error)
		ClearBindingNotes(ctx context.Context, botKey string, chatID int64) (message string, err error)
		SetBindingPublishChat(ctx context.Context, botKey string, chatID int64) (message string, err error)
		NotifyBindingCreated(ctx context.Context, botKey string, chatID int64, sourceURL string, operatorID int64, mode string) error
		SignIn(ctx context.Context, in *sysin.SignInInp) (message string, err error)
		StoreNote(ctx context.Context, in *sysin.NoteStoreInp) (res *sysin.NoteStoreModel, err error)

		GetRuntime(ctx context.Context, botKey string) (rt *model.Runtime, err error)
		SyncBot(ctx context.Context, botKey string) error
		SyncAllBots(ctx context.Context) error
		HandleWebhook(ctx context.Context, botKey string, payload []byte, secretToken string) error
		SetWebhook(ctx context.Context, botKey, webhookURL string) error
	}
)

var localLazySheepTGGo ILazySheepTGGo

func SysLazysheepTggo() ILazySheepTGGo {
	if localLazySheepTGGo == nil {
		panic("implement not found for interface ILazySheepTGGo, forgot register?")
	}
	return localLazySheepTGGo
}

func RegisterSysLazysheepTggo(i ILazySheepTGGo) {
	localLazySheepTGGo = i
}
