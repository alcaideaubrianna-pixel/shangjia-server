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
	"hotgo/internal/library/contexts"
)

func (s *sSysPublish) AdminTaskList(ctx context.Context, in *sysin.TaskListInp) (list []*sysin.TaskModel, totalCount int, err error) {
	return s.taskList(ctx, in)
}

func (s *sSysPublish) AdminTaskSave(ctx context.Context, in *sysin.TaskSaveInp) (id int64, err error) {
	return s.saveTask(ctx, in)
}

func (s *sSysPublish) AdminTaskSubmit(ctx context.Context, in *sysin.TaskSubmitInp) (err error) {
	return s.submitTask(ctx, in.Id, 0)
}

func (s *sSysPublish) AdminTaskCancel(ctx context.Context, in *sysin.TaskCancelInp) (err error) {
	return s.cancelTask(ctx, in.Id, 0)
}

func (s *sSysPublish) CurrentAccount(ctx context.Context) (res *sysin.CurrentAccountModel, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return &sysin.CurrentAccountModel{
		Id:          account.Id,
		TenantId:    account.TenantId,
		ParentId:    account.ParentId,
		AccountType: account.AccountType,
		Nickname:    account.Nickname,
		Username:    account.Username,
		Remark:      account.Remark,
		Status:      account.Status,
		CreatedAt:   account.CreatedAt,
		UpdatedAt:   account.UpdatedAt,
	}, nil
}

func (s *sSysPublish) MyTaskList(ctx context.Context, in *sysin.TaskListInp) (list []*sysin.TaskModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	in.TenantId = account.TenantId
	in.AccountId = account.Id
	return s.taskList(ctx, in)
}

func (s *sSysPublish) MyTaskSave(ctx context.Context, in *sysin.TaskSaveInp) (id int64, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return 0, err
	}
	in.TenantId = account.TenantId
	in.AccountId = account.Id
	return s.saveTask(ctx, in)
}

func (s *sSysPublish) MyTaskSubmit(ctx context.Context, in *sysin.TaskSubmitInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	return s.submitTask(ctx, in.Id, account.Id)
}

func (s *sSysPublish) MyTaskCancel(ctx context.Context, in *sysin.TaskCancelInp) (err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	return s.cancelTask(ctx, in.Id, account.Id)
}

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
			userId := contexts.GetUserId(ctx)
			if userId > 0 {
				mod = mod.WhereNot("a."+accountColumns.Id, userId)
			}
		}
		kw := strings.TrimSpace(in.Keyword)
		if kw != "" {
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
		kw := strings.TrimSpace(in.Keyword)
		if kw != "" {
			mod = mod.Where("(t.title LIKE ? OR t.client_request_id LIKE ?)", "%"+kw+"%", "%"+kw+"%")
		}
		return mod
	}
	base := s.taskBaseModel(ctx)
	base = applyFilters(base)
	totalCount, err = base.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取任务总数失败")
	}
	mod := s.taskListModel(ctx)
	mod = applyFilters(mod)
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
	return s.taskBaseModel(ctx).
		Fields("t.*,m.name AS tenant_name,a.nickname AS account_nickname,a.username AS account_username")
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

	taskColumns := pdao.YoubanPublishTask.Columns()
	var rows []struct {
		AccountId int64  `json:"accountId"`
		Count     int    `json:"count"`
		Status    string `json:"status"`
	}
	if err := pdao.YoubanPublishTask.Ctx(ctx).
		Fields(taskColumns.AccountId, taskColumns.Status, "COUNT(*) AS count").
		WhereIn(taskColumns.AccountId, accountIds).
		WhereIn(taskColumns.Status, []string{
			sysin.PublishTaskStatusCanceled,
			sysin.PublishTaskStatusPublished,
		}).
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

func (s *sSysPublish) saveTask(ctx context.Context, in *sysin.TaskSaveInp) (id int64, err error) {
	if err = in.Filter(ctx); err != nil {
		return 0, err
	}
	if in.TenantId <= 0 {
		return 0, gerror.New("租户ID不能为空")
	}
	if in.AccountId <= 0 {
		return 0, gerror.New("上架账号ID不能为空")
	}
	if err = s.ensureAccountBelongsTenant(ctx, in.AccountId, in.TenantId); err != nil {
		return 0, err
	}
	taskColumns := pdao.YoubanPublishTask.Columns()
	if in.Id == 0 && strings.TrimSpace(in.ClientRequestId) != "" {
		existing, findErr := pdao.YoubanPublishTask.Ctx(ctx).
			Where(taskColumns.TenantId, in.TenantId).
			Where(taskColumns.ClientRequestId, strings.TrimSpace(in.ClientRequestId)).
			WhereNull(taskColumns.DeletedAt).
			Fields(taskColumns.Id).
			Value()
		if findErr != nil {
			return 0, gerror.Wrap(findErr, "检查幂等请求失败")
		}
		if existing.Int64() > 0 {
			return existing.Int64(), nil
		}
	}
	data := g.Map{
		taskColumns.TenantId:        in.TenantId,
		taskColumns.MerchantId:      in.TenantId,
		taskColumns.AccountId:       in.AccountId,
		taskColumns.ClientRequestId: strings.TrimSpace(in.ClientRequestId),
		taskColumns.Title:           strings.TrimSpace(in.Title),
		taskColumns.Province:        strings.TrimSpace(in.Province),
		taskColumns.City:            strings.TrimSpace(in.City),
		taskColumns.PlainText:       strings.TrimSpace(in.PlainText),
		taskColumns.TgPushEnabled:   in.TgPushEnabled,
		taskColumns.UpdatedBy:       contexts.GetUserId(ctx),
		taskColumns.UpdatedAt:       gtime.Now(),
	}
	if in.Id > 0 {
		_, err = pdao.YoubanPublishTask.Ctx(ctx).Where(taskColumns.Id, in.Id).WhereNull(taskColumns.DeletedAt).Data(data).Update()
		id = in.Id
	} else {
		data[taskColumns.Status] = sysin.PublishTaskStatusDraft
		data[taskColumns.TgStatus] = "pending"
		data[taskColumns.MediaCount] = 0
		data[taskColumns.CreatedBy] = contexts.GetUserId(ctx)
		data[taskColumns.CreatedAt] = gtime.Now()
		id, err = pdao.YoubanPublishTask.Ctx(ctx).Data(data).InsertAndGetId()
	}
	if err != nil {
		return 0, gerror.Wrap(err, "保存上架任务失败")
	}
	return
}

func (s *sSysPublish) submitTask(ctx context.Context, id int64, accountId int64) (err error) {
	task, err := s.getTask(ctx, id, accountId)
	if err != nil {
		return err
	}
	if task["status"].String() == sysin.PublishTaskStatusCanceled {
		return gerror.New("已取消的任务不能提交")
	}
	_, err = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", id).Data(g.Map{
		"status":        sysin.PublishTaskStatusPending,
		"tg_status":     "pending",
		"error_message": "",
		"submitted_at":  gtime.Now(),
		"updated_by":    contexts.GetUserId(ctx),
		"updated_at":    gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "提交上架任务失败")
	}
	task, err = s.getTask(ctx, id, accountId)
	if err != nil {
		return err
	}
	if _, err = s.publishTaskToProfile(ctx, task); err != nil {
		_, _ = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", id).Data(g.Map{
			"status":        sysin.PublishTaskStatusFailed,
			"error_message": err.Error(),
			"updated_by":    contexts.GetUserId(ctx),
			"updated_at":    gtime.Now(),
		}).Update()
		return err
	}
	return s.ensureTgJob(ctx, id)
}

func (s *sSysPublish) cancelTask(ctx context.Context, id int64, accountId int64) (err error) {
	if _, err = s.getTask(ctx, id, accountId); err != nil {
		return err
	}
	_, err = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", id).Data(g.Map{
		"status":     sysin.PublishTaskStatusCanceled,
		"updated_by": contexts.GetUserId(ctx),
		"updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "取消上架任务失败")
	}
	return nil
}

func (s *sSysPublish) ensureTgJob(ctx context.Context, taskId int64) error {
	row, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", taskId).WhereNull("deleted_at").One()
	if err != nil {
		return gerror.Wrap(err, "读取上架任务失败")
	}
	if row.IsEmpty() || row["tg_push_enabled"].Int() != 1 {
		return nil
	}
	exists, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("task_id", taskId).Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG任务失败")
	}
	if exists > 0 {
		return nil
	}
	_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
		"task_id":     taskId,
		"tenant_id":   row["tenant_id"].Int64(),
		"merchant_id": row["tenant_id"].Int64(),
		"account_id":  row["account_id"].Int64(),
		"profile_id":  row["profile_id"].Int64(),
		"status":      "pending",
		"created_at":  gtime.Now(),
		"updated_at":  gtime.Now(),
	}).Insert()
	if err != nil {
		return gerror.Wrap(err, "创建TG发布任务失败")
	}
	return nil
}

func (s *sSysPublish) getTask(ctx context.Context, id int64, accountId int64) (gdb.Record, error) {
	if id <= 0 {
		return nil, gerror.New("任务ID不能为空")
	}
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", id).WhereNull("deleted_at")
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	row, err := mod.One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取上架任务失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("上架任务不存在")
	}
	return row, nil
}
