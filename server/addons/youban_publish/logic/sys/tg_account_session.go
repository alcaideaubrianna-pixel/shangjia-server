package sys

import (
	"context"
	"fmt"
	"html"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gfile"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botService "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
)

func (s *sSysPublish) refreshAdminTgAccountSession(ctx context.Context, id int64, tenantId int64, operatorId int64) (status string, message string) {
	item, err := s.adminTgAccountById(ctx, id, tenantId)
	if err != nil {
		return sysin.PublishTgAccountStatusFailed, err.Error()
	}
	var user *tg.User
	usedRuntime, err := s.executeAccountCollectOperation(ctx, item.Id, 30*time.Second, func(runCtx context.Context, client *telegram.Client) error {
		user, err = client.Self(runCtx)
		return err
	})
	if !usedRuntime {
		conf, confErr := NewSysConfig().GetTelegram(ctx)
		if confErr != nil {
			return s.failTgAccountRefresh(ctx, id, tenantId, operatorId, confErr.Error())
		}
		storage, storageErr := s.telegramSessionStorage(item.SessionKey)
		if storageErr != nil {
			return s.failTgAccountRefresh(ctx, id, tenantId, operatorId, storageErr.Error())
		}
		options := telegram.Options{SessionStorage: storage}
		if resolver, resolverErr := telegramMTProtoResolver(conf.ProxyUrl); resolverErr != nil {
			return s.failTgAccountRefresh(ctx, id, tenantId, operatorId, resolverErr.Error())
		} else if resolver != nil {
			options.Resolver = resolver
		}
		user, err = s.readTelegramSelf(ctx, conf.AppId, conf.AppHash, options)
	}
	if err != nil {
		if isTelegramPermanentAccountAuthError(err) {
			return s.expireTgAccountSession(
				context.Background(),
				id,
				tenantId,
				operatorId,
				telegramPermanentAccountAuthMessage(err),
			)
		}
		return s.failTgAccountRefresh(context.Background(), id, tenantId, operatorId, err.Error())
	}
	username := ""
	displayName := ""
	if user != nil {
		username = user.Username
		displayName = strings.TrimSpace(user.FirstName + " " + user.LastName)
	}
	s.updateTgAccountRefreshResult(ctx, id, tenantId, operatorId, sysin.PublishTgAccountStatusAuthorized, "", user, username, displayName)
	return sysin.PublishTgAccountStatusAuthorized, ""
}

func (s *sSysPublish) failTgAccountRefresh(ctx context.Context, id int64, tenantId int64, operatorId int64, message string) (string, string) {
	s.updateTgAccountRefreshResult(ctx, id, tenantId, operatorId, sysin.PublishTgAccountStatusFailed, message, nil, "", "")
	return sysin.PublishTgAccountStatusFailed, message
}

const tgAccountSessionExpiredMessage = "TG账号登录态已失效，请重新扫码登录"

func (s *sSysPublish) expireTgAccountSession(ctx context.Context, id int64, tenantId int64, operatorId int64, message string) (string, string) {
	s.updateTgAccountRefreshResult(ctx, id, tenantId, operatorId, sysin.PublishTgAccountStatusExpired, message, nil, "", "")
	return sysin.PublishTgAccountStatusExpired, message
}

func isTelegramAuthKeyUnregistered(err error) bool {
	return err != nil && strings.Contains(err.Error(), "AUTH_KEY_UNREGISTERED")
}

func isTelegramPermanentAccountAuthError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToUpper(err.Error())
	permanentParts := []string{
		"AUTH_KEY_DUPLICATED",
		"AUTH_KEY_UNREGISTERED",
		"SESSION_REVOKED",
		"USER_DEACTIVATED",
		"USER_DEACTIVATED_BAN",
	}
	for _, part := range permanentParts {
		if strings.Contains(message, part) {
			return true
		}
	}
	return false
}

func telegramPermanentAccountAuthMessage(err error) string {
	if err == nil {
		return tgAccountSessionExpiredMessage
	}
	message := strings.ToUpper(err.Error())
	switch {
	case strings.Contains(message, "AUTH_KEY_DUPLICATED"):
		return "TG账号授权密钥被重复使用，Telegram 已作废该登录态，请重新扫码登录"
	case strings.Contains(message, "USER_DEACTIVATED_BAN"):
		return "TG账号已被 Telegram 封禁，已自动停用，请更换账号"
	case strings.Contains(message, "USER_DEACTIVATED"):
		return "TG账号已被 Telegram 停用或注销，已自动停用，请更换账号"
	case strings.Contains(message, "SESSION_REVOKED"):
		return "TG账号会话已被撤销，已自动停用，请重新扫码登录"
	case strings.Contains(message, "AUTH_KEY_UNREGISTERED"):
		return tgAccountSessionExpiredMessage
	default:
		return tgAccountSessionExpiredMessage
	}
}

func (s *sSysPublish) handleTgAccountPermanentAuthError(ctx context.Context, tgAccountId int64, operatorId int64, message string, cause error) {
	account, err := s.notifyTgAccountOwner(ctx, tgAccountId, 0)
	if err != nil {
		g.Log().Warningf(ctx, "读取失效TG账号失败 tgAccountId:%d err:%+v", tgAccountId, err)
		return
	}
	if account.Id <= 0 {
		return
	}
	if strings.TrimSpace(message) == "" {
		message = telegramPermanentAccountAuthMessage(cause)
	}
	s.expireTgAccountSession(ctx, account.Id, account.TenantId, operatorId, message)
	if account.AccountId <= 0 {
		return
	}
	text := fmt.Sprintf("TG账号已自动停用。\n\nTG账号：%s\n原因：%s", html.EscapeString(firstNonEmpty(account.DisplayName, account.TelegramUsername, fmt.Sprintf("ID:%d", account.Id))), html.EscapeString(message))
	if notifyErr := botService.SysBot().NotifyAccount(ctx, &botsysin.NotifyAccountInp{
		App:       consts.AppApi,
		AccountId: account.AccountId,
		Text:      text,
		ParseMode: "HTML",
	}); notifyErr != nil {
		g.Log().Warningf(ctx, "发送TG账号自动停用通知失败 tgAccountId:%d accountId:%d err:%+v", account.Id, account.AccountId, notifyErr)
	}
}

func (s *sSysPublish) notifyTgAccountOwner(ctx context.Context, tgAccountId int64, tenantId int64) (*messagePushTgAccountOwner, error) {
	if tgAccountId <= 0 {
		return &messagePushTgAccountOwner{}, nil
	}
	var account *messagePushTgAccountOwner
	mod := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,account_id,display_name,telegram_username").
		Where("id", tgAccountId).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if err := mod.Scan(&account); err != nil {
		return nil, gerror.Wrap(err, "读取TG账号失败")
	}
	if account == nil {
		return &messagePushTgAccountOwner{}, nil
	}
	return account, nil
}

func (s *sSysPublish) readTelegramSelf(ctx context.Context, appId int, appHash string, options telegram.Options) (*tg.User, error) {
	client := telegram.NewClient(appId, appHash, options)
	var self *tg.User
	runCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	err := client.Run(runCtx, func(ctx context.Context) error {
		user, err := client.Self(ctx)
		if err != nil {
			return err
		}
		self = user
		return nil
	})
	return self, err
}

func (s *sSysPublish) updateTgAccountRefreshResult(ctx context.Context, id int64, tenantId int64, operatorId int64, status string, message string, user *tg.User, username string, displayName string) {
	data := g.Map{
		"status":        status,
		"error_message": message,
		"updated_by":    operatorId,
		"updated_at":    gtime.Now(),
	}
	if status == sysin.PublishTgAccountStatusAuthorized {
		data["last_login_at"] = gtime.Now()
		if user != nil {
			data["telegram_user_id"] = strconv.FormatInt(user.ID, 10)
			data["telegram_username"] = username
			data["telegram_first_name"] = user.FirstName
			data["telegram_last_name"] = user.LastName
			data["telegram_phone"] = user.Phone
			data["telegram_is_bot"] = boolToInt(user.Bot)
			if displayName != "" {
				data["display_name"] = displayName
			}
		}
	}
	_, _ = g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Where("id", id).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Data(data).
		Update()
}

func (s *sSysPublish) telegramSessionPathByKey(sessionKey string) (string, error) {
	parts := strings.Split(strings.TrimSpace(sessionKey), "/")
	if len(parts) != 3 || !strings.HasPrefix(parts[0], "tenant_") || !strings.HasPrefix(parts[1], "account_") {
		return "", gerror.New("TG账号会话路径无效，请重新登录")
	}
	tenantPart := strings.TrimPrefix(parts[0], "tenant_")
	accountPart := strings.TrimPrefix(parts[1], "account_")
	token := strings.TrimSuffix(parts[2], ".json")
	if tenantPart == "" || accountPart == "" || token == "" {
		return "", gerror.New("TG账号会话路径无效，请重新登录")
	}
	return filepath.Join(gfile.Pwd(), "runtime", "youban_publish", "telegram_sessions", "tenant_"+tenantPart, fmt.Sprintf("account_%s_%s.json", accountPart, token)), nil
}

func (s *sSysPublish) ensureTgAccountsBelongTenant(ctx context.Context, ids []int64, tenantId int64) error {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return gerror.New("请选择TG账号")
	}
	count, err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG账号权限失败")
	}
	if count != len(ids) {
		return gerror.New("存在无权操作的TG账号")
	}
	return nil
}

func (s *sSysPublish) resolveTenantTgAccountId(ctx context.Context, id int64, tenantId int64) (int64, error) {
	if id <= 0 || tenantId <= 0 {
		return 0, gerror.New("请选择TG账号")
	}
	account, err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).Unscoped().
		Where("id", id).
		Where("tenant_id", tenantId).
		One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取TG账号失败")
	}
	if account.IsEmpty() {
		return 0, gerror.New("TG账号不存在或不属于当前租户")
	}
	if account["deleted_at"].IsNil() && account["status"].String() == sysin.PublishTgAccountStatusAuthorized {
		return id, nil
	}
	telegramUserId := strings.TrimSpace(account["telegram_user_id"].String())
	if telegramUserId == "" {
		return 0, gerror.New("TG账号已失效，请重新选择账号")
	}
	replacement, err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("telegram_user_id", telegramUserId).
		Where("status", sysin.PublishTgAccountStatusAuthorized).
		WhereNull("deleted_at").
		OrderDesc("id").
		One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取TG账号最新授权失败")
	}
	if replacement.IsEmpty() {
		return 0, gerror.New("TG账号已失效，请重新扫码登录")
	}
	return replacement["id"].Int64(), nil
}

func (s *sSysPublish) ensureTgAccountsBelongAccount(ctx context.Context, ids []int64, tenantId int64, accountId int64) error {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return gerror.New("请选择TG账号")
	}
	count, err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG账号权限失败")
	}
	if count != len(ids) {
		return gerror.New("存在无权操作的TG账号")
	}
	return nil
}

func (s *sSysPublish) ensureTgAccountBelongsAccount(ctx context.Context, id int64, tenantId int64, accountId int64) error {
	return s.ensureTgAccountsBelongAccount(ctx, []int64{id}, tenantId, accountId)
}

func uniqueIds(ids []int64) []int64 {
	if len(ids) == 0 {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(ids))
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
