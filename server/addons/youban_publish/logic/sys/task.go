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
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.TaskListInp{}
	}
	in.TenantId = account.TenantId
	return s.taskList(ctx, in)
}

func (s *sSysPublish) ServerTaskList(ctx context.Context, in *sysin.TaskListInp) (list []*sysin.TaskModel, totalCount int, err error) {
	if in == nil {
		in = &sysin.TaskListInp{}
	}
	return s.taskList(ctx, in)
}

func (s *sSysPublish) ServerTaskSave(ctx context.Context, in *sysin.TaskSaveInp) (id int64, err error) {
	if in == nil {
		return 0, gerror.New("任务信息不能为空")
	}
	return s.saveTask(ctx, in)
}

func (s *sSysPublish) ServerTaskSubmit(ctx context.Context, in *sysin.TaskSubmitInp) (err error) {
	if in == nil || in.Id <= 0 {
		return gerror.New("任务ID不能为空")
	}
	return s.submitTaskByTenant(ctx, in.Id, 0, contexts.GetUserId(ctx))
}

func (s *sSysPublish) ServerTaskCancel(ctx context.Context, in *sysin.TaskCancelInp) (err error) {
	if in == nil || in.Id <= 0 {
		return gerror.New("任务ID不能为空")
	}
	return s.cancelTaskByTenant(ctx, in.Id, 0, contexts.GetUserId(ctx))
}

func (s *sSysPublish) AdminTaskSave(ctx context.Context, in *sysin.TaskSaveInp) (id int64, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return 0, err
	}
	if in == nil {
		return 0, gerror.New("任务信息不能为空")
	}
	in.TenantId = account.TenantId
	return s.saveTask(ctx, in)
}

func (s *sSysPublish) AdminTaskSubmit(ctx context.Context, in *sysin.TaskSubmitInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.submitTaskByTenant(ctx, in.Id, account.TenantId, account.Id)
}

func (s *sSysPublish) AdminTaskCancel(ctx context.Context, in *sysin.TaskCancelInp) (err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	return s.cancelTaskByTenant(ctx, in.Id, account.TenantId, account.Id)
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

func (s *sSysPublish) saveTask(ctx context.Context, in *sysin.TaskSaveInp) (id int64, err error) {
	if in == nil {
		return 0, gerror.New("任务信息不能为空")
	}
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
		if _, err = s.getTaskByTenant(ctx, in.Id, in.TenantId); err != nil {
			return 0, err
		}
		_, err = pdao.YoubanPublishTask.Ctx(ctx).
			Where(taskColumns.Id, in.Id).
			Where(taskColumns.TenantId, in.TenantId).
			WhereNull(taskColumns.DeletedAt).
			Data(data).
			Update()
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
	_, err = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", id).
		Where("tenant_id", task["tenant_id"].Int64()).
		WhereNull("deleted_at").
		Data(g.Map{
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
		_, _ = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
			Where("id", id).
			Where("tenant_id", task["tenant_id"].Int64()).
			WhereNull("deleted_at").
			Data(g.Map{
				"status":        sysin.PublishTaskStatusFailed,
				"error_message": err.Error(),
				"updated_by":    contexts.GetUserId(ctx),
				"updated_at":    gtime.Now(),
			}).Update()
		return err
	}
	return s.ensureTgJob(ctx, id)
}

func (s *sSysPublish) submitTaskByTenant(ctx context.Context, id int64, tenantId int64, operatorId int64) (err error) {
	task, err := s.getTaskByTenant(ctx, id, tenantId)
	if err != nil {
		return err
	}
	if task["status"].String() == sysin.PublishTaskStatusCanceled {
		return gerror.New("已取消的任务不能提交")
	}
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", id).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	_, err = mod.
		Data(g.Map{
			"status":        sysin.PublishTaskStatusPending,
			"tg_status":     "pending",
			"error_message": "",
			"submitted_at":  gtime.Now(),
			"updated_by":    operatorId,
			"updated_at":    gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "提交上架任务失败")
	}
	task, err = s.getTaskByTenant(ctx, id, tenantId)
	if err != nil {
		return err
	}
	if _, err = s.publishTaskToProfile(ctx, task); err != nil {
		failMod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
			Where("id", id).
			WhereNull("deleted_at")
		if tenantId > 0 {
			failMod = failMod.Where("tenant_id", tenantId)
		}
		_, _ = failMod.Data(g.Map{
			"status":        sysin.PublishTaskStatusFailed,
			"error_message": err.Error(),
			"updated_by":    operatorId,
			"updated_at":    gtime.Now(),
		}).Update()
		return err
	}
	return s.ensureTgJob(ctx, id)
}

func (s *sSysPublish) cancelTask(ctx context.Context, id int64, accountId int64) (err error) {
	task, err := s.getTask(ctx, id, accountId)
	if err != nil {
		return err
	}
	_, err = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", id).
		Where("tenant_id", task["tenant_id"].Int64()).
		WhereNull("deleted_at").
		Data(g.Map{
			"status":     sysin.PublishTaskStatusCanceled,
			"updated_by": contexts.GetUserId(ctx),
			"updated_at": gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "取消上架任务失败")
	}
	return nil
}

func (s *sSysPublish) cancelTaskByTenant(ctx context.Context, id int64, tenantId int64, operatorId int64) (err error) {
	if _, err = s.getTaskByTenant(ctx, id, tenantId); err != nil {
		return err
	}
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", id).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	_, err = mod.Data(g.Map{
		"status":     sysin.PublishTaskStatusCanceled,
		"updated_by": operatorId,
		"updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "取消上架任务失败")
	}
	return nil
}

func (s *sSysPublish) ensureTgJob(ctx context.Context, taskId int64) error {
	return s.ensureTgJobs(ctx, taskId)
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

func (s *sSysPublish) getTaskByTenant(ctx context.Context, id int64, tenantId int64) (gdb.Record, error) {
	if id <= 0 {
		return nil, gerror.New("任务ID不能为空")
	}
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", id).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
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
