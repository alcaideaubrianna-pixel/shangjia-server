package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) AccountCapability(ctx context.Context, app string, accountId int64) (*sysin.AccountCapabilityModel, error) {
	app = strings.TrimSpace(app)
	// 管理员 Bot 使用独立的后台账号校验；这里仅为其保留基础能力标识，
	// 资料客服能力必须走 api 分支并验证上架账号状态及租户归属。
	if app == "admin" {
		if accountId <= 0 {
			return nil, gerror.New("后台账号信息不完整")
		}
		return &sysin.AccountCapabilityModel{AccountId: accountId, AccountType: sysin.PublishAccountTypeAdmin, TelegramBindingEnabled: 1}, nil
	}
	if app != "api" {
		return nil, gerror.Newf("不支持的账号应用类型：%s", app)
	}
	return s.activeAccountCapability(ctx, 0, accountId)
}

func (s *sSysPublish) activeAccountCapability(ctx context.Context, tenantId, accountId int64) (*sysin.AccountCapabilityModel, error) {
	if accountId <= 0 {
		return nil, gerror.New("上架账号信息不完整")
	}
	accountColumns := pdao.YoubanPublishAccount.Columns()
	mod := g.DB().Model(pdao.YoubanPublishAccount.Table()+" a").Safe().Ctx(ctx).
		LeftJoin(publishAccountSettingTable+" s", "s.account_id=a.id AND s.tenant_id=a.tenant_id AND s.deleted_at IS NULL").
		Where("a."+accountColumns.Id, accountId).
		Where("a."+accountColumns.Status, 1).
		WhereNull("a." + accountColumns.DeletedAt)
	if tenantId > 0 {
		mod = mod.Where("a."+accountColumns.TenantId, tenantId)
	}
	var capability *sysin.AccountCapabilityModel
	if err := mod.Fields("a.id AS account_id,a.tenant_id,a.account_type,COALESCE(s.shared_resource_enabled,0) AS shared_resource_enabled,COALESCE(s.telegram_binding_enabled,0) AS telegram_binding_enabled").Scan(&capability); err != nil {
		return nil, gerror.Wrap(err, "读取上架账号权限失败")
	}
	if capability == nil || capability.AccountId <= 0 || capability.TenantId <= 0 {
		return nil, gerror.New("绑定的上架账号不可用")
	}
	if capability.AccountType != sysin.PublishAccountTypeUploader {
		capability.SharedResourceEnabled = 0
		capability.TelegramBindingEnabled = 0
	}
	return capability, nil
}

func (s *sSysPublish) sharedProfileAccountIds(ctx context.Context, capability *sysin.AccountCapabilityModel) ([]int64, error) {
	if capability == nil || capability.AccountId <= 0 {
		return nil, gerror.New("上架账号信息不完整")
	}
	if capability.AccountType != sysin.PublishAccountTypeUploader || capability.SharedResourceEnabled != 1 {
		return []int64{capability.AccountId}, nil
	}
	accountColumns := pdao.YoubanPublishAccount.Columns()
	var rows []struct {
		Id int64 `json:"id"`
	}
	if err := pdao.YoubanPublishAccount.Ctx(ctx).Fields(accountColumns.Id).
		Where(accountColumns.TenantId, capability.TenantId).
		Where(accountColumns.Status, 1).
		WhereNull(accountColumns.DeletedAt).Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取租户共享账号失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.Id > 0 {
			ids = append(ids, row.Id)
		}
	}
	return uniqueIds(ids), nil
}

func sharedProfilePermission(capability *sysin.AccountCapabilityModel, profile *sysin.ProfileModel) string {
	if capability == nil || profile == nil {
		return sysin.ProfilePermissionVisitor
	}
	if profile.AccountId == capability.AccountId {
		return sysin.ProfilePermissionCreator
	}
	if capability.SharedResourceEnabled == 1 && profile.TenantId == capability.TenantId {
		return sysin.ProfilePermissionShared
	}
	return sysin.ProfilePermissionVisitor
}

func markSharedProfilePermission(item *sysin.ProfileModel, capability *sysin.AccountCapabilityModel) {
	permission := sharedProfilePermission(capability, item)
	markProfilePermission(item, permission)
	if item != nil && permission == sysin.ProfilePermissionShared {
		item.CanEdit = true
	}
}
