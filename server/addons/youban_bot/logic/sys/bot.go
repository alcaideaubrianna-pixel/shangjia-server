package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/grand"

	"hotgo/addons/youban_bot/model/input/sysin"
	"hotgo/addons/youban_bot/service"
	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
)

const (
	botTable          = "hg_youban_bot_bot"
	featureTable      = "hg_youban_bot_feature"
	authCodeTable     = "hg_youban_bot_auth_code"
	accountBindTbl    = "hg_youban_bot_account_bind"
	userTable         = "hg_youban_bot_user"
	messageTable      = "hg_youban_bot_message"
	channelCacheTable = "hg_youban_bot_channel_cache"
	customEmojiTable  = "hg_youban_bot_custom_emoji"

	publishAccountTable = "hg_youban_publish_account"
)

var sixDigitRegexp = regexp.MustCompile(`\b\d{6}\b`)

type sSysBot struct {
	telegramBotMu     sync.Mutex
	telegramBots      map[string]*tgbot.Bot
	runtimeMu         sync.Mutex
	runtimeCancel     context.CancelFunc
	cleanupMu         sync.Mutex
	cleanupCancel     context.CancelFunc
	featureMu         sync.RWMutex
	features          map[string]*botFeatureRow
	featureAt         time.Time
	featureDefaultsAt time.Time
}

type authCodeRow struct {
	Id               int64       `json:"id"`
	Code             string      `json:"code"`
	Scene            string      `json:"scene"`
	App              string      `json:"app"`
	AccountId        int64       `json:"account_id"`
	TelegramUserId   string      `json:"telegram_user_id"`
	TelegramUsername string      `json:"telegram_username"`
	LoginToken       string      `json:"login_token"`
	Status           string      `json:"status"`
	ErrorMessage     string      `json:"error_message"`
	ExpiresAt        *gtime.Time `json:"expires_at"`
}

func (row *authCodeRow) statusModel() *sysin.CodeStatusModel {
	if row == nil {
		return nil
	}
	return &sysin.CodeStatusModel{
		AccountId:        row.AccountId,
		App:              row.App,
		Code:             row.Code,
		ErrorMessage:     row.ErrorMessage,
		ExpiresAt:        row.ExpiresAt,
		Scene:            row.Scene,
		Status:           row.Status,
		TelegramUserId:   row.TelegramUserId,
		TelegramUsername: row.TelegramUsername,
		Token:            row.LoginToken,
	}
}

func init() {
	service.RegisterSysBot(NewSysBot())
}

func NewSysBot() *sSysBot { return &sSysBot{} }

func (s *sSysBot) StartRuntime(ctx context.Context) {
	_ = s.syncAllTelegramBotMenus(ctx)
	s.startPolling(ctx)
	s.startTelegramMessageCleanup(ctx)
}
func (s *sSysBot) StopRuntime() {
	s.stopPolling()
	s.stopTelegramMessageCleanup()
	s.clearTelegramBotCache()
}

type botFeature interface {
	Key() string
	Command() string
	Description() string
	Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (handled bool, err error)
}

type botFeatureConfigProvider interface {
	ConfigSchema() []*sysin.FeatureConfigSchema
}

type botFeatureTextMatcher interface {
	Match(ctx context.Context, bot *sSysBot, row *botFeatureRow, text string) bool
}

type botFeatureContext struct {
	BotId int64
	Msg   *models.Message
	Text  string
	Args  string
}

type botFeatureRow struct {
	Id          int64       `json:"id"`
	FeatureKey  string      `json:"feature_key"`
	Name        string      `json:"name"`
	Command     string      `json:"command"`
	Description string      `json:"description"`
	ConfigJson  string      `json:"config_json"`
	Sort        int         `json:"sort"`
	Status      int         `json:"status"`
	CreatedAt   *gtime.Time `json:"created_at"`
	UpdatedAt   *gtime.Time `json:"updated_at"`
}

var botFeatures []botFeature

func registerBotFeature(feature botFeature) {
	if feature == nil || strings.TrimSpace(feature.Key()) == "" {
		return
	}
	botFeatures = append(botFeatures, feature)
}

func init() {
	registerBotFeature(startFeature{})
	registerBotFeature(inlinePromotionFeature{})
	registerBotFeature(loginFeature{})
	registerBotFeature(bindFeature{})
	registerBotFeature(infoFeature{})
	registerBotFeature(notifyFeature{})
	registerBotFeature(superNotifyFeature{})
	registerBotFeature(contactFeature{})
	registerBotFeature(adminFeature{})
	registerBotFeature(inviteFeature{})
	registerBotFeature(instantRegisterFeature{})
	registerBotFeature(profileFeature{})
	registerBotFeature(scanFeature{})
	registerBotFeature(quickPushFeature{})
	registerBotFeature(exchangeRateFeature{})
}

func (s *sSysBot) AdminBotList(ctx context.Context, in *sysin.BotListInp) (list []*sysin.BotModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.BotListInp{}
	}
	mod := g.DB().Model(botTable).Safe().Ctx(ctx).WhereNull("deleted_at")
	if in.Status > 0 {
		mod = mod.Where("status", in.Status)
	}
	if in.IsOfficial == 1 {
		mod = mod.Where("is_official", 1)
	}
	if in.IsOfficial == 2 {
		mod = mod.Where("is_official", 0)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(bot_name LIKE ? OR bot_username LIKE ? OR remark LIKE ?)", like, like, like)
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("id").ScanAndCount(&list, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "获取Bot列表失败")
	}
	if list == nil {
		list = []*sysin.BotModel{}
	}
	for _, item := range list {
		if item != nil && isSystemErrorRemark(item.Remark) {
			item.Remark = ""
		}
	}
	return
}

func (s *sSysBot) AdminBotSave(ctx context.Context, in *sysin.BotSaveInp) (err error) {
	if in == nil {
		return gerror.New("Bot配置不能为空")
	}
	var old *sysin.BotModel
	if in.Id > 0 {
		old, err = s.botById(ctx, in.Id)
		if err != nil {
			return err
		}
	}
	botToken := strings.TrimSpace(in.BotToken)
	if botToken == "" && old != nil {
		botToken = old.BotToken
	}
	if botToken == "" {
		return gerror.New("Bot Token不能为空")
	}
	var tgUser *models.User
	needValidateToken := old == nil || strings.TrimSpace(in.BotToken) != "" && strings.TrimSpace(in.BotToken) != strings.TrimSpace(old.BotToken)
	if needValidateToken {
		tgUser, err = s.telegramBotProfile(ctx, botToken)
		if err != nil {
			return gerror.Wrap(err, "校验Bot Token失败")
		}
	}
	status := in.Status
	if status == 0 {
		status = 1
	}
	if status != 1 && status != 2 {
		return gerror.New("Bot状态不合法")
	}
	botName := strings.TrimSpace(in.BotName)
	botUsername := ""
	if tgUser != nil {
		botUsername = strings.TrimPrefix(strings.TrimSpace(tgUser.Username), "@")
		if botName == "" {
			botName = telegramBotDisplayName(tgUser)
		}
	} else if old != nil {
		botUsername = old.BotUsername
		if botName == "" {
			botName = old.BotName
		}
	}
	isOfficial := 0
	if in.IsOfficial == 1 || in.IsDefault == 1 {
		isOfficial = 1
	}
	isDefault := 0
	if in.IsDefault == 1 {
		isDefault = 1
	}
	data := g.Map{
		"bot_name": botName, "bot_username": botUsername, "bot_token": botToken,
		"is_official": isOfficial, "is_default": isDefault, "remark": strings.TrimSpace(in.Remark),
		"run_mode": normalizeRunMode(in.RunMode), "webhook_url": strings.TrimSpace(in.WebhookUrl),
		"status": status, "updated_by": contexts.GetUserId(ctx), "updated_at": gtime.Now(),
	}
	botId := in.Id
	if in.Id > 0 {
		_, err = g.DB().Model(botTable).Safe().Ctx(ctx).Where("id", in.Id).WhereNull("deleted_at").Data(data).Update()
	} else {
		data["created_by"] = contexts.GetUserId(ctx)
		data["created_at"] = gtime.Now()
		result, insertErr := g.DB().Model(botTable).Safe().Ctx(ctx).Data(data).Insert()
		if insertErr == nil && result != nil {
			botId, _ = result.LastInsertId()
		}
		err = insertErr
	}
	if err != nil {
		return gerror.Wrap(err, "保存Bot配置失败")
	}
	if isDefault == 1 && botId > 0 {
		if _, err = g.DB().Model(botTable).Safe().Ctx(ctx).WhereNot("id", botId).WhereNull("deleted_at").Data(g.Map{"is_default": 0, "updated_at": gtime.Now()}).Update(); err != nil {
			return gerror.Wrap(err, "更新默认Bot失败")
		}
	}
	s.clearTelegramBotCache()
	s.restartRuntime(ctx)
	return nil
}

func (s *sSysBot) AdminBotDelete(ctx context.Context, in *sysin.BotDeleteInp) (err error) {
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的数据")
	}
	_, err = g.DB().Model(botTable).Safe().Ctx(ctx).WhereIn("id", uniqueIds(in.Ids)).Data(g.Map{"deleted_by": contexts.GetUserId(ctx), "deleted_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "删除Bot配置失败")
	}
	s.clearTelegramBotCache()
	s.restartRuntime(ctx)
	return nil
}

func (s *sSysBot) AdminBotRefresh(ctx context.Context, in *sysin.BotRefreshInp) (list []*sysin.BotRefreshModel, err error) {
	if in == nil || len(in.Ids) == 0 {
		return nil, gerror.New("请选择要刷新的Bot")
	}
	var bots []*sysin.BotModel
	if err = g.DB().Model(botTable).Safe().Ctx(ctx).WhereIn("id", uniqueIds(in.Ids)).WhereNull("deleted_at").Scan(&bots); err != nil {
		return nil, gerror.Wrap(err, "读取Bot配置失败")
	}
	list = make([]*sysin.BotRefreshModel, 0, len(bots))
	for _, item := range bots {
		result := &sysin.BotRefreshModel{Id: item.Id, Status: item.Status}
		tgUser, refreshErr := s.telegramBotProfile(ctx, item.BotToken)
		if refreshErr != nil {
			result.Status = 2
			result.ErrorMessage = refreshErr.Error()
			_, _ = g.DB().Model(botTable).Safe().Ctx(ctx).Where("id", item.Id).Data(g.Map{"status": 2, "updated_by": contexts.GetUserId(ctx), "updated_at": gtime.Now()}).Update()
			list = append(list, result)
			continue
		}
		username := strings.TrimPrefix(strings.TrimSpace(tgUser.Username), "@")
		result.Status = 1
		result.BotUsername = username
		data := g.Map{"bot_name": telegramBotDisplayName(tgUser), "bot_username": username, "status": 1, "updated_by": contexts.GetUserId(ctx), "updated_at": gtime.Now()}
		if isSystemErrorRemark(item.Remark) {
			data["remark"] = ""
		}
		_, err = g.DB().Model(botTable).Safe().Ctx(ctx).Where("id", item.Id).Data(data).Update()
		if err != nil {
			return nil, gerror.Wrap(err, "刷新Bot状态失败")
		}
		list = append(list, result)
	}
	s.clearTelegramBotCache()
	s.restartRuntime(ctx)
	return list, nil
}

func (s *sSysBot) AdminBotRestart(ctx context.Context, in *sysin.BotRefreshInp) (list []*sysin.BotRefreshModel, err error) {
	list, err = s.AdminBotRefresh(ctx, in)
	if err != nil {
		return nil, err
	}
	s.restartRuntime(ctx)
	return list, nil
}

func (s *sSysBot) LoginCodeStart(ctx context.Context, in *sysin.CodeStartInp) (res *sysin.CodeStartModel, err error) {
	if in == nil {
		in = &sysin.CodeStartInp{}
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	return s.createCode(ctx, sysin.BotCodeSceneLogin, in.App, 0)
}

func (s *sSysBot) LoginCodeStatus(ctx context.Context, in *sysin.CodeStatusInp) (*sysin.CodeStatusModel, error) {
	return s.codeStatus(ctx, strings.TrimSpace(in.Code), sysin.BotCodeSceneLogin, "", 0)
}

func (s *sSysBot) BindCodeStart(ctx context.Context) (res *sysin.CodeStartModel, err error) {
	user := contexts.GetUser(ctx)
	if user == nil || user.Id <= 0 {
		return nil, gerror.New("请先登录")
	}
	return s.createCode(ctx, sysin.BotCodeSceneBind, user.App, user.Id)
}

func (s *sSysBot) BindCodeStatus(ctx context.Context, in *sysin.CodeStatusInp) (*sysin.CodeStatusModel, error) {
	user := contexts.GetUser(ctx)
	if user == nil || user.Id <= 0 {
		return nil, gerror.New("请先登录")
	}
	return s.codeStatus(ctx, strings.TrimSpace(in.Code), sysin.BotCodeSceneBind, user.App, user.Id)
}

func (s *sSysBot) BindInfo(ctx context.Context) (res *sysin.BindInfoModel, err error) {
	user := contexts.GetUser(ctx)
	if user == nil || user.Id <= 0 {
		return nil, gerror.New("请先登录")
	}
	botUsername := ""
	if bot, botErr := s.officialBot(ctx); botErr == nil && bot != nil {
		botUsername = bot.BotUsername
	} else if botErr != nil {
		g.Log().Warning(ctx, "读取官方Bot失败，绑定信息暂不返回快捷入口", g.Map{"err": botErr.Error()})
	}
	var row *sysin.BindInfoModel
	err = g.DB().Model(accountBindTbl).Safe().Ctx(ctx).Fields("telegram_user_id,telegram_username").Where("app", user.App).Where("account_id", user.Id).Where("status", 1).WhereNull("deleted_at").Scan(&row)
	if err != nil {
		return nil, gerror.Wrap(err, "读取Telegram绑定信息失败")
	}
	if row == nil {
		return &sysin.BindInfoModel{Bound: false, BotUsername: botUsername}, nil
	}
	row.Bound = strings.TrimSpace(row.TelegramUserId) != ""
	row.BotUsername = botUsername
	return row, nil
}

func (s *sSysBot) createCode(ctx context.Context, scene, app string, accountId int64) (*sysin.CodeStartModel, error) {
	app = sysin.NormalizeApp(app)
	if scene == sysin.BotCodeSceneBind && accountId <= 0 {
		return nil, gerror.New("账号ID不能为空")
	}
	bot, err := s.officialBot(ctx)
	if err != nil {
		return nil, err
	}
	now := gtime.Now()
	expiresAt := now.Add(5 * time.Minute)
	code, err := s.uniqueCode(ctx)
	if err != nil {
		return nil, err
	}
	_, _ = g.DB().Model(authCodeTable).Safe().Ctx(ctx).Where("scene", scene).Where("app", app).Where("account_id", accountId).Where("status", sysin.BotCodeStatusPending).Data(g.Map{"status": sysin.BotCodeStatusExpired, "error_message": "已生成新的验证码", "updated_at": now}).Update()
	_, err = g.DB().Model(authCodeTable).Safe().Ctx(ctx).Data(g.Map{"code": code, "scene": scene, "app": app, "account_id": accountId, "bot_id": bot.Id, "status": sysin.BotCodeStatusPending, "expires_at": expiresAt, "created_at": now, "updated_at": now}).Insert()
	if err != nil {
		return nil, gerror.Wrap(err, "创建Telegram验证码失败")
	}
	return &sysin.CodeStartModel{Code: code, Scene: scene, App: app, BotUsername: bot.BotUsername, ExpiresAt: expiresAt}, nil
}

func (s *sSysBot) codeStatus(ctx context.Context, code, scene, app string, accountId int64) (*sysin.CodeStatusModel, error) {
	if strings.TrimSpace(code) == "" {
		return nil, gerror.New("验证码不能为空")
	}
	var codeRow *authCodeRow
	mod := g.DB().Model(authCodeTable).Safe().Ctx(ctx).Where("code", code).Where("scene", scene)
	if app != "" {
		mod = mod.Where("app", app)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	if err := mod.Scan(&codeRow); err != nil {
		return nil, gerror.Wrap(err, "读取验证码状态失败")
	}
	if codeRow == nil {
		return nil, gerror.New("验证码不存在")
	}
	if codeRow.ExpiresAt != nil && codeRow.ExpiresAt.Before(gtime.Now()) && codeRow.Status == sysin.BotCodeStatusPending {
		_, _ = g.DB().Model(authCodeTable).Safe().Ctx(ctx).Where("code", code).Data(g.Map{"status": sysin.BotCodeStatusExpired, "error_message": "验证码已过期", "updated_at": gtime.Now()}).Update()
		codeRow.Status = sysin.BotCodeStatusExpired
		codeRow.ErrorMessage = "验证码已过期"
	}
	row := codeRow.statusModel()
	if scene == sysin.BotCodeSceneLogin && row.Status == sysin.BotCodeStatusAuthorized {
		if strings.TrimSpace(row.Token) == "" {
			row.Status = sysin.BotCodeStatusFailed
			row.ErrorMessage = "登录凭证生成失败，请重新获取验证码"
			_, _ = s.markCodeFailed(ctx, row.Code, row.Status, row.ErrorMessage)
			row.Token = ""
			row.AccessToken = ""
			return row, nil
		}
		profile, profileErr := s.loginAccountProfile(ctx, row.App, row.AccountId)
		if profileErr != nil {
			row.Status = sysin.BotCodeStatusFailed
			row.ErrorMessage = profileErr.Error()
			_, _ = s.markCodeFailed(ctx, row.Code, row.Status, row.ErrorMessage)
			row.Token = ""
			row.AccessToken = ""
			return row, nil
		}
		row.TenantId = profile.TenantId
		row.AccountType = profile.AccountType
		row.Username = profile.Username
		row.Nickname = profile.Nickname
		row.AccessToken = row.Token
	} else {
		row.Token = ""
		row.AccessToken = ""
	}
	return row, nil
}

func (s *sSysBot) uniqueCode(ctx context.Context) (string, error) {
	for i := 0; i < 20; i++ {
		code := fmt.Sprintf("%06d", grand.N(100000, 999999))
		count, err := g.DB().Model(authCodeTable).Safe().Ctx(ctx).Where("code", code).Where("status", sysin.BotCodeStatusPending).Count()
		if err != nil {
			return "", gerror.Wrap(err, "生成验证码失败")
		}
		if count == 0 {
			return code, nil
		}
	}
	return "", gerror.New("生成验证码失败，请重试")
}

func (s *sSysBot) TelegramWebhookRaw(ctx context.Context, in *sysin.WebhookInp) (err error) {
	if in == nil || len(in.Body) == 0 {
		return gerror.New("Webhook消息不能为空")
	}
	var update models.Update
	if err = json.Unmarshal(in.Body, &update); err != nil {
		return gerror.Wrap(err, "解析Telegram消息失败")
	}
	botId := in.BotId
	if botId <= 0 {
		botId = s.fallbackWebhookBotId(ctx)
	}
	g.Log().Infof(ctx, "处理Telegram Webhook updateId:%d botId:%d", update.ID, botId)
	return s.handleUpdate(ctx, botId, &update)
}

func (s *sSysBot) consumeCode(ctx context.Context, botId int64, msg *models.Message, code string) error {
	var row *authCodeRow
	if err := g.DB().Model(authCodeTable).Safe().Ctx(ctx).Where("code", code).Where("status", sysin.BotCodeStatusPending).Scan(&row); err != nil {
		return gerror.Wrap(err, "读取验证码失败")
	}
	chatId := fmt.Sprintf("%d", msg.Chat.ID)
	if row == nil {
		return s.reply(ctx, botId, chatId, "验证码不存在或已失效，请在页面重新生成。")
	}
	if row.ExpiresAt != nil && row.ExpiresAt.Before(gtime.Now()) {
		_, _ = s.markCodeFailed(ctx, code, sysin.BotCodeStatusExpired, "验证码已过期")
		return s.reply(ctx, botId, chatId, "验证码已过期，请在页面重新生成。")
	}
	if row.Scene == sysin.BotCodeSceneBind {
		return s.consumeBindCode(ctx, botId, msg, row)
	}
	return s.consumeLoginCode(ctx, botId, msg, row)
}

func (s *sSysBot) consumeBindCode(ctx context.Context, botId int64, msg *models.Message, row *authCodeRow) error {
	if row.AccountId <= 0 {
		_, _ = s.markCodeFailed(ctx, row.Code, sysin.BotCodeStatusFailed, "绑定账号不存在")
		return nil
	}
	telegramUserId := fmt.Sprintf("%d", msg.From.ID)
	telegramUsername := strings.TrimPrefix(strings.TrimSpace(msg.From.Username), "@")
	conflict, err := s.activeTelegramBindingConflict(ctx, row.App, telegramUserId, row.AccountId)
	if err != nil {
		return gerror.Wrap(err, "检查Telegram绑定失败")
	}
	if conflict {
		_, _ = s.markCodeFailed(ctx, row.Code, sysin.BotCodeStatusFailed, "该Telegram账号已绑定其他账号")
		return s.reply(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), "当前 Telegram 已绑定其他账号，不能重复绑定。")
	}
	now := gtime.Now()
	data := g.Map{"app": row.App, "account_id": row.AccountId, "telegram_user_id": telegramUserId, "telegram_username": telegramUsername, "telegram_first_name": msg.From.FirstName, "telegram_last_name": msg.From.LastName, "bot_id": botId, "status": 1, "updated_at": now}
	var exists *struct {
		Id int64 `json:"id"`
	}
	_ = g.DB().Model(accountBindTbl).Safe().Ctx(ctx).
		Fields("id").
		Where("app", row.App).
		Where("account_id", row.AccountId).
		WhereNull("deleted_at").
		OrderAsc("status").
		OrderDesc("id").
		Scan(&exists)
	if exists != nil && exists.Id > 0 {
		_, err = g.DB().Model(accountBindTbl).Safe().Ctx(ctx).Where("id", exists.Id).Data(data).Update()
	} else {
		data["created_at"] = now
		_, err = g.DB().Model(accountBindTbl).Safe().Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存Telegram绑定失败")
	}
	_, err = g.DB().Model(authCodeTable).Safe().Ctx(ctx).Where("code", row.Code).Data(g.Map{"status": sysin.BotCodeStatusAuthorized, "telegram_user_id": telegramUserId, "telegram_username": telegramUsername, "bot_id": botId, "updated_at": now}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新绑定验证码失败")
	}
	text, markup := s.bindSuccessMessage(ctx, row.App, row.AccountId, telegramUserId, telegramUsername)
	replyErr := s.replyWithMarkup(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), text, markup)
	for _, hookErr := range service.TriggerAccountBoundHooks(ctx, &service.AccountBoundEvent{
		App:              row.App,
		AccountId:        row.AccountId,
		BotId:            botId,
		TelegramUserId:   telegramUserId,
		TelegramUsername: telegramUsername,
	}) {
		g.Log().Warningf(ctx, "Telegram绑定后置处理失败 accountId:%d err:%+v", row.AccountId, hookErr)
	}
	_ = s.notifySuperAdmins(ctx, botId, superNotifyBind, botBindNotifyText(row.App, row.AccountId, telegramUsername))
	return replyErr
}

func (s *sSysBot) activeTelegramBindingConflict(ctx context.Context, app string, telegramUserId string, accountId int64) (bool, error) {
	count, err := g.DB().Model(accountBindTbl).Safe().Ctx(ctx).
		Where("app", strings.TrimSpace(app)).
		Where("telegram_user_id", strings.TrimSpace(telegramUserId)).
		WhereNot("account_id", accountId).
		Where("status", consts.StatusEnabled).
		WhereNull("deleted_at").
		Count()
	return count > 0, err
}

func (s *sSysBot) consumeLoginCode(ctx context.Context, botId int64, msg *models.Message, row *authCodeRow) error {
	telegramUserId := fmt.Sprintf("%d", msg.From.ID)
	bind, err := s.bindingByTelegram(ctx, row.App, telegramUserId)
	if err != nil {
		return err
	}
	if bind == nil || bind.AccountId <= 0 {
		_, _ = s.markCodeFailed(ctx, row.Code, sysin.BotCodeStatusFailed, "当前Telegram未绑定账号")
		return s.reply(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), "当前 Telegram 未绑定账号，请先在个人页面完成绑定。")
	}
	login, err := s.loginBoundAccount(ctx, row.App, bind.AccountId)
	if err != nil {
		_, _ = s.markCodeFailed(ctx, row.Code, sysin.BotCodeStatusFailed, err.Error())
		return s.reply(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), err.Error())
	}
	now := gtime.Now()
	_, err = g.DB().Model(authCodeTable).Safe().Ctx(ctx).Where("code", row.Code).Data(g.Map{"status": sysin.BotCodeStatusAuthorized, "telegram_user_id": telegramUserId, "telegram_username": strings.TrimPrefix(msg.From.Username, "@"), "bot_id": botId, "login_token": login.Token, "account_id": bind.AccountId, "updated_at": now}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新登录验证码失败")
	}
	return s.reply(ctx, botId, fmt.Sprintf("%d", msg.Chat.ID), "登录确认成功，请回到网页继续。")
}

func (s *sSysBot) markCodeFailed(ctx context.Context, code, status, message string) (int64, error) {
	res, err := g.DB().Model(authCodeTable).Safe().Ctx(ctx).Where("code", code).Data(g.Map{"status": status, "error_message": message, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
