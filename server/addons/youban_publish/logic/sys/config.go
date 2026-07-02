package sys

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/gconv"

	"hotgo/addons/youban_publish/global"
	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/addons/youban_publish/service"
	baseservice "hotgo/internal/service"
)

const publishConfigGroupTelegram = "telegram"
const publishConfigGroupAccount = "account"
const publishConfigGroupPublish = "publish"
const publishConfigGroupAutoDelete = "autoDelete"
const publishConfigGroupAntiScan = "antiScan"

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

func (s *sSysConfig) PublishConfigView(ctx context.Context, in *sysin.PublishConfigViewInp) (res *sysin.PublishConfigViewModel, err error) {
	conf := defaultPublishConfig()
	if err = s.scanConfigGroup(ctx, publishConfigGroupPublish, conf); err != nil {
		return nil, err
	}
	res = &sysin.PublishConfigViewModel{PublishConfig: conf}
	return
}

func (s *sSysConfig) PublishConfigSave(ctx context.Context, in *sysin.PublishConfigSaveInp) error {
	if in == nil {
		return gerror.New("发布策略配置不能为空")
	}
	if err := in.Filter(ctx); err != nil {
		return err
	}
	return s.updateConfigGroup(ctx, publishConfigGroupPublish, publishConfigMap(&in.PublishConfig))
}

func (s *sSysConfig) AutoDeleteConfigView(ctx context.Context, in *sysin.AutoDeleteConfigViewInp) (res *sysin.AutoDeleteConfigViewModel, err error) {
	conf := defaultAutoDeleteConfig()
	if err = s.scanConfigGroup(ctx, publishConfigGroupAutoDelete, conf); err != nil {
		return nil, err
	}
	res = &sysin.AutoDeleteConfigViewModel{AutoDeleteConfig: conf}
	return
}

func (s *sSysConfig) AutoDeleteConfigSave(ctx context.Context, in *sysin.AutoDeleteConfigSaveInp) error {
	if in == nil {
		return gerror.New("频道自动删除配置不能为空")
	}
	if err := in.Filter(ctx); err != nil {
		return err
	}
	return s.updateConfigGroup(ctx, publishConfigGroupAutoDelete, autoDeleteConfigMap(&in.AutoDeleteConfig))
}

func (s *sSysConfig) AntiScanConfigView(ctx context.Context, in *sysin.AntiScanConfigViewInp) (res *sysin.AntiScanConfigViewModel, err error) {
	conf := defaultAntiScanConfig()
	if err = s.scanConfigGroup(ctx, publishConfigGroupAntiScan, conf); err != nil {
		return nil, err
	}
	res = &sysin.AntiScanConfigViewModel{AntiScanConfig: conf}
	return
}

func (s *sSysConfig) AntiScanConfigSave(ctx context.Context, in *sysin.AntiScanConfigSaveInp) error {
	if in == nil {
		return gerror.New("防扫图配置不能为空")
	}
	if err := in.Filter(ctx); err != nil {
		return err
	}
	return s.updateConfigGroup(ctx, publishConfigGroupAntiScan, antiScanConfigMap(&in.AntiScanConfig))
}

func (s *sSysConfig) scanConfigGroup(ctx context.Context, group string, dst interface{}) error {
	in := &sysin.GetConfigInp{}
	in.AddonName = global.GetSkeleton().Name
	in.Group = group
	res, err := baseservice.SysAddonsConfig().GetConfigByGroup(ctx, &in.GetAddonsConfigInp)
	if err != nil {
		return err
	}
	if res == nil || len(res.List) == 0 {
		return nil
	}
	return gconv.Struct(res.List, dst)
}

func (s *sSysConfig) updateConfigGroup(ctx context.Context, group string, list g.Map) error {
	in := &sysin.UpdateConfigInp{}
	in.AddonName = global.GetSkeleton().Name
	in.Group = group
	in.List = list
	return baseservice.SysAddonsConfig().UpdateConfigByGroup(ctx, &in.UpdateAddonsConfigInp)
}

func defaultPublishConfig() *model.PublishConfig {
	return &model.PublishConfig{
		CyclePublishEnabled:    0,
		CyclePublishDays:       4,
		CyclePublishTime:       "09:00",
		SkipDownChannelEnabled: 1,
		SendIntervalSeconds:    3,
		SendWindowEnabled:      0,
		SendWindowStart:        "",
		SendWindowEnd:          "",
		FailureStrategy:        "continue",
		RetryEnabled:           1,
		MaxRetryCount:          3,
		RetryIntervalMinutes:   5,
		DefaultAntiScanEnabled: 1,
	}
}

func defaultAutoDeleteConfig() *model.AutoDeleteConfig {
	return &model.AutoDeleteConfig{
		Enabled:  0,
		BotIds:   []int64{},
		Keywords: []string{},
	}
}

func defaultAntiScanConfig() *model.AntiScanConfig {
	return &model.AntiScanConfig{
		Enabled:                   1,
		DefaultNewNoteEnabled:     1,
		MetadataStripEnabled:      1,
		PortraitBackgroundEnabled: 1,
		BackgroundReplaceEnabled:  0,
		MaskMode:                  "qr",
		MaskCount:                 1,
		QrText:                    "仅供本频道查看",
		StickerOpacity:            18,
		StickerImage:              "",
		WatermarkEnabled:          1,
		WatermarkText:             "youban",
		StickerText:               "",
		NoiseEnabled:              1,
		NoiseStrength:             18,
		CompressionEnabled:        1,
		CompressionQuality:        82,
		ColorJitterEnabled:        1,
	}
}

func publishConfigMap(conf *model.PublishConfig) g.Map {
	return g.Map{
		"cyclePublishEnabled":    conf.CyclePublishEnabled,
		"cyclePublishDays":       conf.CyclePublishDays,
		"cyclePublishTime":       conf.CyclePublishTime,
		"skipDownChannelEnabled": conf.SkipDownChannelEnabled,
		"sendIntervalSeconds":    conf.SendIntervalSeconds,
		"sendWindowEnabled":      conf.SendWindowEnabled,
		"sendWindowStart":        conf.SendWindowStart,
		"sendWindowEnd":          conf.SendWindowEnd,
		"failureStrategy":        conf.FailureStrategy,
		"retryEnabled":           conf.RetryEnabled,
		"maxRetryCount":          conf.MaxRetryCount,
		"retryIntervalMinutes":   conf.RetryIntervalMinutes,
		"defaultAntiScanEnabled": conf.DefaultAntiScanEnabled,
	}
}

func autoDeleteConfigMap(conf *model.AutoDeleteConfig) g.Map {
	return g.Map{
		"autoDeleteEnabled": conf.Enabled,
		"botIds":            mustConfigJSON(conf.BotIds),
		"keywords":          mustConfigJSON(conf.Keywords),
	}
}

func antiScanConfigMap(conf *model.AntiScanConfig) g.Map {
	return g.Map{
		"antiScanEnabled":           conf.Enabled,
		"defaultNewNoteEnabled":     conf.DefaultNewNoteEnabled,
		"metadataStripEnabled":      conf.MetadataStripEnabled,
		"portraitBackgroundEnabled": conf.PortraitBackgroundEnabled,
		"backgroundReplaceEnabled":  conf.BackgroundReplaceEnabled,
		"maskMode":                  conf.MaskMode,
		"maskCount":                 conf.MaskCount,
		"qrText":                    conf.QrText,
		"stickerOpacity":            conf.StickerOpacity,
		"stickerImage":              conf.StickerImage,
		"watermarkEnabled":          conf.WatermarkEnabled,
		"watermarkText":             conf.WatermarkText,
		"stickerText":               conf.StickerText,
		"noiseEnabled":              conf.NoiseEnabled,
		"noiseStrength":             conf.NoiseStrength,
		"compressionEnabled":        conf.CompressionEnabled,
		"compressionQuality":        conf.CompressionQuality,
		"colorJitterEnabled":        conf.ColorJitterEnabled,
	}
}

func mustConfigJSON(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(data)
}
