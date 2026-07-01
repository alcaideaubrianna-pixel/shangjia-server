package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/util/gconv"

	"hotgo/addons/youban_publish/global"
	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
	baseservice "hotgo/internal/service"
)

const publishConfigGroupTelegram = "telegram"
const publishConfigGroupAccount = "account"

type sSysConfig struct{}

func NewSysConfig() *sSysConfig {
	return &sSysConfig{}
}

func init() {
	service.RegisterSysConfig(NewSysConfig())
}

func (s *sSysConfig) GetTelegram(ctx context.Context) (conf *model.TelegramConfig, err error) {
	in := &sysin.GetConfigInp{}
	in.AddonName = global.GetSkeleton().Name
	in.Group = publishConfigGroupTelegram
	res, err := baseservice.SysAddonsConfig().GetConfigByGroup(ctx, &in.GetAddonsConfigInp)
	if err != nil {
		return nil, err
	}
	conf = &model.TelegramConfig{}
	if err = gconv.Struct(res.List, conf); err != nil {
		return nil, err
	}
	conf.ProxyUrl = strings.TrimSpace(conf.ProxyUrl)
	conf.BotRuntimeMode = strings.ToLower(strings.TrimSpace(conf.BotRuntimeMode))
	conf.WebhookBaseUrl = strings.TrimRight(strings.TrimSpace(conf.WebhookBaseUrl), "/")
	if conf.WebhookBaseUrl == "" {
		if basic, basicErr := baseservice.SysConfig().GetBasic(ctx); basicErr == nil && basic != nil && strings.TrimSpace(basic.Domain) != "" {
			conf.WebhookBaseUrl = strings.TrimRight(strings.TrimSpace(basic.Domain), "/")
		}
	}
	conf.WebhookSecret = strings.TrimSpace(conf.WebhookSecret)
	conf.DefaultTargetChat = strings.TrimSpace(conf.DefaultTargetChat)
	return conf, nil
}

func (s *sSysConfig) GetAccount(ctx context.Context) (conf *model.AccountConfig, err error) {
	in := &sysin.GetConfigInp{}
	in.AddonName = global.GetSkeleton().Name
	in.Group = publishConfigGroupAccount
	res, err := baseservice.SysAddonsConfig().GetConfigByGroup(ctx, &in.GetAddonsConfigInp)
	if err != nil {
		return nil, err
	}
	conf = &model.AccountConfig{
		DefaultRoleId: 10,
		DefaultDeptId: 1,
	}
	if err = gconv.Struct(res.List, conf); err != nil {
		return nil, err
	}
	if conf.DefaultRoleId <= 0 {
		conf.DefaultRoleId = 10
	}
	if conf.DefaultDeptId <= 0 {
		conf.DefaultDeptId = 1
	}
	return conf, nil
}

func (s *sSysConfig) GetConfigByGroup(ctx context.Context, in *sysin.GetConfigInp) (res *sysin.GetConfigModel, err error) {
	if in.Group == "" {
		in.Group = publishConfigGroupTelegram
	}
	in.AddonName = global.GetSkeleton().Name
	models, err := baseservice.SysAddonsConfig().GetConfigByGroup(ctx, &in.GetAddonsConfigInp)
	if err != nil {
		return nil, err
	}
	res = &sysin.GetConfigModel{List: models.List}
	return res, nil
}

func (s *sSysConfig) UpdateConfigByGroup(ctx context.Context, in *sysin.UpdateConfigInp) error {
	if in.Group == "" {
		in.Group = publishConfigGroupTelegram
	}
	in.AddonName = global.GetSkeleton().Name
	return baseservice.SysAddonsConfig().UpdateConfigByGroup(ctx, &in.UpdateAddonsConfigInp)
}
