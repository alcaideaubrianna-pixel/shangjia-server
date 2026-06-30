package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/contexts"
)

func (s *sSysPublish) currentAccount(ctx context.Context) (*sysin.AccountModel, error) {
	userId := contexts.GetUserId(ctx)
	if userId <= 0 {
		return nil, gerror.New("请先登录")
	}
	accountColumns := pdao.YoubanPublishAccount.Columns()
	tenantColumns := pdao.YoubanPublishTenant.Columns()
	var account *sysin.AccountModel
	err := pdao.YoubanPublishAccount.DB().Model(pdao.YoubanPublishAccount.Table()+" a").Safe().Ctx(ctx).
		LeftJoin(pdao.YoubanPublishTenant.Table()+" m", "m.id=a.tenant_id").
		Where("a."+accountColumns.Id, userId).
		Where("a."+accountColumns.Status, 1).
		WhereNull("a." + accountColumns.DeletedAt).
		WhereNull("m." + tenantColumns.DeletedAt).
		Fields("a.*,m.name AS tenant_name").
		OrderAsc("a." + accountColumns.Id).
		Scan(&account)
	if err != nil {
		return nil, gerror.Wrap(err, "读取当前上架账号失败")
	}
	if account == nil || account.Id <= 0 {
		return nil, gerror.New("当前用户未绑定上架账号")
	}
	return account, nil
}

func (s *sSysPublish) ensureTenant(ctx context.Context, tenantId int64) error {
	tenantColumns := pdao.YoubanPublishTenant.Columns()
	count, err := pdao.YoubanPublishTenant.Ctx(ctx).Where(tenantColumns.Id, tenantId).WhereNull(tenantColumns.DeletedAt).Count()
	if err != nil {
		return gerror.Wrap(err, "检查租户失败")
	}
	if count == 0 {
		return gerror.New("租户不存在")
	}
	return nil
}

func (s *sSysPublish) ensureAccountBelongsTenant(ctx context.Context, accountId int64, tenantId int64) error {
	accountColumns := pdao.YoubanPublishAccount.Columns()
	count, err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(accountColumns.Id, accountId).
		Where(accountColumns.TenantId, tenantId).
		WhereNull(accountColumns.DeletedAt).
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查上架账号失败")
	}
	if count == 0 {
		return gerror.New("上架账号不存在或不属于该租户")
	}
	return nil
}

func (s *sSysPublish) ensureEditableAccount(ctx context.Context, accountId int64, tenantId int64) error {
	if accountId <= 0 {
		return nil
	}
	accountColumns := pdao.YoubanPublishAccount.Columns()
	count, err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(accountColumns.Id, accountId).
		Where(accountColumns.TenantId, tenantId).
		WhereNull(accountColumns.DeletedAt).
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查账号权限失败")
	}
	if count == 0 {
		return gerror.New("账号不存在或无权操作")
	}
	return nil
}
