package sys

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_bot/model/input/sysin"
)

const (
	instantRegisterSource       = "self_register"
	instantRegisterCallbackData = "yb_register|create"
	instantRegisterDefaultTTL   = 15 * time.Minute
)

type instantRegisterFeature struct{}

func (instantRegisterFeature) Key() string         { return "instant_register" }
func (instantRegisterFeature) Command() string     { return "register" }
func (instantRegisterFeature) Description() string { return "立即注册" }
func (instantRegisterFeature) ConfigSchema() []*sysin.FeatureConfigSchema {
	return []*sysin.FeatureConfigSchema{
		{Field: "buttonLabel", Label: "注册按钮文案", Component: "input", Default: "🚀 立即注册", Placeholder: "显示在机器人欢迎消息底部"},
		{Field: "replyText", Label: "注册链接文案", Component: "telegram_rich_text", Default: "<b>注册链接已生成</b>\n\n请在有效期内完成注册。注册成功后，当前 Telegram 将自动绑定到新账号。", Placeholder: "支持 Telegram HTML"},
		{Field: "openButtonLabel", Label: "打开注册页按钮", Component: "input", Default: "打开注册页面", Placeholder: "注册链接按钮文案"},
		{Field: "alreadyBoundText", Label: "已绑定提示", Component: "textarea", Default: "当前 Telegram 已绑定系统账号，无需重复注册。"},
		{Field: "publishDomain", Label: "上架端域名", Component: "input", Default: "", Placeholder: "例如：https://publish.example.com"},
		{Field: "expireMinutes", Label: "链接有效分钟", Component: "input", Default: 15, Placeholder: "默认 15 分钟，最多 120 分钟"},
		{Field: "codeLength", Label: "邀请码长度", Component: "input", Default: 12, Placeholder: "默认 12 位字母+数字"},
	}
}

func (instantRegisterFeature) Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (bool, error) {
	if featureCtx == nil || featureCtx.Msg == nil || featureCtx.Msg.From == nil || featureCtx.Msg.Chat.Type != models.ChatTypePrivate {
		return true, nil
	}
	chatId := fmt.Sprintf("%d", featureCtx.Msg.Chat.ID)
	if err := bot.sendInstantRegisterLink(ctx, featureCtx.BotId, chatId, featureCtx.Msg.From); err != nil {
		g.Log().Warningf(ctx, "发送立即注册链接失败 botId:%d chatId:%s telegramUserId:%d err:%+v", featureCtx.BotId, chatId, featureCtx.Msg.From.ID, err)
		return true, bot.reply(ctx, featureCtx.BotId, chatId, instantRegisterErrorText(err))
	}
	return true, nil
}

func (s *sSysBot) handleInstantRegisterCallback(ctx context.Context, botId int64, query *models.CallbackQuery) (bool, error) {
	if query == nil || strings.TrimSpace(query.Data) != instantRegisterCallbackData {
		return false, nil
	}
	row, err := s.botById(ctx, botId)
	if err != nil {
		return true, err
	}
	tgBot, err := s.telegramBot(ctx, row.BotToken)
	if err != nil {
		return true, err
	}
	callCtx, cancel := telegramAPICtx()
	defer cancel()
	_, _ = tgBot.AnswerCallbackQuery(callCtx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: query.ID})
	if query.Message.Message == nil || query.Message.Message.Chat.Type != models.ChatTypePrivate {
		return true, nil
	}
	chatId := fmt.Sprintf("%d", query.Message.Message.Chat.ID)
	if err = s.sendInstantRegisterLink(ctx, botId, chatId, &query.From); err != nil {
		return true, s.reply(ctx, botId, chatId, instantRegisterErrorText(err))
	}
	return true, nil
}

func (s *sSysBot) sendInstantRegisterLink(ctx context.Context, botId int64, chatId string, user *models.User) error {
	if user == nil || user.ID <= 0 {
		return gerror.New("Telegram用户信息不存在")
	}
	telegramUserId := fmt.Sprintf("%d", user.ID)
	bind, err := s.bindingByTelegram(ctx, sysin.BotAppApi, telegramUserId)
	if err != nil {
		return err
	}
	if bind != nil && bind.AccountId > 0 {
		text := strings.TrimSpace(s.featureConfigValue(ctx, instantRegisterFeature{}.Key(), "alreadyBoundText"))
		if text == "" {
			text = "当前 Telegram 已绑定系统账号，无需重复注册。"
		}
		return s.reply(ctx, botId, chatId, text)
	}
	item, err := s.ensureInstantRegisterInvite(ctx, botId, user)
	if err != nil {
		return err
	}
	text := strings.TrimSpace(s.featureConfigValue(ctx, instantRegisterFeature{}.Key(), "replyText"))
	if text == "" {
		text = "<b>注册链接已生成</b>\n\n请在有效期内完成注册。注册成功后，当前 Telegram 将自动绑定到新账号。"
	}
	buttonLabel := strings.TrimSpace(s.featureConfigValue(ctx, instantRegisterFeature{}.Key(), "openButtonLabel"))
	if buttonLabel == "" {
		buttonLabel = "打开注册页面"
	}
	baseText := text
	buttonURL := telegramInlineButtonURL(item.InviteUrl)
	inlineButton := buttonURL != ""
	messageURL := item.InviteUrl
	if inlineButton {
		messageURL = buttonURL
	}
	text = instantRegisterMessage(baseText, item.Code, item.ExpiresAt.String(), messageURL, buttonLabel, inlineButton)
	var markup models.ReplyMarkup
	if inlineButton {
		markup = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: buttonLabel, URL: buttonURL}}}}
	}
	if err = s.replyWithMarkup(ctx, botId, chatId, text, markup); err != nil && inlineButton && telegramInlineButtonURLInvalid(err) {
		g.Log().Warningf(ctx, "Telegram拒绝注册链接按钮，降级为正文链接 botId:%d chatId:%s url:%s err:%+v", botId, chatId, item.InviteUrl, err)
		text = instantRegisterMessage(baseText, item.Code, item.ExpiresAt.String(), item.InviteUrl, buttonLabel, false)
		return s.replyWithMarkup(ctx, botId, chatId, text, nil)
	}
	return err
}

func instantRegisterMessage(text string, code string, expiresAt string, inviteURL string, linkLabel string, clickableLabel bool) string {
	link := html.EscapeString(strings.TrimSpace(inviteURL))
	if clickableLabel {
		link = fmt.Sprintf(`<a href="%s">%s</a>`, link, html.EscapeString(strings.TrimSpace(linkLabel)))
	}
	return strings.TrimSpace(text) + fmt.Sprintf(
		"\n\n注册链接：%s\n邀请码：<code>%s</code>\n有效期至：%s",
		link,
		html.EscapeString(strings.TrimSpace(code)),
		html.EscapeString(strings.TrimSpace(expiresAt)),
	)
}

func telegramInlineButtonURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return ""
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		parsed.Host = telegramLoopbackHost(parsed.Port())
		return parsed.String()
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			parsed.Host = telegramLoopbackHost(parsed.Port())
			return parsed.String()
		}
		if ip.IsPrivate() || ip.IsUnspecified() {
			return ""
		}
		return parsed.String()
	}
	if !strings.Contains(host, ".") {
		return ""
	}
	return parsed.String()
}

func telegramLoopbackHost(port string) string {
	if strings.TrimSpace(port) == "" {
		return "127.0.0.1.nip.io"
	}
	return net.JoinHostPort("127.0.0.1.nip.io", port)
}

func telegramInlineButtonURLInvalid(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "button url") || strings.Contains(message, "wrong http url")
}

func (s *sSysBot) ensureInstantRegisterInvite(ctx context.Context, botId int64, user *models.User) (*sysin.InviteCreateModel, error) {
	telegramUserId := fmt.Sprintf("%d", user.ID)
	now := gtime.Now()
	var active *inviteRow
	if err := g.DB().Model(inviteTable).Safe().Ctx(ctx).
		Where("source", instantRegisterSource).
		Where("registration_telegram_user_id", telegramUserId).
		Where("status", inviteStatusActive).
		WhereGT("expires_at", now).
		WhereNull("deleted_at").OrderDesc("id").Scan(&active); err != nil {
		return nil, gerror.Wrap(err, "读取立即注册链接失败")
	}
	if active != nil && active.Id > 0 {
		inviteURL, urlErr := s.instantRegisterURL(ctx, active.Code)
		if urlErr != nil {
			return nil, urlErr
		}
		return &sysin.InviteCreateModel{Code: active.Code, Source: active.Source, ExpiresAt: active.ExpiresAt, InviteUrl: inviteURL}, nil
	}
	_, _ = g.DB().Model(inviteTable).Safe().Ctx(ctx).
		Where("source", instantRegisterSource).
		Where("registration_telegram_user_id", telegramUserId).
		Where("status", inviteStatusActive).
		WhereNull("deleted_at").Data(g.Map{"status": inviteStatusExpired, "updated_at": now}).Update()
	code, err := s.uniqueInviteCode(ctx, s.instantRegisterCodeLength(ctx))
	if err != nil {
		return nil, err
	}
	inviteURL, err := s.instantRegisterURL(ctx, code)
	if err != nil {
		return nil, err
	}
	expiresAt := now.Add(s.instantRegisterTTL(ctx))
	_, err = g.DB().Model(inviteTable).Safe().Ctx(ctx).Data(g.Map{
		"code": code, "source": instantRegisterSource, "inviter_app": sysin.BotAppApi,
		"registration_telegram_user_id":    telegramUserId,
		"registration_telegram_username":   strings.TrimPrefix(strings.TrimSpace(user.Username), "@"),
		"registration_telegram_first_name": user.FirstName,
		"registration_telegram_last_name":  user.LastName,
		"registration_bot_id":              botId,
		"status":                           inviteStatusActive, "expires_at": expiresAt, "created_at": now, "updated_at": now,
	}).Insert()
	if err != nil {
		return nil, gerror.Wrap(err, "创建立即注册链接失败")
	}
	return &sysin.InviteCreateModel{Code: code, Source: instantRegisterSource, ExpiresAt: expiresAt, InviteUrl: inviteURL}, nil
}

func (s *sSysBot) instantRegisterTTL(ctx context.Context) time.Duration {
	minutes, err := strconv.Atoi(strings.TrimSpace(s.featureConfigValue(ctx, instantRegisterFeature{}.Key(), "expireMinutes")))
	if err != nil || minutes <= 0 {
		return instantRegisterDefaultTTL
	}
	if minutes > 120 {
		minutes = 120
	}
	return time.Duration(minutes) * time.Minute
}

func (s *sSysBot) instantRegisterCodeLength(ctx context.Context) int {
	length, err := strconv.Atoi(strings.TrimSpace(s.featureConfigValue(ctx, instantRegisterFeature{}.Key(), "codeLength")))
	if err != nil || length < 8 {
		return 12
	}
	if length > 16 {
		return 16
	}
	return length
}

func (s *sSysBot) instantRegisterURL(ctx context.Context, code string) (string, error) {
	path := "/auth/register?inviteCode=" + url.QueryEscape(strings.TrimSpace(code))
	base := strings.TrimRight(strings.TrimSpace(s.featureConfigValue(ctx, instantRegisterFeature{}.Key(), "publishDomain")), "/")
	if base == "" {
		base = strings.TrimRight(strings.TrimSpace(s.featureConfigValue(ctx, inviteFeature{}.Key(), "publishDomain")), "/")
	}
	if base == "" {
		return "", gerror.New("请先在机器人插件配置中填写上架端域名")
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "https://" + base
	}
	return base + path, nil
}

func instantRegisterErrorText(err error) string {
	if err != nil && strings.Contains(err.Error(), "上架端域名") {
		return "注册链接暂未配置，请联系管理员。"
	}
	return "生成注册链接失败，请稍后重试。"
}
