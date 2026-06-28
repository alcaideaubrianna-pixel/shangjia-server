package service

import (
	"context"

	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
)

type ISysConfig interface {
	GetTelegram(ctx context.Context) (conf *model.TelegramConfig, err error)
	GetConfigByGroup(ctx context.Context, in *sysin.GetConfigInp) (res *sysin.GetConfigModel, err error)
	UpdateConfigByGroup(ctx context.Context, in *sysin.UpdateConfigInp) error
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
