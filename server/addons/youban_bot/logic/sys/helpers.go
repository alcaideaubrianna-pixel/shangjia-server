package sys

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/crypto/gmd5"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"golang.org/x/net/proxy"

	"hotgo/addons/youban_bot/model/input/sysin"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/token"
	"hotgo/internal/model"
	"hotgo/internal/model/entity"
	isc "hotgo/internal/service"
)

type telegramUserIdCtxKey struct{}

type botBindRow struct {
	Id               int64  `json:"id"`
	App              string `json:"app"`
	AccountId        int64  `json:"account_id"`
	TelegramUserId   string `json:"telegram_user_id"`
	TelegramUsername string `json:"telegram_username"`
}

type botLoginResult struct {
	Token       string
	Expires     int64
	TenantId    int64
	AccountType string
	Username    string
	Nickname    string
}

func (s *sSysBot) bindingByTelegram(ctx context.Context, app, telegramUserId string) (*botBindRow, error) {
	var row *botBindRow
	err := g.DB().Model(accountBindTbl).Safe().Ctx(ctx).Where("app", app).Where("telegram_user_id", telegramUserId).Where("status", 1).WhereNull("deleted_at").Scan(&row)
	if err != nil {
		return nil, gerror.Wrap(err, "读取Telegram绑定失败")
	}
	return row, nil
}

func (s *sSysBot) loginBoundAccount(ctx context.Context, app string, accountId int64) (*botLoginResult, error) {
	if app == sysin.BotAppAdmin {
		return s.loginAdminAccount(ctx, accountId)
	}
	return s.loginPublishAccount(ctx, accountId)
}

func (s *sSysBot) loginAdminAccount(ctx context.Context, accountId int64) (*botLoginResult, error) {
	var mb *entity.AdminMember
	if err := dao.AdminMember.Ctx(ctx).WherePri(accountId).Scan(&mb); err != nil {
		return nil, gerror.Wrap(err, consts.ErrorORM)
	}
	if mb == nil || mb.Id <= 0 {
		return nil, gerror.New("后台账号不存在")
	}
	if mb.Status != consts.StatusEnabled {
		return nil, gerror.New("后台账号已被禁用")
	}
	role, err := s.adminRole(ctx, mb.RoleId)
	if err != nil {
		return nil, err
	}
	dept, err := s.adminDept(ctx, mb.DeptId)
	if err != nil {
		return nil, err
	}
	user := &model.Identity{Id: mb.Id, Pid: mb.Pid, DeptId: dept.Id, DeptType: dept.Type, RoleId: role.Id, RoleKey: role.Key, Username: mb.Username, RealName: mb.RealName, Avatar: mb.Avatar, Email: mb.Email, Mobile: mb.Mobile, App: consts.AppAdmin, LoginAt: gtime.Now()}
	lt, expires, err := token.Login(ctx, user)
	if err != nil {
		return nil, err
	}
	return &botLoginResult{Token: lt, Expires: expires, AccountType: "admin", Username: mb.Username, Nickname: firstNonEmpty(mb.RealName, mb.Username)}, nil
}

func (s *sSysBot) adminRole(ctx context.Context, roleId int64) (*entity.AdminRole, error) {
	var role *entity.AdminRole
	cols := dao.AdminRole.Columns()
	if err := dao.AdminRole.Ctx(ctx).Fields(cols.Id, cols.Key, cols.Status).WherePri(roleId).Scan(&role); err != nil {
		return nil, gerror.Wrap(err, consts.ErrorORM)
	}
	if role == nil {
		return nil, gerror.New("角色不存在或已被删除")
	}
	if role.Status != consts.StatusEnabled {
		return nil, gerror.New("角色已被禁用")
	}
	return role, nil
}

func (s *sSysBot) adminDept(ctx context.Context, deptId int64) (*entity.AdminDept, error) {
	var dept *entity.AdminDept
	cols := dao.AdminDept.Columns()
	if err := dao.AdminDept.Ctx(ctx).Fields(cols.Id, cols.Type, cols.Status).WherePri(deptId).Scan(&dept); err != nil {
		return nil, gerror.Wrap(err, "获取部门信息失败，请稍后重试！")
	}
	if dept == nil {
		return nil, gerror.New("部门不存在或已被删除")
	}
	if dept.Status != consts.StatusEnabled {
		return nil, gerror.New("部门已被禁用")
	}
	return dept, nil
}

func (s *sSysBot) loginPublishAccount(ctx context.Context, accountId int64) (*botLoginResult, error) {
	var account *publishsysin.AccountModel
	err := g.DB().Model(publishAccountTable).Safe().Ctx(ctx).Where("id", accountId).WhereNull("deleted_at").Scan(&account)
	if err != nil {
		return nil, gerror.Wrap(err, "读取上架账号失败")
	}
	if account == nil || account.Id <= 0 {
		return nil, gerror.New("上架账号不存在")
	}
	if account.Status != consts.StatusEnabled {
		return nil, gerror.New("上架账号已被停用")
	}
	user := &model.Identity{Id: account.Id, Pid: account.ParentId, DeptType: account.AccountType, Username: account.Username, RealName: account.Nickname, App: consts.AppApi, LoginAt: gtime.Now()}
	lt, expires, err := token.Login(ctx, user)
	if err != nil {
		return nil, err
	}
	return &botLoginResult{Token: lt, Expires: expires, TenantId: account.TenantId, AccountType: account.AccountType, Username: account.Username, Nickname: account.Nickname}, nil
}

func (s *sSysBot) botById(ctx context.Context, id int64) (*sysin.BotModel, error) {
	var row *sysin.BotModel
	if err := g.DB().Model(botTable).Safe().Ctx(ctx).Where("id", id).WhereNull("deleted_at").Scan(&row); err != nil {
		return nil, gerror.Wrap(err, "读取Bot配置失败")
	}
	if row == nil || row.Id <= 0 {
		return nil, gerror.New("Bot配置不存在")
	}
	return row, nil
}

func (s *sSysBot) officialBot(ctx context.Context) (*sysin.BotModel, error) {
	var row *sysin.BotModel
	if err := g.DB().Model(botTable).Safe().Ctx(ctx).Where("is_official", 1).Where("status", 1).WhereNull("deleted_at").OrderDesc("is_default").OrderAsc("id").Scan(&row); err != nil {
		return nil, gerror.Wrap(err, "读取官方Bot失败")
	}
	if row == nil || row.Id <= 0 {
		return nil, gerror.New("请先在后台配置并启用官方Bot")
	}
	return row, nil
}

func (s *sSysBot) enabledBots(ctx context.Context) ([]*sysin.BotModel, error) {
	var rows []*sysin.BotModel
	if err := g.DB().Model(botTable).Safe().Ctx(ctx).Where("status", 1).WhereNull("deleted_at").OrderAsc("id").Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取Bot配置失败")
	}
	return rows, nil
}

func (s *sSysBot) telegramBotProfile(ctx context.Context, botToken string) (*models.User, error) {
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return nil, err
	}
	user, err := bot.GetMe(ctx)
	if err != nil {
		return nil, err
	}
	if user == nil || !user.IsBot {
		return nil, gerror.New("Token不是有效的Telegram Bot")
	}
	return user, nil
}

func (s *sSysBot) telegramBot(ctx context.Context, botToken string) (*tgbot.Bot, error) {
	botToken = strings.TrimSpace(botToken)
	if botToken == "" {
		return nil, gerror.New("Telegram Bot Token未配置")
	}
	cacheKey := botToken
	s.telegramBotMu.Lock()
	if s.telegramBots != nil {
		if bot := s.telegramBots[cacheKey]; bot != nil {
			s.telegramBotMu.Unlock()
			return bot, nil
		}
	}
	s.telegramBotMu.Unlock()
	client := &http.Client{Timeout: 35 * time.Second}
	if proxyUrl := g.Cfg().MustGet(ctx, "youbanBot.telegram.proxyUrl").String(); strings.TrimSpace(proxyUrl) != "" {
		proxyClient, err := telegramHTTPClient(proxyUrl)
		if err != nil {
			return nil, err
		}
		client = proxyClient
	}
	bot, err := tgbot.New(botToken, tgbot.WithHTTPClient(21*time.Second, client), tgbot.WithSkipGetMe(), tgbot.WithAllowedUpdates([]string{"message", "edited_message"}), tgbot.WithErrorsHandler(func(err error) { g.Log().Warningf(ctx, "Telegram SDK错误：%+v", err) }))
	if err != nil {
		return nil, err
	}
	s.telegramBotMu.Lock()
	if s.telegramBots == nil {
		s.telegramBots = map[string]*tgbot.Bot{}
	}
	s.telegramBots[cacheKey] = bot
	s.telegramBotMu.Unlock()
	return bot, nil
}

func (s *sSysBot) clearTelegramBotCache() {
	s.telegramBotMu.Lock()
	defer s.telegramBotMu.Unlock()
	s.telegramBots = nil
}

func telegramHTTPClient(proxyUrl string) (*http.Client, error) {
	transport := &http.Transport{}
	parsed, err := url.Parse(strings.TrimSpace(proxyUrl))
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(parsed.Scheme) {
	case "socks5", "socks5h":
		dialer, err := proxy.FromURL(parsed, proxy.Direct)
		if err != nil {
			return nil, err
		}
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialer.Dial(network, address)
		}
	case "http", "https":
		transport.Proxy = http.ProxyURL(parsed)
	default:
		return nil, gerror.New("仅支持 http/https/socks5 代理")
	}
	return &http.Client{Timeout: 35 * time.Second, Transport: transport}, nil
}

func (s *sSysBot) reply(ctx context.Context, botId int64, chatId string, text string) error {
	tokenText := ""
	if botId > 0 {
		if row, err := s.botById(ctx, botId); err == nil && row != nil {
			tokenText = row.BotToken
		}
	}
	if tokenText == "" {
		if row, err := s.officialBot(ctx); err == nil && row != nil {
			tokenText = row.BotToken
		}
	}
	if tokenText == "" {
		return nil
	}
	_, err := s.sendMessageWithMarkup(ctx, tokenText, chatId, text, "HTML", false, s.replyKeyboard(ctx))
	if err != nil && botId > 0 && shouldMarkBotOffline(err) {
		_ = s.markBotOffline(ctx, botId, err)
	}
	return err
}

func (s *sSysBot) Notify(ctx context.Context, in *sysin.NotifyInp) error {
	if in == nil {
		return gerror.New("消息内容不能为空")
	}
	if strings.TrimSpace(in.ChatId) == "" {
		return gerror.New("目标Chat ID不能为空")
	}
	if strings.TrimSpace(in.Text) == "" {
		return gerror.New("消息内容不能为空")
	}
	botToken := ""
	if in.BotId > 0 {
		row, err := s.botById(ctx, in.BotId)
		if err != nil {
			return err
		}
		botToken = row.BotToken
	} else {
		row, err := s.officialBot(ctx)
		if err != nil {
			return err
		}
		botToken = row.BotToken
	}
	_, err := s.sendMessage(ctx, botToken, in.ChatId, in.Text, firstNonEmpty(in.ParseMode, "HTML"), in.DisableNotice)
	if err != nil && in.BotId > 0 && shouldMarkBotOffline(err) {
		_ = s.markBotOffline(ctx, in.BotId, err)
	}
	return err
}

func (s *sSysBot) sendMessage(ctx context.Context, botToken, chatId, text, parseMode string, disableNotice bool) (*models.Message, error) {
	return s.sendMessageWithMarkup(ctx, botToken, chatId, text, parseMode, disableNotice, nil)
}

func (s *sSysBot) sendMessageWithMarkup(ctx context.Context, botToken, chatId, text, parseMode string, disableNotice bool, replyMarkup models.ReplyMarkup) (*models.Message, error) {
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return nil, err
	}
	params := &tgbot.SendMessageParams{ChatID: chatId, Text: text, ParseMode: models.ParseMode(parseMode), DisableNotification: disableNotice, ReplyMarkup: replyMarkup}
	callCtx, cancel := telegramAPICtx()
	defer cancel()
	return bot.SendMessage(callCtx, params)
}

func (s *sSysBot) replyKeyboard(ctx context.Context) *models.ReplyKeyboardMarkup {
	buttons := make([]models.KeyboardButton, 0, len(botFeatures))
	for _, feature := range botFeatures {
		if feature == nil {
			continue
		}
		if !s.featureMenuVisible(ctx, feature.Key()) {
			continue
		}
		if !s.featureVisibleForTelegramUser(ctx, feature, telegramUserIdFromCtx(ctx)) {
			continue
		}
		label := s.featureDescription(ctx, feature)
		if strings.TrimSpace(label) == "" {
			label = "/" + strings.TrimPrefix(s.featureCommand(ctx, feature), "/")
		}
		buttons = append(buttons, models.KeyboardButton{Text: label})
	}
	if len(buttons) == 0 {
		return nil
	}
	rows := make([][]models.KeyboardButton, 0, (len(buttons)+1)/2)
	for i := 0; i < len(buttons); i += 2 {
		end := i + 2
		if end > len(buttons) {
			end = len(buttons)
		}
		rows = append(rows, buttons[i:end])
	}
	return &models.ReplyKeyboardMarkup{Keyboard: rows, IsPersistent: true, ResizeKeyboard: true, InputFieldPlaceholder: "输入验证码或选择菜单"}
}

func telegramBotDisplayName(user *models.User) string {
	if user == nil {
		return ""
	}
	name := strings.TrimSpace(strings.TrimSpace(user.FirstName + " " + user.LastName))
	if name == "" {
		name = strings.TrimPrefix(user.Username, "@")
	}
	return name
}

func uniqueIds(ids []int64) []int64 {
	seen := map[int64]struct{}{}
	list := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		list = append(list, id)
	}
	return list
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func checkPassword(password, salt, hash string) bool {
	return gmd5.MustEncryptString(password+salt) == hash
}

func currentApp(ctx context.Context) string {
	if user := contexts.GetUser(ctx); user != nil {
		return user.App
	}
	return ""
}

func accountLabel(app string) string {
	if app == sysin.BotAppAdmin {
		return "管理后台"
	}
	return "上架端"
}

func _unusedFormat() { _ = fmt.Sprintf }

func gCfgString(ctx context.Context, key string) string { return g.Cfg().MustGet(ctx, key).String() }

func normalizeRunMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "webhook", "polling", "auto":
		return strings.ToLower(strings.TrimSpace(mode))
	default:
		return "auto"
	}
}

func normalizeSwitch(v int) int {
	if v == 1 {
		return 1
	}
	return 0
}

func (s *sSysBot) botRuntimeMode(ctx context.Context, row *sysin.BotModel) string {
	if raw := strings.TrimSpace(g.Cfg().MustGet(ctx, "youbanBot.telegram.runtimeMode").String()); raw != "" {
		if mode := normalizeRunMode(raw); mode != "auto" {
			return mode
		}
	}
	if row != nil {
		return normalizeRunMode(row.RunMode)
	}
	return "auto"
}

func (s *sSysBot) shouldUseWebhookInAuto(ctx context.Context) bool {
	if g.Cfg().MustGet(ctx, "youbanBot.telegram.autoWebhook", false).Bool() {
		return true
	}
	mode := strings.ToLower(strings.TrimSpace(g.Cfg().MustGet(ctx, "system.mode").String()))
	return mode == "product" || mode == "staging"
}

func (s *sSysBot) botWebhookURL(ctx context.Context, row *sysin.BotModel) string {
	if row == nil {
		return ""
	}
	if url := strings.TrimSpace(row.WebhookUrl); url != "" {
		return url
	}
	base := strings.TrimRight(strings.TrimSpace(g.Cfg().MustGet(ctx, "youbanBot.telegram.webhookDomain").String()), "/")
	if base == "" {
		if basic, err := isc.SysConfig().GetBasic(ctx); err == nil && basic != nil {
			base = strings.TrimRight(strings.TrimSpace(basic.Domain), "/")
		}
	}
	if base == "" {
		return ""
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "https://" + base
	}
	return fmt.Sprintf("%s/api/youban_bot/telegram/webhook?botId=%d", base, row.Id)
}

func (s *sSysBot) markBotOffline(ctx context.Context, botId int64, err error) error {
	if botId <= 0 {
		return nil
	}
	if err != nil {
		g.Log().Warningf(ctx, "Bot调用失败，已标记离线 botId:%d err:%+v", botId, err)
	}
	_, updateErr := g.DB().Model(botTable).Safe().Ctx(ctx).Where("id", botId).Data(g.Map{"status": 2, "updated_at": gtime.Now()}).Update()
	return updateErr
}

func (s *sSysBot) fallbackWebhookBotId(ctx context.Context) int64 {
	if row, err := s.officialBot(ctx); err == nil && row != nil && row.Id > 0 {
		return row.Id
	}
	var row *sysin.BotModel
	if err := g.DB().Model(botTable).Safe().Ctx(ctx).Fields("id").Where("status", 1).WhereNull("deleted_at").OrderAsc("id").Scan(&row); err != nil {
		g.Log().Warningf(ctx, "Webhook fallback读取Bot失败 err:%+v", err)
		return 0
	}
	if row != nil {
		return row.Id
	}
	return 0
}

func telegramAPICtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 35*time.Second)
}

func isIgnorableTelegramError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "unexpected end of json input") || strings.Contains(text, "context canceled")
}

func shouldMarkBotOffline(err error) bool {
	if err == nil || isIgnorableTelegramError(err) {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "unauthorized") || strings.Contains(text, "token") || strings.Contains(text, "not found") || strings.Contains(text, "forbidden")
}

func telegramUserIdFromCtx(ctx context.Context) string {
	if value := ctx.Value(telegramUserIdCtxKey{}); value != nil {
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
	return ""
}

func (s *sSysBot) featureVisibleForTelegramUser(ctx context.Context, feature botFeature, telegramUserId string) bool {
	if feature == nil {
		return false
	}
	if feature.Key() != (adminFeature{}).Key() {
		return true
	}
	if telegramUserId == "" {
		return false
	}
	bind, err := s.bindingByTelegram(ctx, sysin.BotAppAdmin, telegramUserId)
	if err != nil {
		g.Log().Warningf(ctx, "判断管理后台菜单可见失败 telegramUserId:%s err:%+v", telegramUserId, err)
		return false
	}
	return bind != nil && bind.AccountId > 0
}

func isSystemErrorRemark(remark string) bool {
	text := strings.ToLower(strings.TrimSpace(remark))
	if text == "" {
		return false
	}
	return strings.Contains(text, "deletewebhook") || strings.Contains(text, "getme") || strings.Contains(text, "sendmessage") || strings.Contains(text, "unexpected end of json input") || strings.Contains(text, "context deadline exceeded") || strings.Contains(text, "context canceled")
}
