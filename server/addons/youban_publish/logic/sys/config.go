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
const publishConfigGroupCloudResource = "cloudResource"

type sSysConfig struct{}

func NewSysConfig() *sSysConfig {
	return &sSysConfig{}
}

func init() {
	service.RegisterSysConfig(NewSysConfig())
}

func (s *sSysConfig) GetTelegram(ctx context.Context) (conf *model.TelegramConfig, err error) {
	in := &sysin.GetConfigInp{}
	in.AddonName = global.GetAddonName()
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
	in.AddonName = global.GetAddonName()
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
	if in.Group == publishConfigGroupCloudResource {
		cloudRes, cloudErr := s.CloudResourceConfigView(ctx, &sysin.CloudResourceConfigViewInp{})
		if cloudErr != nil {
			return nil, cloudErr
		}
		return &sysin.GetConfigModel{List: gconv.Map(cloudRes.CloudResourceConfig)}, nil
	}
	in.AddonName = global.GetAddonName()
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
	if in.Group == publishConfigGroupCloudResource {
		saveInp := &sysin.CloudResourceConfigSaveInp{}
		if err := gconv.Struct(in.List, saveInp); err != nil {
			return err
		}
		return s.CloudResourceConfigSave(ctx, saveInp)
	}
	in.AddonName = global.GetAddonName()
	if err := baseservice.SysAddonsConfig().UpdateConfigByGroup(ctx, &in.UpdateAddonsConfigInp); err != nil {
		return err
	}
	s.clearConfigCache(ctx, in.Group)
	return nil
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

func (s *sSysConfig) CloudResourceConfigView(ctx context.Context, in *sysin.CloudResourceConfigViewInp) (res *sysin.CloudResourceConfigViewModel, err error) {
	conf := defaultCloudResourceConfig()
	if err = s.scanConfigGroup(ctx, publishConfigGroupCloudResource, conf); err != nil {
		return nil, err
	}
	conf.TencentSecretKey = maskSecretValue(conf.TencentSecretKey)
	conf.FapiHubApiKey = maskSecretValue(conf.FapiHubApiKey)
	res = &sysin.CloudResourceConfigViewModel{CloudResourceConfig: conf}
	return
}

func (s *sSysConfig) CloudResourceConfigSave(ctx context.Context, in *sysin.CloudResourceConfigSaveInp) error {
	if in == nil {
		return gerror.New("云资源配置不能为空")
	}
	if err := in.Filter(ctx); err != nil {
		return err
	}
	if strings.Contains(in.TencentSecretKey, "*") {
		oldConf, err := s.GetCloudResource(ctx)
		if err != nil {
			return err
		}
		in.TencentSecretKey = oldConf.TencentSecretKey
	}
	if strings.Contains(in.FapiHubApiKey, "*") {
		oldConf, err := s.GetCloudResource(ctx)
		if err != nil {
			return err
		}
		in.FapiHubApiKey = oldConf.FapiHubApiKey
	}
	if err := validateCloudResourceCredential(ctx, &in.CloudResourceConfig); err != nil {
		return err
	}
	return s.updateConfigGroup(ctx, publishConfigGroupCloudResource, cloudResourceConfigMap(&in.CloudResourceConfig))
}

func (s *sSysConfig) AntiScanConfigView(ctx context.Context, in *sysin.AntiScanConfigViewInp) (res *sysin.AntiScanConfigViewModel, err error) {
	conf := defaultAntiScanConfig()
	if err = s.scanConfigGroup(ctx, publishConfigGroupAntiScan, conf); err != nil {
		return nil, err
	}
	res = &sysin.AntiScanConfigViewModel{AntiScanConfig: conf}
	return
}

func (s *sSysConfig) GetCloudResource(ctx context.Context) (conf *model.CloudResourceConfig, err error) {
	conf = defaultCloudResourceConfig()
	if err = s.scanConfigGroup(ctx, publishConfigGroupCloudResource, conf); err != nil {
		return nil, err
	}
	return conf, nil
}

func (s *sSysConfig) AntiScanConfigSave(ctx context.Context, in *sysin.AntiScanConfigSaveInp) error {
	if in == nil {
		return gerror.New("防扫图配置不能为空")
	}
	if err := in.Filter(ctx); err != nil {
		return err
	}
	if err := s.ensureAntiScanConfigRows(ctx); err != nil {
		return err
	}
	return s.updateConfigGroup(ctx, publishConfigGroupAntiScan, antiScanConfigMap(&in.AntiScanConfig))
}

func (s *sSysConfig) AntiScanConfigSaveTab(ctx context.Context, in *sysin.AntiScanConfigSaveTabInp) error {
	if in == nil {
		return gerror.New("防扫图分栏配置不能为空")
	}
	if err := in.Filter(ctx); err != nil {
		return err
	}
	if err := s.ensureAntiScanConfigRows(ctx); err != nil {
		return err
	}
	return s.updateConfigGroup(ctx, publishConfigGroupAntiScan, antiScanTabConfigMap(in.Tab, &in.AntiScanConfig))
}

func (s *sSysConfig) scanConfigGroup(ctx context.Context, group string, dst interface{}) error {
	in := &sysin.GetConfigInp{}
	in.AddonName = global.GetAddonName()
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
	in.AddonName = global.GetAddonName()
	in.Group = group
	in.List = list
	if err := baseservice.SysAddonsConfig().UpdateConfigByGroup(ctx, &in.UpdateAddonsConfigInp); err != nil {
		return err
	}
	s.clearConfigCache(ctx, group)
	return nil
}

func (s *sSysConfig) clearConfigCache(ctx context.Context, group string) {
	if group == publishConfigGroupAutoDelete {
		clearAutoDeleteDefaultConfigCache(ctx)
	}
}

func defaultPublishConfig() *model.PublishConfig {
	return &model.PublishConfig{
		SkipDownChannelEnabled: 1,
		SendIntervalSeconds:    3,
		SendWindowEnabled:      0,
		SendWindowStart:        "",
		SendWindowEnd:          "",
		FailureStrategy:        "continue",
		RetryEnabled:           1,
		MaxRetryCount:          3,
		RetryIntervalMinutes:   5,
		DefaultAntiScanEnabled: 0,
	}
}

func defaultAntiScanConfig() *model.AntiScanConfig {
	return &model.AntiScanConfig{
		Enabled:                    0,
		DefaultNewNoteEnabled:      0,
		ExistingBatchEnabled:       0,
		ForceBeforeSendEnabled:     0,
		AllowSingleOverrideEnabled: 0,
		MetadataStripEnabled:       0,
		ResizeEnabled:              0,
		ResizeScale:                96,
		CropEnabled:                0,
		CropPercent:                2,
		PortraitBackgroundEnabled:  0,
		BackgroundReplaceEnabled:   0,
		BackgroundBlurEnabled:      0,
		BackgroundTextureEnabled:   0,
		BackgroundTexturePreset:    "rabbit",
		BackgroundTextureImage:     "",
		MaskEnabled:                0,
		MaskMode:                   "qr",
		MaskCount:                  1,
		QrText:                     "",
		StickerOpacity:             18,
		StickerImage:               "",
		MaskItemsJson:              "[]",
		WatermarkEnabled:           0,
		ProfileNoWatermarkEnabled:  0,
		WatermarkFontSize:          22,
		WatermarkOpacity:           28,
		WatermarkText:              "",
		StickerText:                "",
		NoiseEnabled:               0,
		NoiseStrength:              18,
		CompressionEnabled:         0,
		CompressionQuality:         82,
		JpegQualityControlEnabled:  0,
		ColorJitterEnabled:         0,
		ColorJitterStrength:        12,
		SharpenBlurEnabled:         0,
		SharpenBlurMode:            "blur",
		SharpenBlurStrength:        8,
	}
}

func publishConfigMap(conf *model.PublishConfig) g.Map {
	return g.Map{
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

func antiScanConfigMap(conf *model.AntiScanConfig) g.Map {
	return g.Map{
		"antiScanEnabled":            conf.Enabled,
		"defaultNewNoteEnabled":      conf.DefaultNewNoteEnabled,
		"existingBatchEnabled":       conf.ExistingBatchEnabled,
		"forceBeforeSendEnabled":     conf.ForceBeforeSendEnabled,
		"allowSingleOverrideEnabled": conf.AllowSingleOverrideEnabled,
		"metadataStripEnabled":       conf.MetadataStripEnabled,
		"resizeEnabled":              conf.ResizeEnabled,
		"resizeScale":                conf.ResizeScale,
		"cropEnabled":                conf.CropEnabled,
		"cropPercent":                conf.CropPercent,
		"portraitBackgroundEnabled":  conf.PortraitBackgroundEnabled,
		"backgroundReplaceEnabled":   conf.BackgroundReplaceEnabled,
		"backgroundBlurEnabled":      conf.BackgroundBlurEnabled,
		"backgroundTextureEnabled":   conf.BackgroundTextureEnabled,
		"backgroundTexturePreset":    conf.BackgroundTexturePreset,
		"backgroundTextureImage":     conf.BackgroundTextureImage,
		"maskEnabled":                conf.MaskEnabled,
		"maskMode":                   conf.MaskMode,
		"maskCount":                  conf.MaskCount,
		"qrText":                     conf.QrText,
		"stickerOpacity":             conf.StickerOpacity,
		"stickerImage":               conf.StickerImage,
		"maskItemsJson":              conf.MaskItemsJson,
		"watermarkEnabled":           conf.WatermarkEnabled,
		"profileNoWatermarkEnabled":  conf.ProfileNoWatermarkEnabled,
		"watermarkFontSize":          conf.WatermarkFontSize,
		"watermarkOpacity":           conf.WatermarkOpacity,
		"watermarkText":              conf.WatermarkText,
		"stickerText":                conf.StickerText,
		"noiseEnabled":               conf.NoiseEnabled,
		"noiseStrength":              conf.NoiseStrength,
		"compressionEnabled":         conf.CompressionEnabled,
		"compressionQuality":         conf.CompressionQuality,
		"jpegQualityControlEnabled":  conf.JpegQualityControlEnabled,
		"colorJitterEnabled":         conf.ColorJitterEnabled,
		"colorJitterStrength":        conf.ColorJitterStrength,
		"sharpenBlurEnabled":         conf.SharpenBlurEnabled,
		"sharpenBlurMode":            conf.SharpenBlurMode,
		"sharpenBlurStrength":        conf.SharpenBlurStrength,
	}
}

func antiScanTabConfigMap(tab string, conf *model.AntiScanConfig) g.Map {
	switch tab {
	case "basic":
		return g.Map{
			"metadataStripEnabled": conf.MetadataStripEnabled,
			"resizeEnabled":        conf.ResizeEnabled,
			"resizeScale":          conf.ResizeScale,
			"cropEnabled":          conf.CropEnabled,
			"cropPercent":          conf.CropPercent,
			"compressionEnabled":   conf.CompressionEnabled,
			"compressionQuality":   conf.CompressionQuality,
		}
	case "disturbance":
		return g.Map{
			"noiseEnabled":              conf.NoiseEnabled,
			"noiseStrength":             conf.NoiseStrength,
			"jpegQualityControlEnabled": conf.JpegQualityControlEnabled,
			"colorJitterEnabled":        conf.ColorJitterEnabled,
			"colorJitterStrength":       conf.ColorJitterStrength,
			"sharpenBlurEnabled":        conf.SharpenBlurEnabled,
			"sharpenBlurMode":           conf.SharpenBlurMode,
			"sharpenBlurStrength":       conf.SharpenBlurStrength,
		}
	case "mask":
		return g.Map{
			"maskEnabled":    conf.MaskEnabled,
			"maskMode":       conf.MaskMode,
			"maskCount":      conf.MaskCount,
			"qrText":         conf.QrText,
			"stickerOpacity": conf.StickerOpacity,
			"stickerImage":   conf.StickerImage,
			"maskItemsJson":  conf.MaskItemsJson,
			"stickerText":    conf.StickerText,
		}
	case "scope":
		return g.Map{
			"antiScanEnabled":            conf.Enabled,
			"defaultNewNoteEnabled":      conf.DefaultNewNoteEnabled,
			"existingBatchEnabled":       conf.ExistingBatchEnabled,
			"forceBeforeSendEnabled":     conf.ForceBeforeSendEnabled,
			"allowSingleOverrideEnabled": conf.AllowSingleOverrideEnabled,
		}
	case "watermark":
		return g.Map{
			"portraitBackgroundEnabled": conf.PortraitBackgroundEnabled,
			"backgroundReplaceEnabled":  conf.BackgroundReplaceEnabled,
			"backgroundBlurEnabled":     conf.BackgroundBlurEnabled,
			"backgroundTextureEnabled":  conf.BackgroundTextureEnabled,
			"backgroundTexturePreset":   conf.BackgroundTexturePreset,
			"backgroundTextureImage":    conf.BackgroundTextureImage,
			"stickerOpacity":            conf.StickerOpacity,
			"watermarkEnabled":          conf.WatermarkEnabled,
			"profileNoWatermarkEnabled": conf.ProfileNoWatermarkEnabled,
			"watermarkFontSize":         conf.WatermarkFontSize,
			"watermarkOpacity":          conf.WatermarkOpacity,
			"watermarkText":             conf.WatermarkText,
		}
	default:
		return antiScanConfigMap(conf)
	}
}

func maskSecretValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "********"
	}
	return value[:4] + "****" + value[len(value)-4:]
}

func mustConfigJSON(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(data)
}
