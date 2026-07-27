package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) CurrentAccount(ctx context.Context) (*sysin.CurrentAccountModel, error) {
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

func (s *sSysPublish) getPublishWorkflowTask(ctx context.Context, id int64, tenantId int64, accountId int64) (gdb.Record, error) {
	if accountId > 0 {
		return s.getTask(ctx, id, accountId)
	}
	return s.getTaskByTenant(ctx, id, tenantId)
}

func (s *sSysPublish) markTaskPublishQueued(ctx context.Context, id int64, tenantId int64, operatorId int64, operationNo string) error {
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", id).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	_, err := mod.Data(g.Map{
		"status":          sysin.PublishTaskStatusPublishing,
		"tg_status":       "pending",
		"tg_operation_no": operationNo,
		"tg_push_enabled": 1,
		"error_message":   "",
		"submitted_at":    gtime.Now(),
		"updated_by":      operatorId,
		"updated_at":      gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "提交采集发布任务失败")
	}
	return nil
}

func (s *sSysPublish) markTaskPublishFailed(ctx context.Context, id int64, tenantId int64, operatorId int64, cause error) error {
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", id).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	_, err := mod.Data(g.Map{
		"status":        sysin.PublishTaskStatusFailed,
		"error_message": message,
		"updated_by":    operatorId,
		"updated_at":    gtime.Now(),
	}).Update()
	if err == nil {
		s.appendPublishTaskFailureLog(ctx, id, tenantId, message)
	}
	return err
}

func (s *sSysPublish) appendPublishTaskFailureLog(ctx context.Context, taskId int64, tenantId int64, message string) {
	if taskId <= 0 {
		return
	}
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", taskId).WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	task, err := mod.Fields("id,tenant_id,account_id,profile_id").One()
	if err != nil || task.IsEmpty() {
		return
	}
	s.appendTelegramJobLog(ctx, telegramJobRecord{
		TaskId:    task["id"].Int64(),
		TenantId:  task["tenant_id"].Int64(),
		AccountId: task["account_id"].Int64(),
		ProfileId: task["profile_id"].Int64(),
	}, "publish", "failed", message)
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
		return nil, gerror.Wrap(err, "读取采集任务失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("采集任务不存在")
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
		return nil, gerror.Wrap(err, "读取采集任务失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("采集任务不存在")
	}
	return row, nil
}
