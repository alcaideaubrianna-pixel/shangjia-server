package sys

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/grand"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"golang.org/x/net/proxy"

	"hotgo/addons/youban_publish/model"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const (
	tgLoginStatusPending          = "pending"
	tgLoginStatusScanning         = "scanning"
	tgLoginStatusPasswordRequired = "password_required"
	tgLoginStatusAuthorized       = "authorized"
	tgLoginStatusFailed           = "failed"
	tgLoginStatusExpired          = "expired"
)

type telegramLoginRuntime struct {
	loginToken       string
	accountId        int64
	adminTgAccount   bool
	tenantId         int64
	tgAccountId      int64
	tgAccountName    string
	tgAccountRemark  string
	tgLoginSessionId int64
	cancel           context.CancelFunc
	passwordCh       chan string
}

func (s *sSysPublish) TelegramLoginStart(ctx context.Context, in *sysin.TelegramLoginStartInp) (res *sysin.TelegramLoginModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return nil, err
	}
	if conf.AppId <= 0 || strings.TrimSpace(conf.AppHash) == "" {
		return nil, gerror.New("请先在插件配置中填写Telegram App ID和App Hash")
	}

	s.cancelAccountLogin(account.Id)

	now := gtime.Now()
	token := grand.S(40)
	expiresAt := now.Add(5 * time.Minute)
	sessionKey, sessionPath, err := s.telegramSessionPath(account.TenantId, account.Id, token)
	if err != nil {
		return nil, err
	}
	data := g.Map{
		"tenant_id":   account.TenantId,
		"merchant_id": account.TenantId,
		"account_id":  account.Id,
		"login_token": token,
		"session_key": sessionKey,
		"status":      tgLoginStatusPending,
		"expires_at":  expiresAt,
		"created_at":  now,
		"updated_at":  now,
	}
	id, err := g.DB().Model(publishTgLoginTable).Safe().Ctx(ctx).Data(data).InsertAndGetId()
	if err != nil {
		return nil, gerror.Wrap(err, "创建Telegram扫码登录会话失败")
	}

	loginCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	runtime := &telegramLoginRuntime{
		loginToken: token,
		accountId:  account.Id,
		tenantId:   account.TenantId,
		cancel:     cancel,
		passwordCh: make(chan string),
	}
	s.storeLoginRuntime(token, runtime)
	go s.runTelegramLogin(loginCtx, runtime, conf, account.TenantId, account.Id, sessionKey, sessionPath, s.updateTelegramLoginStatus)

	return s.telegramLoginById(ctx, id, account.Id)
}

func (s *sSysPublish) TelegramLoginStatus(ctx context.Context, in *sysin.TelegramLoginStatusInp) (res *sysin.TelegramLoginModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(in.LoginToken)
	if token == "" {
		return nil, gerror.New("登录令牌不能为空")
	}
	return s.telegramLoginByToken(ctx, token, account.Id)
}

func (s *sSysPublish) TelegramLoginPassword(ctx context.Context, in *sysin.TelegramLoginPasswordInp) (res *sysin.TelegramLoginModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(in.LoginToken)
	password := in.Password
	if token == "" {
		return nil, gerror.New("登录令牌不能为空")
	}
	if password == "" {
		return nil, gerror.New("二次验证密码不能为空")
	}
	item, err := s.telegramLoginByToken(ctx, token, account.Id)
	if err != nil {
		return nil, err
	}
	if item.Status != tgLoginStatusPasswordRequired {
		return nil, gerror.New("当前扫码会话不需要二次验证密码")
	}
	runtime := s.getLoginRuntime(token, account.Id)
	if runtime == nil {
		return nil, gerror.New("扫码登录会话已失效，请重新发起登录")
	}
	if err = s.updateTelegramLoginStatus(ctx, token, account.Id, g.Map{
		"error_message": "",
	}); err != nil {
		return nil, err
	}
	select {
	case runtime.passwordCh <- password:
	case <-time.After(10 * time.Second):
		return nil, gerror.New("提交二次验证密码超时，请重试")
	}
	return s.waitTelegramPasswordResult(ctx, token, account.Id)
}

type telegramLoginStatusUpdater func(ctx context.Context, token string, accountId int64, data g.Map) error

func (s *sSysPublish) runTelegramLogin(ctx context.Context, runtime *telegramLoginRuntime, conf *model.TelegramConfig, tenantId int64, accountId int64, sessionKey string, sessionPath string, updateStatus telegramLoginStatusUpdater) {
	defer func() {
		runtime.cancel()
		s.removeLoginRuntime(runtime.loginToken)
	}()

	dispatcher := tg.NewUpdateDispatcher()
	loggedIn := qrlogin.OnLoginToken(dispatcher)
	storage, err := s.telegramSessionStorage(sessionKey)
	if err != nil {
		s.markTelegramLoginFailed(context.Background(), runtime.loginToken, accountId, err.Error(), updateStatus)
		return
	}
	options := telegram.Options{
		SessionStorage: storage,
		UpdateHandler:  dispatcher,
	}
	if resolver, err := telegramMTProtoResolver(conf.ProxyUrl); err != nil {
		s.markTelegramLoginFailed(context.Background(), runtime.loginToken, accountId, err.Error(), updateStatus)
		return
	} else if resolver != nil {
		options.Resolver = resolver
	}

	client := telegram.NewClient(conf.AppId, conf.AppHash, options)
	err = client.Run(ctx, func(runCtx context.Context) error {
		qr := client.QR()
		authorization, err := qr.Auth(runCtx, loggedIn, func(showCtx context.Context, token qrlogin.Token) error {
			return updateStatus(showCtx, runtime.loginToken, accountId, g.Map{
				"qr_url":        token.URL(),
				"status":        tgLoginStatusScanning,
				"error_message": "",
				"expires_at":    gtime.New(token.Expires()),
			})
		})
		if telegramPasswordNeeded(err) {
			updateStatus(runCtx, runtime.loginToken, accountId, g.Map{
				"status":        tgLoginStatusPasswordRequired,
				"error_message": "",
				"expires_at":    gtime.Now().Add(5 * time.Minute),
			})
			authorization, err = s.waitTelegramPassword(runCtx, runtime, client)
		}
		if err != nil {
			return err
		}
		if runtime.adminTgAccount {
			return s.finishAdminTgAccountLogin(runCtx, runtime, tenantId, accountId, sessionKey, authorization)
		}
		return s.finishTelegramLogin(runCtx, runtime.loginToken, tenantId, accountId, sessionKey, authorization)
	})
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	status := tgLoginStatusFailed
	if errors.Is(err, context.DeadlineExceeded) {
		status = tgLoginStatusExpired
	}
	_ = updateStatus(context.Background(), runtime.loginToken, accountId, g.Map{
		"status":        status,
		"error_message": err.Error(),
	})
}

func (s *sSysPublish) waitTelegramPassword(ctx context.Context, runtime *telegramLoginRuntime, client *telegram.Client) (*tg.AuthAuthorization, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case password := <-runtime.passwordCh:
			authorization, err := client.Auth().Password(ctx, password)
			if errors.Is(err, auth.ErrPasswordInvalid) {
				_ = s.updateLoginRuntimeStatus(ctx, runtime, g.Map{
					"status":        tgLoginStatusPasswordRequired,
					"error_message": "二次验证密码错误，请重新输入",
					"expires_at":    gtime.Now().Add(5 * time.Minute),
				})
				continue
			}
			if err != nil {
				return nil, err
			}
			return authorization, nil
		}
	}
}

func (s *sSysPublish) finishTelegramLogin(ctx context.Context, token string, tenantId int64, accountId int64, sessionKey string, authorization *tg.AuthAuthorization) error {
	user, ok := authorization.User.AsNotEmpty()
	if !ok {
		return gerror.New("Telegram授权结果缺少用户信息")
	}
	username := user.Username
	if username == "" {
		username = strings.TrimSpace(strings.TrimSpace(user.FirstName + " " + user.LastName))
	}
	now := gtime.Now()
	if err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Model(publishTgLoginTable).Safe().Ctx(ctx).
			Where("login_token", token).
			Where("account_id", accountId).
			Data(g.Map{
				"telegram_user_id":  strconv.FormatInt(user.ID, 10),
				"telegram_username": username,
				"session_key":       sessionKey,
				"status":            tgLoginStatusAuthorized,
				"error_message":     "",
				"updated_at":        now,
			}).Update(); err != nil {
			return gerror.Wrap(err, "更新Telegram扫码登录状态失败")
		}
		if _, err := tx.Model(publishAccountTable).Safe().Ctx(ctx).
			Where("id", accountId).
			Where("tenant_id", tenantId).
			WhereNull("deleted_at").
			Data(g.Map{
				"telegram_user_id":  strconv.FormatInt(user.ID, 10),
				"telegram_username": username,
				"updated_at":        now,
			}).Update(); err != nil {
			return gerror.Wrap(err, "回写上架账号Telegram信息失败")
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (s *sSysPublish) finishAdminTgAccountLogin(ctx context.Context, runtime *telegramLoginRuntime, tenantId int64, accountId int64, sessionKey string, authorization *tg.AuthAuthorization) error {
	user, ok := authorization.User.AsNotEmpty()
	if !ok {
		return gerror.New("Telegram授权结果缺少用户信息")
	}
	username := user.Username
	telegramName := strings.TrimSpace(strings.TrimSpace(user.FirstName + " " + user.LastName))
	displayName := telegramName
	if displayName == "" {
		displayName = fmt.Sprintf("TG账号-%d", user.ID)
	}
	if runtime.tgAccountName != "" {
		displayName = runtime.tgAccountName
	}
	now := gtime.Now()
	if err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Model(publishTgLoginTable).Safe().Ctx(ctx).
			Where("login_token", runtime.loginToken).
			Where("account_id", accountId).
			Data(g.Map{
				"telegram_user_id":  strconv.FormatInt(user.ID, 10),
				"telegram_username": username,
				"session_key":       sessionKey,
				"status":            tgLoginStatusAuthorized,
				"error_message":     "",
				"updated_at":        now,
			}).Update(); err != nil {
			return gerror.Wrap(err, "更新TG账号扫码登录状态失败")
		}
		data := g.Map{
			"tenant_id":           tenantId,
			"merchant_id":         tenantId,
			"account_id":          accountId,
			"display_name":        displayName,
			"telegram_user_id":    strconv.FormatInt(user.ID, 10),
			"telegram_username":   username,
			"telegram_first_name": user.FirstName,
			"telegram_last_name":  user.LastName,
			"telegram_phone":      user.Phone,
			"telegram_is_bot":     boolToInt(user.Bot),
			"session_key":         sessionKey,
			"login_token":         runtime.loginToken,
			"qr_url":              "",
			"remark":              runtime.tgAccountRemark,
			"status":              sysin.PublishTgAccountStatusAuthorized,
			"error_message":       "",
			"last_login_at":       now,
			"created_by":          accountId,
			"updated_by":          accountId,
			"created_at":          now,
			"updated_at":          now,
			"deleted_by":          0,
			"deleted_at":          nil,
		}
		tgAccountId := runtime.tgAccountId
		if tgAccountId <= 0 {
			existing, findErr := tx.Model(publishTgAccountTable).Safe().Ctx(ctx).Unscoped().
				Where("tenant_id", tenantId).
				Where("telegram_user_id", strconv.FormatInt(user.ID, 10)).
				OrderDesc("id").
				LockUpdate().
				One()
			if findErr != nil {
				return gerror.Wrap(findErr, "读取TG账号历史授权失败")
			}
			tgAccountId = existing["id"].Int64()
		}
		if tgAccountId > 0 {
			delete(data, "created_by")
			delete(data, "created_at")
			if _, err := tx.Model(publishTgAccountTable).Safe().Ctx(ctx).Unscoped().
				Where("id", tgAccountId).
				Where("tenant_id", tenantId).
				Data(data).
				Update(); err != nil {
				return gerror.Wrap(err, "更新TG账号授权信息失败")
			}
		} else if _, err := tx.Model(publishTgAccountTable).Safe().Ctx(ctx).Data(data).Insert(); err != nil {
			return gerror.Wrap(err, "保存TG账号授权信息失败")
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func telegramPasswordNeeded(err error) bool {
	return errors.Is(err, auth.ErrPasswordAuthNeeded) || tgerr.Is(err, "SESSION_PASSWORD_NEEDED")
}

func (s *sSysPublish) telegramLoginById(ctx context.Context, id int64, accountId int64) (*sysin.TelegramLoginModel, error) {
	var item *sysin.TelegramLoginModel
	err := g.DB().Model(publishTgLoginTable).Safe().Ctx(ctx).
		Where("id", id).
		Where("account_id", accountId).
		Scan(&item)
	if err != nil {
		return nil, gerror.Wrap(err, "读取Telegram扫码登录会话失败")
	}
	if item == nil || item.Id <= 0 {
		return nil, gerror.New("扫码登录会话不存在")
	}
	return item, nil
}

func (s *sSysPublish) telegramLoginByToken(ctx context.Context, token string, accountId int64) (*sysin.TelegramLoginModel, error) {
	var item *sysin.TelegramLoginModel
	if err := g.DB().Model(publishTgLoginTable).Safe().Ctx(ctx).
		Where("login_token", token).
		Where("account_id", accountId).
		Scan(&item); err != nil {
		return nil, gerror.Wrap(err, "读取Telegram扫码登录状态失败")
	}
	if item == nil || item.Id <= 0 {
		return nil, gerror.New("扫码登录会话不存在")
	}
	return item, nil
}

func (s *sSysPublish) waitTelegramPasswordResult(ctx context.Context, token string, accountId int64) (*sysin.TelegramLoginModel, error) {
	deadline := time.Now().Add(8 * time.Second)
	for {
		item, err := s.telegramLoginByToken(ctx, token, accountId)
		if err != nil {
			return nil, err
		}
		if item.Status == tgLoginStatusAuthorized || item.Status == tgLoginStatusFailed || item.Status == tgLoginStatusExpired {
			return item, nil
		}
		if time.Now().After(deadline) {
			return item, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func (s *sSysPublish) updateTelegramLoginStatus(ctx context.Context, token string, accountId int64, data g.Map) error {
	data["updated_at"] = gtime.Now()
	_, err := g.DB().Model(publishTgLoginTable).Safe().Ctx(ctx).
		Where("login_token", token).
		Where("account_id", accountId).
		Data(data).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新Telegram扫码登录状态失败")
	}
	return nil
}

func (s *sSysPublish) updateLoginRuntimeStatus(ctx context.Context, runtime *telegramLoginRuntime, data g.Map) error {
	return s.updateTelegramLoginStatus(ctx, runtime.loginToken, runtime.accountId, data)
}

func (s *sSysPublish) markTelegramLoginFailed(ctx context.Context, token string, accountId int64, message string, updateStatus telegramLoginStatusUpdater) {
	_ = updateStatus(ctx, token, accountId, g.Map{
		"status":        tgLoginStatusFailed,
		"error_message": message,
	})
}

func (s *sSysPublish) storeLoginRuntime(token string, runtime *telegramLoginRuntime) {
	s.tgLoginMu.Lock()
	defer s.tgLoginMu.Unlock()
	if s.tgLogins == nil {
		s.tgLogins = make(map[string]*telegramLoginRuntime)
	}
	s.tgLogins[token] = runtime
}

func (s *sSysPublish) getLoginRuntime(token string, accountId int64) *telegramLoginRuntime {
	s.tgLoginMu.Lock()
	defer s.tgLoginMu.Unlock()
	runtime := s.tgLogins[token]
	if runtime == nil || runtime.accountId != accountId {
		return nil
	}
	return runtime
}

func (s *sSysPublish) removeLoginRuntime(token string) {
	s.tgLoginMu.Lock()
	defer s.tgLoginMu.Unlock()
	delete(s.tgLogins, token)
}

func (s *sSysPublish) cancelAccountLogin(accountId int64) {
	s.tgLoginMu.Lock()
	defer s.tgLoginMu.Unlock()
	for token, runtime := range s.tgLogins {
		if runtime.accountId == accountId {
			runtime.cancel()
			delete(s.tgLogins, token)
		}
	}
}

func (s *sSysPublish) telegramSessionPath(tenantId int64, accountId int64, token string) (sessionKey string, path string, err error) {
	dir := filepath.Join(gfile.Pwd(), "runtime", "youban_publish", "telegram_sessions", fmt.Sprintf("tenant_%d", tenantId))
	sessionKey = fmt.Sprintf("tenant_%d/account_%d/%s.json", tenantId, accountId, token)
	return sessionKey, filepath.Join(dir, fmt.Sprintf("account_%d_%s.json", accountId, token)), nil
}

func telegramMTProtoResolver(proxyURL string) (dcs.Resolver, error) {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return nil, nil
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, gerror.Wrap(err, "Telegram代理地址格式错误")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "socks5" && scheme != "socks5h" && scheme != "s5" {
		return nil, gerror.New("Telegram扫码登录仅支持socks5代理，例如 socks5://127.0.0.1:7890")
	}
	address := u.Host
	if address == "" {
		return nil, gerror.New("Telegram代理地址缺少host")
	}
	var authInfo *proxy.Auth
	if u.User != nil {
		password, _ := u.User.Password()
		authInfo = &proxy.Auth{User: u.User.Username(), Password: password}
	}
	dialer, err := proxy.SOCKS5("tcp", address, authInfo, proxy.Direct)
	if err != nil {
		return nil, gerror.Wrap(err, "初始化Telegram SOCKS5代理失败")
	}
	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, gerror.New("当前SOCKS5代理不支持ContextDialer")
	}
	return dcs.Plain(dcs.PlainOptions{Dial: contextDialer.DialContext}), nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
