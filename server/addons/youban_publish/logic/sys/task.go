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
	return s.submitPublishWorkflow(ctx, id, 0, accountId, contexts.GetUserId(ctx))
}

func (s *sSysPublish) submitTaskByTenant(ctx context.Context, id int64, tenantId int64, operatorId int64) (err error) {
	return s.submitPublishWorkflow(ctx, id, tenantId, 0, operatorId)
}

func (s *sSysPublish) submitPublishWorkflow(ctx context.Context, id int64, tenantId int64, accountId int64, operatorId int64) (err error) {
	if err = ensureTelegramOperationColumns(ctx); err != nil {
		return err
	}
	task, err := s.getPublishWorkflowTask(ctx, id, tenantId, accountId)
	if err != nil {
		return err
	}
	if task["status"].String() == sysin.PublishTaskStatusPublishing {
		return nil
	}
	if !canSubmitPublishTask(task) {
		return gerror.New("已取消的任务不能提交")
	}
	hasChannels, err := s.hasPublishChannels(ctx, task)
	if err != nil {
		return err
	}
	if !hasChannels {
		return s.markTaskSavedWithoutPublish(ctx, task, operatorId)
	}
	operationNo := newTelegramOperationNo("publish", id)
	if err = s.markTaskPublishQueued(ctx, id, tenantId, operatorId, operationNo); err != nil {
		return err
	}
	if err = s.enqueuePublishSubmitTask(ctx, publishSubmitQueuePayload{
		TaskId:      id,
		TenantId:    tenantId,
		AccountId:   accountId,
		OperatorId:  operatorId,
		OperationNo: operationNo,
	}, 0); err != nil {
		_ = s.markTaskPublishFailed(ctx, id, tenantId, operatorId, err)
		return gerror.Wrap(err, "上架任务加入队列失败")
	}
	return nil
}

func (s *sSysPublish) markTaskPublishQueued(ctx context.Context, id int64, tenantId int64, operatorId int64, operationNo string) error {
	mod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", id).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	_, err := mod.
		Data(g.Map{
			"status":          sysin.PublishTaskStatusPublishing,
			"tg_status":       "pending",
			"tg_operation_no": operationNo,
			"tg_push_enabled": 1,
			"error_message":   "",
			"submitted_at":    gtime.Now(),
			"updated_by":      operatorId,
			"updated_at":      gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "提交上架任务失败")
	}
	return nil
}

func (s *sSysPublish) markTaskPublishFailed(ctx context.Context, id int64, tenantId int64, operatorId int64, cause error) error {
	failMod := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", id).
		WhereNull("deleted_at")
	if tenantId > 0 {
		failMod = failMod.Where("tenant_id", tenantId)
	}
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	_, err := failMod.Data(g.Map{
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

func (s *sSysPublish) executePublishSubmitWorkflow(ctx context.Context, payload publishSubmitQueuePayload) error {
	if err := ensureTelegramOperationColumns(ctx); err != nil {
		return err
	}
	task, err := s.getPublishWorkflowTask(ctx, payload.TaskId, payload.TenantId, payload.AccountId)
	if err != nil {
		return err
	}
	if task["status"].String() != sysin.PublishTaskStatusPublishing {
		return nil
	}
	if payload.OperationNo != "" && task["tg_operation_no"].String() != payload.OperationNo {
		return nil
	}
	if _, err = s.publishTaskToProfile(ctx, task); err != nil {
		_ = s.markTaskPublishFailed(ctx, payload.TaskId, payload.TenantId, payload.OperatorId, err)
		return err
	}
	if err = s.ensureTgJob(ctx, payload.TaskId, payload.OperationNo, payload.ChannelIds, payload.OnlySelectedChannels); err != nil {
		_ = s.markTaskPublishFailed(ctx, payload.TaskId, payload.TenantId, payload.OperatorId, err)
		return err
	}
	return nil
}

func (s *sSysPublish) getPublishWorkflowTask(ctx context.Context, id int64, tenantId int64, accountId int64) (gdb.Record, error) {
	if accountId > 0 {
		return s.getTask(ctx, id, accountId)
	}
	return s.getTaskByTenant(ctx, id, tenantId)
}

func (s *sSysPublish) markTaskSavedWithoutPublish(ctx context.Context, task gdb.Record, operatorId int64) error {
	if task.IsEmpty() {
		return nil
	}
	_, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", task["id"].Int64()).
		WhereNull("deleted_at").
		Data(g.Map{
			"status":          sysin.PublishTaskStatusDraft,
			"tg_status":       "skipped",
			"tg_operation_no": "",
			"error_message":   "",
			"updated_by":      operatorId,
			"updated_at":      gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "保存未上架任务失败")
	}
	profileId := task["profile_id"].Int64()
	if profileId <= 0 {
		return nil
	}
	_, err = s.syncProfilePublishState(ctx, profileId, 2, consts.ContentVisibilityPrivate, nil)
	if err != nil {
		return gerror.Wrap(err, "同步未上架资料状态失败")
	}
	return nil
}

func (s *sSysPublish) hasPublishChannels(ctx context.Context, task gdb.Record) (bool, error) {
	channelIds := decodeInt64JSON(task["channel_id_json"].String())
	if len(channelIds) > 0 {
		migratedIds, err := s.migratePublishTaskChannelIds(ctx, task, channelIds)
		if err != nil {
			return false, err
		}
		if len(migratedIds) == 0 {
			return false, gerror.New("所选上架频道已失效，请重新选择频道")
		}
		channelIds = migratedIds
	}
	mod := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("tenant_id", task["tenant_id"].Int64()).
		Where("publish_direction", "up").
		Where("status", 1).
		WhereNull("deleted_at")
	if len(channelIds) > 0 {
		mod = mod.WhereIn("id", channelIds)
	} else {
		mod = mod.Where("is_default_selected", 1)
	}
	count, err := mod.Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查上架频道失败")
	}
	return count > 0, nil
}

func (s *sSysPublish) migratePublishTaskChannelIds(ctx context.Context, task gdb.Record, channelIds []int64) ([]int64, error) {
	var oldChannels []struct {
		TargetChatId string `json:"targetChatId"`
	}
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Unscoped().
		Fields("target_chat_id").
		Where("tenant_id", task["tenant_id"].Int64()).
		WhereIn("id", channelIds).
		Scan(&oldChannels); err != nil {
		return nil, gerror.Wrap(err, "读取历史上架频道失败")
	}
	targetChatIds := make([]string, 0, len(oldChannels))
	for _, channel := range oldChannels {
		if targetChatId := strings.TrimSpace(channel.TargetChatId); targetChatId != "" {
			targetChatIds = append(targetChatIds, targetChatId)
		}
	}
	if len(targetChatIds) == 0 {
		return nil, nil
	}
	var currentChannels []struct {
		Id int64 `json:"id"`
	}
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id").
		Where("tenant_id", task["tenant_id"].Int64()).
		Where("publish_direction", "up").
		Where("status", 1).
		WhereNull("deleted_at").
		WhereIn("target_chat_id", targetChatIds).
		OrderAsc("id").
		Scan(&currentChannels); err != nil {
		return nil, gerror.Wrap(err, "匹配当前上架频道失败")
	}
	migratedIds := make([]int64, 0, len(currentChannels))
	for _, channel := range currentChannels {
		if channel.Id > 0 {
			migratedIds = append(migratedIds, channel.Id)
		}
	}
	migratedIds = uniqueIds(migratedIds)
	if len(migratedIds) == 0 {
		return nil, nil
	}
	channelJSON, err := encodeBotIds(migratedIds)
	if err != nil {
		return nil, gerror.Wrap(err, "编码当前上架频道失败")
	}
	_, err = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", task["id"].Int64()).
		Where("tenant_id", task["tenant_id"].Int64()).
		WhereNull("deleted_at").
		Data(g.Map{"channel_id_json": channelJSON, "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return nil, gerror.Wrap(err, "更新任务上架频道失败")
	}
	return migratedIds, nil
}

func canSubmitPublishTask(task gdb.Record) bool {
	if task["status"].String() != sysin.PublishTaskStatusCanceled {
		return true
	}
	return task["profile_id"].Int64() > 0
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

func (s *sSysPublish) ensureTgJob(ctx context.Context, taskId int64, operationNo string, channelIds []int64, onlySelectedChannels bool) error {
	if len(channelIds) == 0 {
		return s.ensureTgJobs(ctx, taskId, operationNo)
	}
	return s.submitTelegramPublish(ctx, telegramPublishRequest{
		TaskId:               taskId,
		OperationNo:          operationNo,
		OperationPrefix:      telegramPublishBizProfile,
		ChannelIds:           channelIds,
		OnlySelectedChannels: onlySelectedChannels,
	})
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
