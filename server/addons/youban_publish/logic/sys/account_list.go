package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
)

func (s *sSysPublish) accountList(ctx context.Context, in *sysin.AccountListInp) (list []*sysin.AccountModel, totalCount int, err error) {
	accountColumns := pdao.YoubanPublishAccount.Columns()
	base := pdao.YoubanPublishAccount.DB().Model(pdao.YoubanPublishAccount.Table()+" a").Safe().Ctx(ctx).
		LeftJoin(publishTenantTable+" m", "m.id=a.tenant_id").
		WhereNull("a." + accountColumns.DeletedAt)
	if in.TenantId > 0 {
		base = base.Where("a."+accountColumns.TenantId, in.TenantId)
	}
	if in.ManagerAccountId > 0 {
		base = base.Wheref("(a.%s = ? OR a.%s = ?)", accountColumns.ParentId, accountColumns.Id, in.ManagerAccountId, in.ManagerAccountId)
	}
	if in.AccountType != "" {
		base = base.Where("a."+accountColumns.AccountType, in.AccountType)
	}
	if in.Status > 0 {
		base = base.Where("a."+accountColumns.Status, in.Status)
	}
	if in.ExcludeCurrent == 1 {
		if userId := contexts.GetUserId(ctx); userId > 0 {
			base = base.WhereNot("a."+accountColumns.Id, userId)
		}
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		base = base.Where("(a.nickname LIKE ? OR a.username LIKE ?)", "%"+keyword+"%", "%"+keyword+"%")
	}
	totalCount, err = base.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取账号总数失败")
	}
	if err = base.Fields("a.*,m.name AS tenant_name").Page(in.Page, in.PerPage).OrderDesc("a." + accountColumns.Id).Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取账号列表失败")
	}
	if err = s.applyAccountOwnerNames(ctx, list); err != nil {
		return nil, 0, err
	}
	if err = s.applyAccountProfileCounts(ctx, list); err != nil {
		return nil, 0, err
	}
	return
}

func (s *sSysPublish) applyAccountOwnerNames(ctx context.Context, list []*sysin.AccountModel) error {
	tenantIds := make([]int64, 0, len(list))
	for _, item := range list {
		if item.TenantId > 0 {
			tenantIds = append(tenantIds, item.TenantId)
		}
	}
	names, err := s.tenantOwnerNames(ctx, tenantIds)
	if err != nil {
		return err
	}
	for _, item := range list {
		if name := names[item.TenantId]; name != "" {
			item.TenantName = name
		}
	}
	return nil
}

func (s *sSysPublish) applyAccountProfileCounts(ctx context.Context, list []*sysin.AccountModel) error {
	accountIds := make([]int64, 0, len(list))
	for _, item := range list {
		if item.Id > 0 {
			accountIds = append(accountIds, item.Id)
		}
	}
	if len(accountIds) == 0 {
		return nil
	}
	var rows []struct {
		AccountId  int64  `json:"accountId"`
		Visibility string `json:"visibility"`
		Count      int    `json:"count"`
	}
	err := dao.ContentProfile.DB().Model(dao.ContentProfile.Table()+" p").Safe().Ctx(ctx).
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		Fields("ps.account_id,p.visibility,COUNT(*) AS count").
		WhereIn("ps.account_id", accountIds).
		WhereNull("p.deleted_at").
		Group("ps.account_id,p.visibility").
		Scan(&rows)
	if err != nil {
		return gerror.Wrap(err, "获取账号资料数量失败")
	}
	counts := make(map[int64]map[string]int, len(rows))
	for _, row := range rows {
		if counts[row.AccountId] == nil {
			counts[row.AccountId] = make(map[string]int)
		}
		counts[row.AccountId][row.Visibility] = row.Count
	}
	for _, item := range list {
		item.UploadCount = counts[item.Id]["public"]
		item.DownCount = counts[item.Id]["private"]
	}
	return nil
}

func (s *sSysPublish) tenantOwnerNames(ctx context.Context, tenantIds []int64) (map[int64]string, error) {
	names := make(map[int64]string)
	if len(tenantIds) == 0 {
		return names, nil
	}
	accountColumns := pdao.YoubanPublishAccount.Columns()
	var accounts []struct {
		TenantId int64  `json:"tenantId"`
		Username string `json:"username"`
	}
	err := pdao.YoubanPublishAccount.Ctx(ctx).
		Fields(accountColumns.TenantId, accountColumns.Username).
		WhereIn(accountColumns.TenantId, tenantIds).
		Where(accountColumns.AccountType, sysin.PublishAccountTypeAdmin).
		WhereNull(accountColumns.DeletedAt).
		OrderAsc(accountColumns.Id).
		Scan(&accounts)
	if err != nil {
		return nil, gerror.Wrap(err, "获取账号归属失败")
	}
	for _, account := range accounts {
		if _, exists := names[account.TenantId]; !exists {
			names[account.TenantId] = account.Username
		}
	}
	return names, nil
}
