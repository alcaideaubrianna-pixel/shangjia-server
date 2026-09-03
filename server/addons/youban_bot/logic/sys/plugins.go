package sys

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_bot/model/input/sysin"
)

func accountFeatureSchema() []*sysin.FeatureConfigSchema {
	return []*sysin.FeatureConfigSchema{
		{Field: "successText", Label: "成功提示文案", Component: "textarea", Default: "操作成功，请回到页面继续。", Placeholder: "发送验证码成功后的提示"},
		{Field: "failedText", Label: "失败提示文案", Component: "textarea", Default: "验证码不存在或已失效，请在页面重新生成。", Placeholder: "验证码失败时提示"},
	}
}

func bindFeatureSchema() []*sysin.FeatureConfigSchema {
	return []*sysin.FeatureConfigSchema{
		{Field: "successText", Label: "绑定成功文案", Component: "telegram_rich_text", Default: "<b>绑定成功</b>，回到页面即可查看绑定状态。", Placeholder: "支持 Telegram HTML 和模板变量"},
		{Field: "successButtons", Label: "底部按钮", Component: "telegram_buttons", Default: []interface{}{}, Placeholder: "仅支持 URL 按钮"},
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
	return []*sysin.FeatureConfigSchema{
		{Field: "welcomeImage", Label: "欢迎图片", Component: "image_upload_general", Default: "", Placeholder: "支持上传或填写 Telegram 可访问的图片外链，留空则只发送欢迎文案"},
		{Field: "replyText", Label: "欢迎文案", Component: "textarea", Default: "<b>✈️ 小灰机上架机器人</b>\n\n<blockquote>Telegram 上架协作工具，资料管理、频道推送、代理采集和群聊监听，一套系统完成。</blockquote>\n\n<b>功能介绍</b>\n\n• <b>资料管理</b>：创建、编辑、上架、下架\n• <b>资料搜索</b>：支持编号、标题、正文和图片搜索\n• <b>频道推送</b>：全量推送、循环上架、自动删除\n• <b>代理采集</b>：自动采集频道消息并整理资料\n• <b>群聊监听</b>：关键词命中后自动通知\n• <b>快速分发</b>：快速推送资料和媒体内容\n• <b>团队协作</b>：支持管理员、上架账号和客服协同使用\n• <b>双向机器人</b>：统一处理客户咨询和消息转发\n\n<b>开始使用</b>\n\n1. 点击下方「账号绑定」\n2. 绑定你的上架系统账号\n3. 使用菜单进入资料管理或其他功能\n\n<i>当前系统免费开放使用，欢迎体验。</i>", Placeholder: "支持 Telegram HTML 格式的欢迎文案"},
		{Field: "contactAdminId", Label: "管理员 Telegram ID 或用户名", Component: "input", Default: "", Placeholder: "例如：123456789、@admin 或 https://t.me/admin，留空则不显示联系管理员按钮"},
		{Field: "contactButtonLabel", Label: "联系管理员按钮文案", Component: "input", Default: "👨‍💻 联系管理员", Placeholder: "例如：👨‍💻 联系管理员"},
		{Field: "helpChannelUrl", Label: "使用帮助频道", Component: "input", Default: "", Placeholder: "填写 https://t.me/xxx 或 @xxx，留空则不显示使用帮助按钮"},
		{Field: "helpButtonLabel", Label: "使用帮助按钮文案", Component: "input", Default: "📚 使用帮助", Placeholder: "例如：📚 使用帮助"},
	}
}
func (startFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	if handled, err := bot.dispatchInlinePromotionStart(ctx, featureCtx); handled || err != nil {
		return handled, err
	}
	chatID := fmt.Sprintf("%d", featureCtx.Msg.Chat.ID)
	text := startFeatureReplyText(bot, ctx)
	markup := bot.replyKeyboard(ctx)
	imageSource := strings.TrimSpace(bot.featureConfigValue(ctx, startFeature{}.Key(), "welcomeImage"))
	if imageSource != "" {
		if err := bot.replyPhotoWithCaption(ctx, featureCtx.BotId, chatID, imageSource, text, markup); err == nil {
			return true, nil
		} else {
			g.Log().Warningf(ctx, "发送Start欢迎图片失败 botId:%d chatId:%s err:%+v", featureCtx.BotId, chatID, err)
		}
	}
	return true, bot.replyWithMarkup(ctx, featureCtx.BotId, chatID, text, markup)
}

func startFeatureReplyText(bot *sSysBot, ctx context.Context) string {
	key := startFeature{}.Key()
	text := sanitizeTelegramHTML(bot.featureConfigValue(ctx, key, "replyText"))
	links := make([]string, 0, 2)
	adminID := strings.TrimSpace(bot.featureConfigValue(ctx, key, "contactAdminId"))
	adminLabel := strings.TrimSpace(bot.featureConfigValue(ctx, key, "contactButtonLabel"))
	if adminURL := telegramUserURL(adminID); adminURL != "" && adminLabel != "" {
		links = append(links, fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(adminURL), html.EscapeString(adminLabel)))
	}
	helpURL := telegramChannelURL(bot.featureConfigValue(ctx, key, "helpChannelUrl"))
	helpLabel := strings.TrimSpace(bot.featureConfigValue(ctx, key, "helpButtonLabel"))
	if helpURL != "" && helpLabel != "" {
		links = append(links, fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(helpURL), html.EscapeString(helpLabel)))
	}
	if len(links) == 0 {
		return text
	}
	withLinks := strings.TrimSpace(text + "\n\n" + strings.Join(links, "  ·  "))
	if telegramHTMLTextLength(withLinks) > telegramPhotoCaptionMaxLength {
		return text
	}
	return withLinks
}

func telegramUserURL(value string) string {
	if strings.HasPrefix(value, "tg://user?id=") {
		return value
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err == nil && id > 0 {
		return fmt.Sprintf("tg://user?id=%d", id)
	}
	return telegramPublicURL(value)
}

func telegramChannelURL(value string) string {
	return telegramPublicURL(value)
}

func telegramPublicURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "https://t.me/") || strings.HasPrefix(value, "http://t.me/") {
		return value
	}
	if strings.HasPrefix(value, "t.me/") {
		return "https://" + value
	}
	if strings.HasPrefix(value, "@") {
		value = strings.TrimPrefix(value, "@")
	}
	if strings.ContainsAny(value, " /?#") {
		return ""
	}
	return "https://t.me/" + value
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
func (bindFeature) ConfigSchema() []*sysin.FeatureConfigSchema { return bindFeatureSchema() }
func (bindFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	code := sixDigitRegexp.FindString(featureCtx.Args)
	if code == "" {
		return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), "请发送：/bind 6位验证码")
	}
	return true, bot.consumeCode(ctx, featureCtx.BotId, featureCtx.Msg, code)
}

type infoFeature struct{}

func (infoFeature) Key() string                                { return "info" }
func (infoFeature) Command() string                            { return "info" }
func (infoFeature) Description() string                        { return "查看当前聊天信息" }
func (infoFeature) ConfigSchema() []*sysin.FeatureConfigSchema { return nil }
func (infoFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	if featureCtx.Msg == nil || featureCtx.Msg.Chat.Type != "private" || featureCtx.Msg.From == nil {
		return true, nil
	}
	user := featureCtx.Msg.From
	username := "未设置"
	if strings.TrimSpace(user.Username) != "" {
		username = "@" + user.Username
	}
	nickname := strings.TrimSpace(strings.TrimSpace(user.FirstName) + " " + strings.TrimSpace(user.LastName))
	if nickname == "" {
		nickname = "未设置"
	}
	text := fmt.Sprintf("当前为私聊\nChat ID：%d\nUser ID：%d\n用户名：%s\n昵称：%s", featureCtx.Msg.Chat.ID, user.ID, username, nickname)
	telegramUserId := fmt.Sprintf("%d", user.ID)
	for _, app := range []string{sysin.BotAppAdmin, sysin.BotAppApi} {
		bind, err := bot.bindingByTelegram(ctx, app, telegramUserId)
		if err != nil || bind == nil || bind.AccountId <= 0 {
			continue
		}
		account, accountErr := bot.loginBoundAccount(ctx, app, bind.AccountId)
		if accountErr != nil || account == nil {
			continue
		}
		label := "资料账号"
		if app == sysin.BotAppAdmin {
			label = "后台账号"
		}
		text += fmt.Sprintf("\n%s：%s", label, firstInfoValue(account.Username, account.Nickname))
	}
	return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), text)
}

func firstInfoValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "未设置"
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
		{Field: "adminTelegramUserIds", Label: "通知管理员", Component: "bot_admin_user_select", Default: []string{}, Placeholder: "选择超级机器人关注用户（支持多选）"},
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
		{Field: "adminMarkdown", Label: "管理后台文案", Component: "markdown", Default: "", Placeholder: "支持 Markdown，可配置多个域名链接；为空时兼容读取原管理后台地址"},
		{Field: "adminUrl", Label: "旧管理后台地址", Component: "hidden", Default: "", Placeholder: "旧配置兼容字段"},
		{Field: "unboundText", Label: "未绑定提示", Component: "textarea", Default: "请先绑定管理后台账号后再打开管理后台。"},
	}
}
func (adminFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	if featureCtx.Msg == nil || featureCtx.Msg.From == nil {
		return true, nil
	}
	telegramUserId := fmt.Sprintf("%d", featureCtx.Msg.From.ID)
	allowed, err := bot.adminAccessByTelegram(ctx, telegramUserId)
	if err != nil {
		return true, err
	}
	if !allowed {
		if featureCtx.Msg.Chat.Type != "private" {
			return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), "无权限")
		}
		return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), bot.featureConfigValue(ctx, adminFeature{}.Key(), "unboundText"))
	}
	markdown := strings.TrimSpace(bot.featureConfigValue(ctx, adminFeature{}.Key(), "adminMarkdown"))
	if markdown == "" {
		url := strings.TrimSpace(bot.featureConfigValue(ctx, adminFeature{}.Key(), "adminUrl"))
		if url == "" {
			url = strings.TrimSpace(gCfgString(ctx, "youbanBot.adminUrl"))
		}
		if url == "" {
			url = "/"
		}
		markdown = "管理后台：[点击打开](" + url + ")"
	}
	text, err := telegramMarkdownToHTML(markdown)
	if err != nil {
		return true, err
	}
	return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), text)
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
func (inviteFeature) Description() string { return "注册邀请" }
func (inviteFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return []*sysin.FeatureConfigSchema{
		{Field: "replyText", Label: "回复文案", Component: "textarea", Default: "邀请码已生成，请复制下方内容到上架系统注册页使用。", Placeholder: "机器人生成邀请码后的回复文案"},
		{Field: "publishDomain", Label: "上架端域名", Component: "input", Default: "", Placeholder: "例如：https://publish.example.com，留空则只返回相对路径"},
		{Field: "codeLength", Label: "邀请码长度", Component: "input", Default: 6, Placeholder: "默认 6 位字母+数字"},
		{Field: "unboundText", Label: "未绑定提示", Component: "textarea", Default: "请先在个人中心绑定系统账号后再使用。"},
		{Field: "forbiddenText", Label: "无权限提示", Component: "textarea", Default: "仅管理员可生成好友邀请码。"},
	}
}
func (inviteFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	item, err := bot.CreateInviteCode(ctx, &sysin.InviteCreateInp{Source: inviteSourceBot})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "绑定") || strings.Contains(msg, "系统账号") {
			msg = bot.featureConfigValue(ctx, inviteFeature{}.Key(), "unboundText")
		}
		if strings.Contains(err.Error(), "管理员") || strings.Contains(err.Error(), "权限") {
			msg = bot.featureConfigValue(ctx, inviteFeature{}.Key(), "forbiddenText")
		}
		return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), msg)
	}
	text := bot.featureConfigValue(ctx, inviteFeature{}.Key(), "replyText")
	if strings.TrimSpace(text) == "" {
		text = "邀请码已生成，请复制下方内容到上架系统注册页使用。"
	}
	expireText := "-"
	if item.ExpiresAt != nil {
		expireText = item.ExpiresAt.String()
	}
	return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), fmt.Sprintf(
		"%s\n\n邀请码：<code>%s</code>\n有效期至：%s\n注册链接：%s",
		text,
		html.EscapeString(item.Code),
		html.EscapeString(expireText),
		html.EscapeString(item.InviteUrl),
	))
}

type profileFeature struct{}

type collectManageFeature struct{}

func (collectManageFeature) Key() string         { return "collect_manage" }
func (collectManageFeature) Command() string     { return "collect" }
func (collectManageFeature) Description() string { return "采集管理" }
func (collectManageFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return simpleTextSchema("采集管理文案", "采集管理已启用，请选择采集源。")
}
func (collectManageFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	if featureCtx == nil || featureCtx.Msg == nil {
		return true, nil
	}
	account, err := bot.boundProfileAccount(ctx, featureCtx.Msg)
	if err != nil {
		return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), err.Error())
	}
	if !bot.collectManageAllowed(ctx, account) {
		return true, bot.reply(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), "当前租户未开通VIP，无法使用采集管理。")
	}
	return true, bot.showCollectSourceList(ctx, featureCtx.BotId, fmt.Sprintf("%d", featureCtx.Msg.Chat.ID), account, 1)
}

func (profileFeature) Key() string         { return "profile_manage" }
func (profileFeature) Command() string     { return "profile" }
func (profileFeature) Description() string { return "资料管理" }
func (profileFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return simpleTextSchema("资料管理文案", "资料管理插件已启用，后续会接入快速上架和快速下架。")
}
func (profileFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	if featureCtx == nil || featureCtx.Msg == nil {
		return true, nil
	}
	return true, bot.showProfileMenu(ctx, featureCtx.BotId, featureCtx.Msg)
}
