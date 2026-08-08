package sysin

import (
	"context"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model"
	basesysin "hotgo/internal/model/input/sysin"
)

type GetConfigInp struct {
	basesysin.GetAddonsConfigInp
}

type GetConfigModel struct {
	List g.Map `json:"list"`
}

type UpdateConfigInp struct {
	basesysin.UpdateAddonsConfigInp
}

type PublishConfigViewInp struct{}

type PublishConfigViewModel struct {
	*model.PublishConfig
}

type PublishConfigSaveInp struct {
	model.PublishConfig
}

func (in *PublishConfigSaveInp) Filter(ctx context.Context) error {
	in.SendWindowStart = strings.TrimSpace(in.SendWindowStart)
	in.SendWindowEnd = strings.TrimSpace(in.SendWindowEnd)
	in.FailureStrategy = strings.TrimSpace(in.FailureStrategy)
	if in.FailureStrategy == "" {
		in.FailureStrategy = "continue"
	}
	if in.FailureStrategy != "continue" && in.FailureStrategy != "stop" {
		return gerror.New("失败处理策略不合法")
	}
	if err := checkSwitch(in.SkipDownChannelEnabled, "下架频道推送开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.SendWindowEnabled, "发送时间窗口开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.RetryEnabled, "重试开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.DefaultAntiScanEnabled, "防扫图默认开关"); err != nil {
		return err
	}
	if in.SendIntervalSeconds <= 0 {
		in.SendIntervalSeconds = 3
	}
	if in.SendIntervalSeconds > 3600 {
		return gerror.New("发送间隔不能超过3600秒")
	}
	if err := checkTimeRange(in.SendWindowEnabled, in.SendWindowStart, in.SendWindowEnd); err != nil {
		return err
	}
	if in.MaxRetryCount < 0 || in.MaxRetryCount > 10 {
		return gerror.New("最大重试次数不合法")
	}
	if in.RetryIntervalMinutes <= 0 {
		in.RetryIntervalMinutes = 5
	}
	if in.RetryIntervalMinutes > 1440 {
		return gerror.New("重试间隔不能超过1440分钟")
	}
	return nil
}

type AutoDeleteConfigViewInp struct{}

type AutoDeleteConfigViewModel struct {
	*model.AutoDeleteConfig
}

type AutoDeleteConfigSaveInp struct {
	model.AutoDeleteConfig
}

func (in *AutoDeleteConfigSaveInp) Filter(ctx context.Context) error {
	in.CustomKeywords = uniqueStringsConfig(in.CustomKeywords)
	in.CustomRules = uniqueStringsConfig(in.CustomRules)
	for _, rule := range in.CustomRules {
		if err := validateAutoDeleteRule(rule); err != nil {
			return err
		}
	}
	return nil
}

func validateAutoDeleteRule(rule string) error {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return gerror.New("自动删除规则不能为空")
	}
	if strings.HasPrefix(rule, "single:") {
		rule = strings.TrimSpace(strings.TrimPrefix(rule, "single:"))
	}
	if strings.HasPrefix(rule, "text:") {
		rule = strings.TrimSpace(strings.TrimPrefix(rule, "text:"))
	}
	if rule == "" {
		return gerror.New("自动删除规则不能为空")
	}
	if _, err := regexp.Compile(rule); err != nil {
		return gerror.Newf("自动删除规则格式不合法: %s", err.Error())
	}
	return nil
}

type AntiScanConfigViewInp struct{}

type AntiScanConfigViewModel struct {
	*model.AntiScanConfig
}

type AntiScanConfigSaveInp struct {
	model.AntiScanConfig
}

type AntiScanConfigSaveTabInp struct {
	Tab string `json:"tab"`
	model.AntiScanConfig
}

func (in *AntiScanConfigSaveTabInp) Filter(ctx context.Context) error {
	in.Tab = strings.ToLower(strings.TrimSpace(in.Tab))
	switch in.Tab {
	case "basic", "disturbance", "mask", "scope", "watermark":
	default:
		return gerror.New("防扫图分栏不合法")
	}
	save := &AntiScanConfigSaveInp{AntiScanConfig: in.AntiScanConfig}
	if err := save.Filter(ctx); err != nil {
		return err
	}
	in.AntiScanConfig = save.AntiScanConfig
	return nil
}

func (in *AntiScanConfigSaveInp) Filter(ctx context.Context) error {
	in.MaskMode = strings.ToLower(strings.TrimSpace(in.MaskMode))
	in.QrText = strings.TrimSpace(in.QrText)
	in.StickerImage = strings.TrimSpace(in.StickerImage)
	in.MaskItemsJson = strings.TrimSpace(in.MaskItemsJson)
	in.BackgroundTexturePreset = strings.ToLower(strings.TrimSpace(in.BackgroundTexturePreset))
	in.BackgroundTextureImage = strings.TrimSpace(in.BackgroundTextureImage)
	in.WatermarkText = strings.TrimSpace(in.WatermarkText)
	if in.WatermarkText == "" {
		in.WatermarkText = "xiaohuiji"
	}
	in.StickerText = strings.TrimSpace(in.StickerText)
	if in.MaskMode == "" {
		in.MaskMode = "qr"
	}
	if in.BackgroundTexturePreset == "" {
		in.BackgroundTexturePreset = "rabbit"
	}
	if !isBackgroundTexturePreset(in.BackgroundTexturePreset) {
		return gerror.New("背景纹理预设不合法")
	}
	if in.MaskMode != "qr" && in.MaskMode != "sticker" {
		return gerror.New("打码方式不合法")
	}
	if in.WatermarkFontSize <= 0 {
		in.WatermarkFontSize = 20
	}
	if in.WatermarkFontSize < 12 || in.WatermarkFontSize > 56 {
		return gerror.New("水印字体大小必须在12到56之间")
	}
	if in.WatermarkOpacity <= 0 {
		in.WatermarkOpacity = 20
	}
	if in.WatermarkOpacity < 5 || in.WatermarkOpacity > 80 {
		return gerror.New("水印透明度必须在5到80之间")
	}
	if err := checkSwitch(in.Enabled, "防扫图开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.DefaultNewNoteEnabled, "新笔记默认防扫图开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.ExistingBatchEnabled, "已有资料批量处理开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.ForceBeforeSendEnabled, "发送前强制处理开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.AllowSingleOverrideEnabled, "单条资料覆盖开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.MetadataStripEnabled, "移除图片元信息开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.ResizeEnabled, "尺寸微调开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.CropEnabled, "轻微裁剪开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.PortraitBackgroundEnabled, "人像背景贴图开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.BackgroundReplaceEnabled, "体验替换背景开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.BackgroundBlurEnabled, "背景模糊开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.BackgroundTextureEnabled, "背景纹理叠加开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.MaskEnabled, "内容遮挡开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.WatermarkEnabled, "水印开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.ProfileNoWatermarkEnabled, "资料编号水印开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.NoiseEnabled, "噪点扰动开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.CompressionEnabled, "压缩重采样开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.JpegQualityControlEnabled, "JPEG质量控制开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.ColorJitterEnabled, "色彩轻扰动开关"); err != nil {
		return err
	}
	if err := checkSwitch(in.SharpenBlurEnabled, "锐化模糊微扰开关"); err != nil {
		return err
	}
	if in.ResizeScale <= 0 {
		in.ResizeScale = 96
	}
	if in.ResizeScale < 80 || in.ResizeScale > 100 {
		return gerror.New("尺寸缩放比例必须在80到100之间")
	}
	if in.CropPercent <= 0 {
		in.CropPercent = 2
	}
	if in.CropPercent < 1 || in.CropPercent > 8 {
		return gerror.New("裁剪比例必须在1到8之间")
	}
	if in.MaskCount <= 0 {
		in.MaskCount = 1
	}
	if in.MaskCount > 3 {
		return gerror.New("打码数量不能超过3个")
	}
	if in.StickerOpacity <= 0 {
		in.StickerOpacity = 18
	}
	if in.StickerOpacity > 100 {
		return gerror.New("贴图透明度不能超过100")
	}
	if in.NoiseStrength <= 0 {
		in.NoiseStrength = 18
	}
	if in.NoiseStrength > 60 {
		return gerror.New("噪点强度不能超过60")
	}
	if in.ColorJitterStrength <= 0 {
		in.ColorJitterStrength = 12
	}
	if in.ColorJitterStrength > 40 {
		return gerror.New("色彩扰动强度不能超过40")
	}
	if in.CompressionQuality <= 0 {
		in.CompressionQuality = 82
	}
	if in.CompressionQuality < 60 || in.CompressionQuality > 95 {
		return gerror.New("输出质量必须在60到95之间")
	}
	in.SharpenBlurMode = strings.ToLower(strings.TrimSpace(in.SharpenBlurMode))
	if in.SharpenBlurMode == "" {
		in.SharpenBlurMode = "blur"
	}
	if in.SharpenBlurMode != "blur" && in.SharpenBlurMode != "sharpen" {
		return gerror.New("锐化模糊模式不合法")
	}
	if in.SharpenBlurStrength <= 0 {
		in.SharpenBlurStrength = 8
	}
	if in.SharpenBlurStrength > 30 {
		return gerror.New("锐化模糊强度不能超过30")
	}
	return nil
}

func checkSwitch(value int, name string) error {
	if value != 0 && value != 1 {
		return gerror.Newf("%s不合法", name)
	}
	return nil
}

func isBackgroundTexturePreset(value string) bool {
	switch value {
	case "rabbit", "heart", "dot", "grid":
		return true
	default:
		return false
	}
}

func checkTimeRange(enabled int, start string, end string) error {
	if start != "" && !isTimeHHMM(start) {
		return gerror.New("发送开始时间格式不合法")
	}
	if end != "" && !isTimeHHMM(end) {
		return gerror.New("发送结束时间格式不合法")
	}
	if enabled == 1 {
		if start == "" || end == "" {
			return gerror.New("启用发送时间窗口后，请设置开始和结束时间")
		}
		if start >= end {
			return gerror.New("发送开始时间必须早于结束时间")
		}
	}
	return nil
}

func isTimeHHMM(value string) bool {
	matched, _ := regexp.MatchString(`^([01]\d|2[0-3]):[0-5]\d$`, value)
	return matched
}

func uniquePositiveInt64Config(items []int64) []int64 {
	if len(items) == 0 {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(items))
	list := make([]int64, 0, len(items))
	for _, item := range items {
		if item <= 0 {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		list = append(list, item)
	}
	return list
}

func uniqueStringsConfig(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(items))
	list := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		list = append(list, item)
	}
	return list
}
