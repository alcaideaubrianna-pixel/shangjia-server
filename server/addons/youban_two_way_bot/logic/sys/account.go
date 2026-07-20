package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	publishmodel "hotgo/addons/youban_publish/model"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	publishservice "hotgo/addons/youban_publish/service"
	"hotgo/internal/library/contexts"
)

type publishAccount struct {
	Id          int64  `json:"id"`
	TenantId    int64  `json:"tenant_id"`
	AccountType string `json:"account_type"`
	Username    string `json:"username"`
	Nickname    string `json:"nickname"`
}

type publishTgAccount struct {
	Id         int64  `json:"id"`
	TenantId   int64  `json:"tenant_id"`
	SessionKey string `json:"session_key"`
	Status     string `json:"status"`
}

func currentAdminAccount(ctx context.Context) (*publishAccount, error) {
	userId := contexts.GetUserId(ctx)
	if userId <= 0 {
		return nil, gerror.New("请先登录")
	}
	var account *publishAccount
	err := g.DB().Model("hg_youban_publish_account").Safe().Ctx(ctx).
		Where("id", userId).
		Where("status", 1).
		WhereNull("deleted_at").
		Scan(&account)
	if err != nil {
		return nil, gerror.Wrap(err, "读取当前上架账号失败")
	}
	if account == nil || account.Id <= 0 || account.TenantId <= 0 {
		return nil, gerror.New("当前用户未绑定上架账号")
	}
	if account.AccountType != publishsysin.PublishAccountTypeAdmin {
		return nil, gerror.New("当前账号无管理权限")
	}
	return account, nil
}

func ensureTgAccountBelongsTenant(ctx context.Context, tgAccountId int64, tenantId int64) error {
	_, err := tgAccountById(ctx, tgAccountId, tenantId)
	return err
}

func tgAccountById(ctx context.Context, tgAccountId int64, tenantId int64) (*publishTgAccount, error) {
	if tgAccountId <= 0 {
		return nil, gerror.New("请选择TG账号")
	}
	var item *publishTgAccount
	err := g.DB().Model("hg_youban_publish_tg_login").Safe().Ctx(ctx).
		Where("id", tgAccountId).
		Where("tenant_id", tenantId).
		Where("status", "authorized").
		Scan(&item)
	if err != nil {
		return nil, gerror.Wrap(err, "检查TG账号失败")
	}
	if item == nil || item.Id <= 0 {
		return nil, gerror.New("TG账号不存在或未登录")
	}
	if item.SessionKey == "" {
		return nil, gerror.New("TG账号会话不存在，请重新登录")
	}
	return item, nil
}

func publishTelegramConfig(ctx context.Context) (*publishmodel.TelegramConfig, error) {
	conf, err := publishservice.SysConfig().GetTelegram(ctx)
	if err != nil {
		return nil, err
	}
	if conf == nil {
		return &publishmodel.TelegramConfig{}, nil
	}
	return conf, nil
}
