package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"

	"hotgo/addons/youban_bot/model/input/sysin"
)

func accountFeatureSchema() []*sysin.FeatureConfigSchema {
	return []*sysin.FeatureConfigSchema{
		{Field: "successText", Label: "成功提示文案", Component: "textarea", Default: "操作成功，请回到页面继续。", Placeholder: "发送验证码成功后的提示"},
		{Field: "failedText", Label: "失败提示文案", Component: "textarea", Default: "验证码不存在或已失效，请在页面重新生成。", Placeholder: "验证码失败时提示"},
	}
}

func simpleTextSchema(label string, defaultText string) []*sysin.FeatureConfigSchema {
	return []*sysin.FeatureConfigSchema{
		{Field: "replyText", Label: label, Component: "textarea", Default: defaultText, Placeholder: "机器人回复文案"},
	}
}

type startFeature struct{}

func (startFeature) Key() string         { return "start" }
func (startFeature) Command() string     { return "start" }
func (startFeature) Description() string { return "开始使用" }
func (startFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return simpleTextSchema("欢迎文案", "欢迎使用悦伴全局机器人。\n\n登录：在网页点击 TG 登录后，发送页面上的 6 位验证码。\n绑定：在个人页面生成绑定码后，发送 6 位验证码。")
}
func (startFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	text := bot.featureConfigValue(ctx, startFeature{}.Key(), "replyText")
	return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), text)
}

type loginFeature struct{}

func (loginFeature) Key() string                                { return "account_login" }
func (loginFeature) Command() string                            { return "login" }
func (loginFeature) Description() string                        { return "账号登录" }
func (loginFeature) ConfigSchema() []*sysin.FeatureConfigSchema { return accountFeatureSchema() }
func (loginFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	code := sixDigitRegexp.FindString(featureCtx.Args)
	if code == "" {
		return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), "请发送：/login 6位验证码")
	}
	return true, bot.consumeCode(ctx, featureCtx.BotId, featureCtx.Msg, code)
}

type bindFeature struct{}

func (bindFeature) Key() string                                { return "account_bind" }
func (bindFeature) Command() string                            { return "bind" }
func (bindFeature) Description() string                        { return "账号绑定" }
func (bindFeature) ConfigSchema() []*sysin.FeatureConfigSchema { return accountFeatureSchema() }
func (bindFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	code := sixDigitRegexp.FindString(featureCtx.Args)
	if code == "" {
		return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), "请发送：/bind 6位验证码")
	}
	return true, bot.consumeCode(ctx, featureCtx.BotId, featureCtx.Msg, code)
}

type notifyFeature struct{}

func (notifyFeature) Key() string         { return "notify" }
func (notifyFeature) Command() string     { return "notify" }
func (notifyFeature) Description() string { return "消息通知" }
func (notifyFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return simpleTextSchema("通知说明文案", "消息通知插件已启用。系统会在任务完成、费用到期等场景主动推送通知。")
}
func (notifyFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	return true, nil
}

type superNotifyFeature struct{}

func (superNotifyFeature) Key() string     { return "super_notify" }
func (superNotifyFeature) Command() string { return "supernotify" }
func (superNotifyFeature) Description() string {
	return "超级通知"
}
func (superNotifyFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return []*sysin.FeatureConfigSchema{
		{Field: "enableRegister", Label: "用户注册通知", Component: "switch", Default: 0},
		{Field: "enableError", Label: "系统错误通知", Component: "switch", Default: 0},
		{Field: "enableBind", Label: "用户绑定通知", Component: "switch", Default: 0},
		{Field: "adminTelegramUserIds", Label: "通知管理员", Component: "bot_admin_user_select", Default: []string{}, Placeholder: "选择已关注机器人且已绑定后台账号的用户"},
	}
}
func (superNotifyFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	return true, nil
}

type adminFeature struct{}

func (adminFeature) Key() string         { return "admin_panel" }
func (adminFeature) Command() string     { return "admin" }
func (adminFeature) Description() string { return "打开管理后台" }
func (adminFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return []*sysin.FeatureConfigSchema{
		{Field: "adminUrl", Label: "管理后台地址", Component: "input", Default: "", Placeholder: "为空读取 youbanBot.adminUrl"},
		{Field: "unboundText", Label: "未绑定提示", Component: "textarea", Default: "请先绑定管理后台账号后再打开管理后台。"},
	}
}
func (adminFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	if featureCtx.Msg == nil || featureCtx.Msg.From == nil {
		return true, nil
	}
	telegramUserId := fmt.Sprintf("%d", featureCtx.Msg.From.ID)
	bind, err := bot.bindingByTelegram(ctx, "admin", telegramUserId)
	if err != nil {
		return true, err
	}
	if bind == nil || bind.AccountId <= 0 {
		return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), bot.featureConfigValue(ctx, adminFeature{}.Key(), "unboundText"))
	}
	url := strings.TrimSpace(bot.featureConfigValue(ctx, adminFeature{}.Key(), "adminUrl"))
	if url == "" {
		url = strings.TrimSpace(gCfgString(ctx, "youbanBot.adminUrl"))
	}
	if url == "" {
		url = "/"
	}
	return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), "管理后台："+url)
}

type contactFeature struct{}

func (contactFeature) Key() string         { return "contact_service" }
func (contactFeature) Command() string     { return "contact" }
func (contactFeature) Description() string { return "联系客服" }
func (contactFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return []*sysin.FeatureConfigSchema{
		{Field: "replyText", Label: "客服文案", Component: "textarea", Default: "请通过下方按钮联系我们。", Placeholder: "点击联系客服时回复的文案"},
		{Field: "buttonLabel", Label: "按钮文案", Component: "input", Default: "联系客服", Placeholder: "例如：联系客服"},
		{Field: "buttonUrl", Label: "按钮链接", Component: "input", Default: "", Placeholder: "例如：https://t.me/xxx"},
	}
}
func (contactFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	text := bot.featureConfigValue(ctx, contactFeature{}.Key(), "replyText")
	buttonLabel := bot.featureConfigValue(ctx, contactFeature{}.Key(), "buttonLabel")
	buttonUrl := bot.featureConfigValue(ctx, contactFeature{}.Key(), "buttonUrl")
	chatId := fmt.Sprintf("%d", featureCtx.Msg.Chat.ID)
	if strings.TrimSpace(buttonUrl) == "" || strings.TrimSpace(buttonLabel) == "" {
		return true, bot.reply(ctx, featureCtx.BotId, chatId, text)
	}
	row, err := bot.botById(ctx, featureCtx.BotId)
	if err != nil {
		return true, err
	}
	_, err = bot.sendMessageWithMarkup(ctx, row.BotToken, chatId, text, "HTML", false, &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: buttonLabel, URL: buttonUrl}}}})
	return true, err
}

type inviteFeature struct{}

func (inviteFeature) Key() string         { return "invite_code" }
func (inviteFeature) Command() string     { return "invite" }
func (inviteFeature) Description() string { return "生成注册邀请码" }
func (inviteFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return simpleTextSchema("邀请码插件文案", "邀请码插件已启用，后续会接入上架端管理员的邀请码生成能力。")
}
func (inviteFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	return true, nil
}

type profileFeature struct{}

func (profileFeature) Key() string         { return "profile_manage" }
func (profileFeature) Command() string     { return "profile" }
func (profileFeature) Description() string { return "资料管理" }
func (profileFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return simpleTextSchema("资料管理文案", "资料管理插件已启用，后续会接入快速上架和快速下架。")
}
func (profileFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	return true, nil
}
