package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
	"hotgo/internal/library/cache"

	"hotgo/addons/youban_bot/model/input/sysin"
)

type startFeature struct{}

func (startFeature) Key() string         { return "start" }
func (startFeature) Command() string     { return "start" }
func (startFeature) Description() string { return "开始使用" }
func (startFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return append([]*sysin.FeatureConfigSchema{
		{Field: "welcomeImage", Label: "欢迎图片", Component: "image_upload_general", Default: "", Placeholder: "支持上传或填写 Telegram 可访问的图片外链，留空则只发送欢迎文案"},
		{Field: "replyText", Label: "欢迎文案", Component: "textarea", Default: "<b>✈️ 小灰机上架机器人</b>\n\n<blockquote>Telegram 上架协作工具，资料管理、频道推送、代理采集和群聊监听，一套系统完成。</blockquote>\n\n<b>功能介绍</b>\n\n• <b>资料管理</b>：创建、编辑、上架、下架\n• <b>资料搜索</b>：支持编号、标题、正文和图片搜索\n• <b>频道推送</b>：全量推送、循环上架、自动删除\n• <b>代理采集</b>：自动采集频道消息并整理资料\n• <b>群聊监听</b>：关键词命中后自动通知\n• <b>快速分发</b>：快速推送资料和媒体内容\n• <b>团队协作</b>：支持管理员、上架账号和客服协同使用\n• <b>双向机器人</b>：统一处理客户咨询和消息转发\n\n<b>开始使用</b>\n\n1. 点击下方「账号绑定」\n2. 绑定你的上架系统账号\n3. 使用菜单进入资料管理或其他功能\n\n<i>当前系统免费开放使用，欢迎体验。</i>", Placeholder: "支持 Telegram HTML 格式的欢迎文案"},
		{Field: "contactAdminId", Label: "管理员 Telegram ID 或用户名", Component: "input", Default: "", Placeholder: "例如：123456789、@admin 或 https://t.me/admin，留空则不显示联系管理员按钮"},
		{Field: "contactButtonLabel", Label: "联系管理员按钮文案", Component: "input", Default: "👨‍💻 联系管理员", Placeholder: "例如：👨‍💻 联系管理员"},
		{Field: "helpChannelUrl", Label: "使用帮助频道", Component: "input", Default: "", Placeholder: "填写 https://t.me/xxx 或 @xxx，留空则不显示使用帮助按钮"},
		{Field: "helpButtonLabel", Label: "使用帮助按钮文案", Component: "input", Default: "📚 使用帮助", Placeholder: "例如：📚 使用帮助"},
	}, replyKeyboardConfigSchema()...)
}
func (startFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	if strings.HasPrefix(strings.TrimSpace(featureCtx.Args), accountAuthorizationPrefix) {
		return true, bot.beginAccountAuthorization(ctx, featureCtx)
	}
	if strings.HasPrefix(strings.TrimSpace(featureCtx.Args), "chat_profile_") {
		return true, bot.handleChatProfilePreview(ctx, featureCtx)
	}
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

func (s *sSysBot) handleChatProfilePreview(ctx context.Context, featureCtx *botFeatureContext) error {
	chatID := fmt.Sprintf("%d", featureCtx.Msg.Chat.ID)
	token := strings.TrimPrefix(strings.TrimSpace(featureCtx.Args), "chat_profile_")
	value, err := cache.Instance().Get(ctx, "youban:chat:profile_preview:"+token)
	if err != nil || value.IsNil() {
		return s.sendMessageOnly(ctx, featureCtx.BotId, chatID, "预览链接已失效，请重新打开。")
	}
	var payload struct {
		ProfileId int64 `json:"profileId"`
	}
	if err = json.Unmarshal([]byte(value.String()), &payload); err != nil || payload.ProfileId <= 0 {
		return s.sendMessageOnly(ctx, featureCtx.BotId, chatID, "预览链接无效。")
	}
	if err = s.sendMessageOnly(ctx, featureCtx.BotId, chatID, "正在打开资料预览，请稍候…"); err != nil {
		return err
	}
	note, err := publishService.SysPublish().BotProfileView(ctx, &publishsysin.BotProfileViewInp{ProfileId: payload.ProfileId, PublicOnly: true})
	if err != nil || note == nil {
		return s.sendMessageOnly(ctx, featureCtx.BotId, chatID, "资料不存在或已下架。")
	}
	if err = s.sendScanProfileContent(ctx, featureCtx.BotId, chatID, note); err != nil {
		return err
	}
	return nil
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
