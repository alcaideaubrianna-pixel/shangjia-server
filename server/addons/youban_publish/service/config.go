package service

import (
	"context"

	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type ISysConfig interface {
	GetTelegram(ctx context.Context) (conf *model.TelegramConfig, err error)
	GetAccount(ctx context.Context) (conf *model.AccountConfig, err error)
	GetConfigByGroup(ctx context.Context, in *sysin.GetConfigInp) (res *sysin.GetConfigModel, err error)
	UpdateConfigByGroup(ctx context.Context, in *sysin.UpdateConfigInp) error
	PublishConfigView(ctx context.Context, in *sysin.PublishConfigViewInp) (res *sysin.PublishConfigViewModel, err error)
	PublishConfigSave(ctx context.Context, in *sysin.PublishConfigSaveInp) error
	AutoDeleteConfigView(ctx context.Context, in *sysin.AutoDeleteConfigViewInp) (res *sysin.AutoDeleteConfigViewModel, err error)
	AutoDeleteConfigSave(ctx context.Context, in *sysin.AutoDeleteConfigSaveInp) error
	AntiScanConfigView(ctx context.Context, in *sysin.AntiScanConfigViewInp) (res *sysin.AntiScanConfigViewModel, err error)
	AntiScanConfigSave(ctx context.Context, in *sysin.AntiScanConfigSaveInp) error
}

var localSysConfig ISysConfig

func SysConfig() ISysConfig {
	if localSysConfig == nil {
		panic("implement not found for interface ISysConfig, forgot register?")
	}
	return localSysConfig
}

func RegisterSysConfig(i ISysConfig) {
	localSysConfig = i
}
