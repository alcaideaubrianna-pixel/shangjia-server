package sys

import (
	"context"
	"fmt"
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
	"github.com/gotd/td/tg"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/contexts"
)

func (s *sSysPublish) AdminTgAccountList(ctx context.Context, in *sysin.TgAccountListInp) (list []*sysin.TgAccountModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.TgAccountListInp{}
	}
	current, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	base := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Where("tenant_id", current.TenantId).
		WhereNull("deleted_at")
	base = applyTgAccountFilters(base, in)
	totalCount, err = base.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取TG账号总数失败")
	}
	if err = base.Page(in.Page, in.PerPage).OrderDesc("id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取TG账号列表失败")
	}
	if list == nil {
		list = []*sysin.TgAccountModel{}
	}
	return list, totalCount, nil
}

func (s *sSysPublish) ServerTgAccountList(ctx context.Context, in *sysin.TgAccountListInp) (list []*sysin.TgAccountModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.TgAccountListInp{}
	}
	base := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).WhereNull("deleted_at")
	if in.TenantId > 0 {
		base = base.Where("tenant_id", in.TenantId)
	}
	base = applyTgAccountFilters(base, in)
	totalCount, err = base.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取TG账号总数失败")
	}
	if err = base.Page(in.Page, in.PerPage).OrderDesc("id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取TG账号列表失败")
	}
	if list == nil {
		list = []*sysin.TgAccountModel{}
	}
	return list, totalCount, nil
}

func (s *sSysPublish) AdminTgAccountStartLogin(ctx context.Context, in *sysin.TgAccountStartLoginInp) (res *sysin.TgAccountModel, err error) {
	if in == nil {
		in = &sysin.TgAccountStartLoginInp{}
	}
	current, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	in.TenantId = current.TenantId
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return nil, err
	}
	if conf.AppId <= 0 || strings.TrimSpace(conf.AppHash) == "" {
		return nil, gerror.New("请先在插件配置中填写Telegram App ID和App Hash")
	}
	now := gtime.Now()
	token := grand.S(40)
	expiresAt := now.Add(5 * time.Minute)
	sessionKey, sessionPath, err := s.telegramSessionPath(current.TenantId, current.Id, token)
	if err != nil {
		return nil, err
	}
	id, err := g.DB().Model(publishTgLoginTable).Safe().Ctx(ctx).Data(g.Map{
		"tenant_id":   current.TenantId,
		"merchant_id": current.TenantId,
		"account_id":  current.Id,
		"login_token": token,
		"session_key": sessionKey,
		"status":      tgLoginStatusPending,
		"expires_at":  expiresAt,
		"created_at":  now,
		"updated_at":  now,
	}).InsertAndGetId()
	if err != nil {
		return nil, gerror.Wrap(err, "创建TG账号扫码会话失败")
	}
	loginCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	runtime := &telegramLoginRuntime{
		loginToken:       token,
		accountId:        current.Id,
		tenantId:         current.TenantId,
		adminTgAccount:   true,
		tgAccountName:    in.DisplayName,
		tgAccountRemark:  in.Remark,
		tgLoginSessionId: id,
		cancel:           cancel,
		passwordCh:       make(chan string),
	}
	s.storeLoginRuntime(token, runtime)
	go s.runTelegramLogin(loginCtx, runtime, conf, current.TenantId, current.Id, sessionKey, sessionPath, s.updateTelegramLoginStatus)
	return s.adminTgAccountLoginById(ctx, id, current.TenantId, current.Id)
}

func (s *sSysPublish) AdminTgAccountLoginStatus(ctx context.Context, in *sysin.TgAccountLoginStatusInp) (res *sysin.TgAccountModel, err error) {
	if in == nil || strings.TrimSpace(in.LoginToken) == "" {
		return nil, gerror.New("登录令牌不能为空")
	}
	current, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.adminTgAccountLoginByToken(ctx, strings.TrimSpace(in.LoginToken), current.TenantId, current.Id)
	if err != nil {
		return nil, err
	}
	if item.Status == sysin.PublishTgAccountStatusAuthorized {
		return s.adminTgAccountByToken(ctx, strings.TrimSpace(in.LoginToken), current.TenantId, current.Id)
	}
	if item.ExpiresAt != nil && item.ExpiresAt.Before(gtime.Now()) {
		if _, err = g.DB().Model(publishTgLoginTable).Safe().Ctx(ctx).
			Where("tenant_id", current.TenantId).
			Where("account_id", current.Id).
			Where("login_token", strings.TrimSpace(in.LoginToken)).
			Data(g.Map{
				"status":        sysin.PublishTgAccountStatusExpired,
				"error_message": "扫码会话已过期",
				"updated_at":    gtime.Now(),
			}).
			Update(); err != nil {
			return nil, gerror.Wrap(err, "更新TG账号扫码状态失败")
		}
		return s.adminTgAccountLoginByToken(ctx, strings.TrimSpace(in.LoginToken), current.TenantId, current.Id)
	}
	return item, nil
}

func (s *sSysPublish) AdminTgAccountPassword(ctx context.Context, in *sysin.TgAccountPasswordInp) (res *sysin.TgAccountModel, err error) {
	current, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || strings.TrimSpace(in.LoginToken) == "" {
		return nil, gerror.New("登录令牌不能为空")
	}
	password := strings.TrimSpace(in.Password)
	if password == "" {
		return nil, gerror.New("二次验证密码不能为空")
	}
	item, err := s.AdminTgAccountLoginStatus(ctx, &sysin.TgAccountLoginStatusInp{LoginToken: in.LoginToken})
	if err != nil {
		return nil, err
	}
	if item.Status != tgLoginStatusPasswordRequired {
		return nil, gerror.New("当前扫码会话不需要二次验证密码")
	}
	if item.TenantId != current.TenantId || item.AccountId != current.Id {
		return nil, gerror.New("TG账号扫码会话不存在")
	}
	runtime := s.getLoginRuntime(strings.TrimSpace(in.LoginToken), current.Id)
	if runtime == nil {
		return nil, gerror.New("扫码登录会话已失效，请重新发起登录")
	}
	if err = s.updateTelegramLoginStatus(ctx, strings.TrimSpace(in.LoginToken), current.Id, g.Map{
		"error_message": "",
	}); err != nil {
		return nil, err
	}
	select {
	case runtime.passwordCh <- password:
	case <-time.After(10 * time.Second):
		return nil, gerror.New("提交二次验证密码超时，请重试")
	}
	return s.waitAdminTgAccountPasswordResult(ctx, strings.TrimSpace(in.LoginToken), current.TenantId, current.Id)
}

func (s *sSysPublish) AdminTgAccountDelete(ctx context.Context, in *sysin.TgAccountDeleteInp) (err error) {
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的TG账号")
	}
	in.Ids = uniqueIds(in.Ids)
	current, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if err = s.ensureTgAccountsBelongTenant(ctx, in.Ids, current.TenantId); err != nil {
		return err
	}
	if _, err = g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		WhereIn("id", in.Ids).
		Where("tenant_id", current.TenantId).
		Data(g.Map{
			"deleted_by": current.Id,
			"deleted_at": gtime.Now(),
		}).
		Update(); err != nil {
		return gerror.Wrap(err, "删除TG账号失败")
	}
	return nil
}

func (s *sSysPublish) ServerTgAccountDelete(ctx context.Context, in *sysin.TgAccountDeleteInp) (err error) {
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的TG账号")
	}
	in.Ids = uniqueIds(in.Ids)
	if _, err = g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		WhereIn("id", in.Ids).
		Data(g.Map{
			"deleted_by": contexts.GetUserId(ctx),
			"deleted_at": gtime.Now(),
		}).
		Update(); err != nil {
		return gerror.Wrap(err, "删除TG账号失败")
	}
	return nil
}

func (s *sSysPublish) AdminTgAccountRefresh(ctx context.Context, in *sysin.TgAccountRefreshInp) (list []*sysin.TgAccountRefreshModel, err error) {
	if in == nil || len(in.Ids) == 0 {
		return nil, gerror.New("请选择要刷新的TG账号")
	}
	in.Ids = uniqueIds(in.Ids)
	current, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.ensureTgAccountsBelongTenant(ctx, in.Ids, current.TenantId); err != nil {
		return nil, err
	}
	list = make([]*sysin.TgAccountRefreshModel, 0, len(in.Ids))
	for _, id := range uniqueIds(in.Ids) {
		status, message := s.refreshAdminTgAccountSession(ctx, id, current.TenantId, current.Id)
		list = append(list, &sysin.TgAccountRefreshModel{
			Id:           id,
			Status:       status,
			ErrorMessage: message,
		})
	}
	return list, nil
}

func (s *sSysPublish) ServerTgAccountRefresh(ctx context.Context, in *sysin.TgAccountRefreshInp) (list []*sysin.TgAccountRefreshModel, err error) {
	if in == nil || len(in.Ids) == 0 {
		return nil, gerror.New("请选择要刷新的TG账号")
	}
	in.Ids = uniqueIds(in.Ids)
	list = make([]*sysin.TgAccountRefreshModel, 0, len(in.Ids))
	for _, id := range in.Ids {
		item, itemErr := s.adminTgAccountById(ctx, id, 0)
		if itemErr != nil {
			list = append(list, &sysin.TgAccountRefreshModel{
				Id:           id,
				Status:       sysin.PublishTgAccountStatusFailed,
				ErrorMessage: itemErr.Error(),
			})
			continue
		}
		status, message := s.refreshAdminTgAccountSession(ctx, id, item.TenantId, contexts.GetUserId(ctx))
		list = append(list, &sysin.TgAccountRefreshModel{
			Id:           id,
			Status:       status,
			ErrorMessage: message,
		})
	}
	return list, nil
}

func applyTgAccountFilters(mod *gdb.Model, in *sysin.TgAccountListInp) *gdb.Model {
	if in.Status != "" {
		mod = mod.Where("status", strings.TrimSpace(in.Status))
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(display_name LIKE ? OR telegram_username LIKE ? OR telegram_user_id LIKE ? OR remark LIKE ?)", like, like, like, like)
	}
	return mod
}

func (s *sSysPublish) adminTgAccountById(ctx context.Context, id int64, tenantId int64) (*sysin.TgAccountModel, error) {
	var item *sysin.TgAccountModel
	mod := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Where("id", id).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if err := mod.Scan(&item); err != nil {
		return nil, gerror.Wrap(err, "读取TG账号失败")
	}
	if item == nil || item.Id <= 0 {
		return nil, gerror.New("TG账号不存在")
	}
	return item, nil
}

func (s *sSysPublish) adminTgAccountByToken(ctx context.Context, token string, tenantId int64, accountId int64) (*sysin.TgAccountModel, error) {
	var item *sysin.TgAccountModel
	mod := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Where("login_token", strings.TrimSpace(token)).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	if err := mod.Scan(&item); err != nil {
		return nil, gerror.Wrap(err, "读取TG账号失败")
	}
	if item == nil || item.Id <= 0 {
		return nil, gerror.New("TG账号不存在")
	}
	return item, nil
}

func (s *sSysPublish) adminTgAccountLoginById(ctx context.Context, id int64, tenantId int64, accountId int64) (*sysin.TgAccountModel, error) {
	var item *sysin.TgAccountModel
	mod := g.DB().Model(publishTgLoginTable).Safe().Ctx(ctx).Where("id", id)
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	if err := mod.Scan(&item); err != nil {
		return nil, gerror.Wrap(err, "读取TG账号扫码会话失败")
	}
	if item == nil || item.Id <= 0 {
		return nil, gerror.New("TG账号扫码会话不存在")
	}
	return item, nil
}

func (s *sSysPublish) adminTgAccountLoginByToken(ctx context.Context, token string, tenantId int64, accountId int64) (*sysin.TgAccountModel, error) {
	var item *sysin.TgAccountModel
	mod := g.DB().Model(publishTgLoginTable).Safe().Ctx(ctx).Where("login_token", strings.TrimSpace(token))
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	if err := mod.Scan(&item); err != nil {
		return nil, gerror.Wrap(err, "读取TG账号扫码状态失败")
	}
	if item == nil || item.Id <= 0 {
		return nil, gerror.New("TG账号扫码会话不存在")
	}
	return item, nil
}

func (s *sSysPublish) waitAdminTgAccountPasswordResult(ctx context.Context, token string, tenantId int64, accountId int64) (*sysin.TgAccountModel, error) {
	deadline := time.Now().Add(8 * time.Second)
	for {
		item, err := s.adminTgAccountLoginByToken(ctx, token, tenantId, accountId)
		if err != nil {
			return nil, err
		}
		if item.Status == sysin.PublishTgAccountStatusAuthorized {
			return s.adminTgAccountByToken(ctx, token, tenantId, accountId)
		}
		if item.Status == sysin.PublishTgAccountStatusFailed || item.Status == sysin.PublishTgAccountStatusExpired {
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

func (s *sSysPublish) refreshAdminTgAccountSession(ctx context.Context, id int64, tenantId int64, operatorId int64) (status string, message string) {
	item, err := s.adminTgAccountById(ctx, id, tenantId)
	if err != nil {
		return sysin.PublishTgAccountStatusFailed, err.Error()
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		message = err.Error()
		s.updateTgAccountRefreshResult(ctx, id, tenantId, operatorId, sysin.PublishTgAccountStatusFailed, message, nil, "", "")
		return sysin.PublishTgAccountStatusFailed, message
	}
	sessionPath, err := s.telegramSessionPathByKey(item.SessionKey)
	if err != nil {
		message = err.Error()
		s.updateTgAccountRefreshResult(ctx, id, tenantId, operatorId, sysin.PublishTgAccountStatusFailed, message, nil, "", "")
		return sysin.PublishTgAccountStatusFailed, message
	}
	options := telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{Path: sessionPath},
	}
	if resolver, err := telegramMTProtoResolver(conf.ProxyUrl); err != nil {
		message = err.Error()
		s.updateTgAccountRefreshResult(ctx, id, tenantId, operatorId, sysin.PublishTgAccountStatusFailed, message, nil, "", "")
		return sysin.PublishTgAccountStatusFailed, message
	} else if resolver != nil {
		options.Resolver = resolver
	}
	client := telegram.NewClient(conf.AppId, conf.AppHash, options)
	var self *tg.User
	runCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	err = client.Run(runCtx, func(ctx context.Context) error {
		user, err := client.Self(ctx)
		if err != nil {
			return err
		}
		self = user
		return nil
	})
	if err != nil {
		message = err.Error()
		s.updateTgAccountRefreshResult(context.Background(), id, tenantId, operatorId, sysin.PublishTgAccountStatusFailed, message, nil, "", "")
		return sysin.PublishTgAccountStatusFailed, message
	}
	username := ""
	displayName := ""
	if self != nil {
		username = self.Username
		displayName = strings.TrimSpace(strings.TrimSpace(self.FirstName + " " + self.LastName))
	}
	s.updateTgAccountRefreshResult(ctx, id, tenantId, operatorId, sysin.PublishTgAccountStatusAuthorized, "", self, username, displayName)
	return sysin.PublishTgAccountStatusAuthorized, ""
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
