package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/contexts"
)

func (s *sSysPublish) accountList(ctx context.Context, in *sysin.AccountListInp) (list []*sysin.AccountModel, totalCount int, err error) {
	accountColumns := pdao.YoubanPublishAccount.Columns()
	base := pdao.YoubanPublishAccount.DB().Model(pdao.YoubanPublishAccount.Table()+" a").Safe().Ctx(ctx).
		LeftJoin(publishTenantTable+" m", "m.id=a.tenant_id").
		WhereNull("a." + accountColumns.DeletedAt)
	applyFilters := func(mod *gdb.Model) *gdb.Model {
		if in.TenantId > 0 {
			mod = mod.Where("a."+accountColumns.TenantId, in.TenantId)
		}
		if in.AccountType != "" {
			mod = mod.Where("a."+accountColumns.AccountType, in.AccountType)
		}
		if in.Status > 0 {
			mod = mod.Where("a."+accountColumns.Status, in.Status)
		}
		if in.ExcludeCurrent == 1 {
			if userId := contexts.GetUserId(ctx); userId > 0 {
				mod = mod.WhereNot("a."+accountColumns.Id, userId)
			}
		}
		if kw := strings.TrimSpace(in.Keyword); kw != "" {
			mod = mod.Where("(a.nickname LIKE ? OR a.username LIKE ?)", "%"+kw+"%", "%"+kw+"%")
		}
		return mod
	}
	base = applyFilters(base)
	totalCount, err = base.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取账号总数失败")
	}
	mod := base.Fields("a.*,m.name AS tenant_name")
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("a." + accountColumns.Id).Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取账号列表失败")
	}
	if err = s.applyAccountOwnerNames(ctx, list); err != nil {
		return nil, 0, err
	}
	if err = s.applyAccountTaskCounts(ctx, list); err != nil {
		return nil, 0, err
	}
	return
}

func (s *sSysPublish) taskList(ctx context.Context, in *sysin.TaskListInp) (list []*sysin.TaskModel, totalCount int, err error) {
	taskColumns := pdao.YoubanPublishTask.Columns()
	applyFilters := func(mod *gdb.Model) *gdb.Model {
		if in.TenantId > 0 {
			mod = mod.Where("t."+taskColumns.TenantId, in.TenantId)
		}
		if in.AccountId > 0 {
			mod = mod.Where("t."+taskColumns.AccountId, in.AccountId)
		}
		if in.Status != "" {
			mod = mod.Where("t."+taskColumns.Status, in.Status)
		}
		if kw := strings.TrimSpace(in.Keyword); kw != "" {
			mod = mod.Where("(t.title LIKE ? OR t.client_request_id LIKE ?)", "%"+kw+"%", "%"+kw+"%")
		}
		return mod
	}
	base := applyFilters(s.taskBaseModel(ctx))
	totalCount, err = base.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取任务总数失败")
	}
	mod := applyFilters(s.taskListModel(ctx))
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("t." + taskColumns.Id).Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取任务列表失败")
	}
	if err = s.applyTaskOwnerNames(ctx, list); err != nil {
		return nil, 0, err
	}
	return
}

func (s *sSysPublish) taskBaseModel(ctx context.Context) *gdb.Model {
	taskColumns := pdao.YoubanPublishTask.Columns()
	return pdao.YoubanPublishTask.DB().Model(pdao.YoubanPublishTask.Table()+" t").Safe().Ctx(ctx).
		LeftJoin(publishTenantTable+" m", "m.id=t.tenant_id").
		LeftJoin(publishAccountTable+" a", "a.id=t.account_id").
		WhereNull("t." + taskColumns.DeletedAt)
}

func (s *sSysPublish) taskListModel(ctx context.Context) *gdb.Model {
	return s.taskBaseModel(ctx).Fields("t.*,m.name AS tenant_name,a.nickname AS account_nickname,a.username AS account_username")
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

func (s *sSysPublish) applyAccountTaskCounts(ctx context.Context, list []*sysin.AccountModel) error {
	accountIds := make([]int64, 0, len(list))
	for _, item := range list {
		if item.Id > 0 {
			accountIds = append(accountIds, item.Id)
		}
	}
	if len(accountIds) == 0 {
		return nil
	}
	return s.fillAccountTaskCounts(ctx, list, accountIds)
}

func (s *sSysPublish) fillAccountTaskCounts(ctx context.Context, list []*sysin.AccountModel, accountIds []int64) error {
	taskColumns := pdao.YoubanPublishTask.Columns()
	var rows []struct {
		AccountId int64  `json:"accountId"`
		Count     int    `json:"count"`
		Status    string `json:"status"`
	}
	if err := pdao.YoubanPublishTask.Ctx(ctx).
		Fields(taskColumns.AccountId, taskColumns.Status, "COUNT(*) AS count").
		WhereIn(taskColumns.AccountId, accountIds).
		WhereIn(taskColumns.Status, []string{sysin.PublishTaskStatusCanceled, sysin.PublishTaskStatusPublished}).
		WhereNull(taskColumns.DeletedAt).
		Group(taskColumns.AccountId + "," + taskColumns.Status).
		Scan(&rows); err != nil {
		return gerror.Wrap(err, "获取账号资料数量失败")
	}
	counts := make(map[int64]map[string]int, len(rows))
	for _, row := range rows {
		if _, ok := counts[row.AccountId]; !ok {
			counts[row.AccountId] = make(map[string]int)
		}
		counts[row.AccountId][row.Status] = row.Count
	}
	for _, item := range list {
		item.UploadCount = counts[item.Id][sysin.PublishTaskStatusPublished]
		item.DownCount = counts[item.Id][sysin.PublishTaskStatusCanceled]
	}
	return nil
}

func (s *sSysPublish) applyTaskOwnerNames(ctx context.Context, list []*sysin.TaskModel) error {
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
		if _, ok := names[account.TenantId]; !ok {
			names[account.TenantId] = account.Username
		}
	}
	return names, nil
}
