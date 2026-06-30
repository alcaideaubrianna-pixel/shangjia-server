package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
)

func (s *sSysPublish) AdminTenantList(ctx context.Context, in *sysin.TenantListInp) (list []*sysin.TenantModel, totalCount int, err error) {
	tenantColumns := pdao.YoubanPublishTenant.Columns()
	accountColumns := pdao.YoubanPublishAccount.Columns()
	base := pdao.YoubanPublishTenant.DB().Model(pdao.YoubanPublishTenant.Table() + " t").Safe().Ctx(ctx).
		WhereNull("t." + tenantColumns.DeletedAt)
	applyFilters := func(mod *gdb.Model) *gdb.Model {
		if in.Status > 0 {
			mod = mod.Where("t."+tenantColumns.Status, in.Status)
		}
		kw := strings.TrimSpace(in.Keyword)
		if kw != "" {
			mod = mod.Where(
				"(t.remark LIKE ? OR EXISTS (SELECT 1 FROM "+publishAccountTable+" a WHERE a.tenant_id=t.id AND a.account_type=? AND a.deleted_at IS NULL AND a.username LIKE ?))",
				"%"+kw+"%",
				sysin.PublishAccountTypeAdmin,
				"%"+kw+"%",
			)
		}
		return mod
	}
	mod := applyFilters(base)
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取租户总数失败")
	}
	mod = mod.Fields("t.*")
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("t." + tenantColumns.Id).Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取租户列表失败")
	}
	if len(list) == 0 {
		return
	}
	tenantIds := make([]int64, 0, len(list))
	for _, item := range list {
		tenantIds = append(tenantIds, item.Id)
	}
	var accounts []struct {
		TenantId int64  `json:"tenantId"`
		Id       int64  `json:"id"`
		Username string `json:"username"`
	}
	if err = pdao.YoubanPublishAccount.Ctx(ctx).
		Fields(accountColumns.TenantId, accountColumns.Id, accountColumns.Username).
		WhereIn(accountColumns.TenantId, tenantIds).
		Where(accountColumns.AccountType, sysin.PublishAccountTypeAdmin).
		WhereNull(accountColumns.DeletedAt).
		OrderAsc(accountColumns.Id).
		Scan(&accounts); err != nil {
		return nil, 0, gerror.Wrap(err, "获取租户账号失败")
	}
	accountIdMap := make(map[int64]int64, len(accounts))
	accountNameMap := make(map[int64]string, len(accounts))
	for _, account := range accounts {
		if _, ok := accountNameMap[account.TenantId]; !ok {
			accountIdMap[account.TenantId] = account.Id
			accountNameMap[account.TenantId] = account.Username
		}
	}
	for _, item := range list {
		item.AdminAccountId = accountIdMap[item.Id]
		item.Username = accountNameMap[item.Id]
	}
	return
}

func (s *sSysPublish) AdminTenantSave(ctx context.Context, in *sysin.TenantSaveInp) (res *sysin.TenantSaveModel, err error) {
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	returnRes := &sysin.TenantSaveModel{Id: in.Id}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		tenantColumns := pdao.YoubanPublishTenant.Columns()
		data := g.Map{
			tenantColumns.Name:      in.Name,
			tenantColumns.Remark:    strings.TrimSpace(in.Remark),
			tenantColumns.Status:    in.Status,
			tenantColumns.UpdatedBy: contexts.GetUserId(ctx),
			tenantColumns.UpdatedAt: gtime.Now(),
		}
		if in.Id > 0 {
			if _, err = tx.Model(pdao.YoubanPublishTenant.Table()).Safe().Ctx(ctx).Where(tenantColumns.Id, in.Id).WhereNull(tenantColumns.DeletedAt).Data(data).Update(); err != nil {
				return gerror.Wrap(err, "保存租户失败")
			}
			return nil
		}
		data[tenantColumns.CreatedBy] = contexts.GetUserId(ctx)
		data[tenantColumns.CreatedAt] = gtime.Now()
		id, insertErr := tx.Model(pdao.YoubanPublishTenant.Table()).Safe().Ctx(ctx).Data(data).InsertAndGetId()
		if insertErr != nil {
			return gerror.Wrap(insertErr, "保存租户失败")
		}
		accountIn := &sysin.AccountSaveInp{
			TenantId:    id,
			AccountType: sysin.PublishAccountTypeAdmin,
			Username:    in.Username,
			Password:    in.Password,
			Nickname:    in.Username,
			Remark:      in.Remark,
			Status:      consts.StatusEnabled,
		}
		if err = accountIn.Filter(ctx); err != nil {
			return err
		}
		if err = s.savePublishAccount(ctx, tx, accountIn); err != nil {
			return err
		}
		returnRes.Id = id
		returnRes.Password = accountIn.Password
		return nil
	})
	if err != nil {
		return nil, err
	}
	return returnRes, nil
}

func (s *sSysPublish) AdminTenantDelete(ctx context.Context, in *sysin.TenantDeleteInp) (err error) {
	if len(in.Ids) == 0 {
		return gerror.New("请选择要删除的数据")
	}
	tenantColumns := pdao.YoubanPublishTenant.Columns()
	_, err = pdao.YoubanPublishTenant.Ctx(ctx).WhereIn(tenantColumns.Id, in.Ids).Data(g.Map{
		tenantColumns.DeletedBy: contexts.GetUserId(ctx),
		tenantColumns.DeletedAt: gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "删除租户失败")
	}
	return nil
}
