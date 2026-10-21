package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
)

// ServerAccountLoginLink issues a normal account session. The caller must be a
// system administrator, not an AppApi tenant administrator with the same ID.
func (s *sSysPublish) ServerAccountLoginLink(ctx context.Context, in *sysin.AccountLoginLinkInp) (*sysin.AccountLoginModel, error) {
	user := contexts.GetUser(ctx)
	if user == nil || user.App != consts.AppAdmin {
		return nil, gerror.New("仅系统超级管理员可以生成登录链接")
	}
	if err := s.requireSystemSuperAdmin(ctx); err != nil {
		return nil, err
	}
	if err := in.Filter(ctx); err != nil {
		return nil, err
	}
	account, err := s.publishAccountById(ctx, in.Id)
	if err != nil {
		return nil, err
	}
	if account.Status != consts.StatusEnabled {
		return nil, gerror.New("账号已被停用")
	}
	var tenant *sysin.TenantModel
	cols := pdao.YoubanPublishTenant.Columns()
	if err = pdao.YoubanPublishTenant.Ctx(ctx).Where(cols.Id, account.TenantId).WhereNull(cols.DeletedAt).Scan(&tenant); err != nil {
		return nil, gerror.Wrap(err, "读取账号归属失败")
	}
	if tenant == nil || tenant.Status != consts.StatusEnabled {
		return nil, gerror.New("账号归属不存在或已被停用")
	}
	res, err := s.loginPublishAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	g.Log().Info(ctx, "生成上架账号登录链接", g.Map{"operatorId": user.Id, "accountId": account.Id, "tenantId": account.TenantId})
	return res, nil
}
