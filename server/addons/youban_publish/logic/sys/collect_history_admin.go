package sys

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) CollectSourceHistoryStart(ctx context.Context, in *sysin.CollectSourceHistoryStartInp) (*sysin.CollectHistoryTaskModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.Id <= 0 {
		return nil, gerror.New("采集源ID不能为空")
	}
	if err = s.ensureTenantVipFeature(ctx, account.TenantId, sysin.TenantVipFeatureCollectSource); err != nil {
		return nil, err
	}
	taskId, err := s.createCollectHistoryTask(ctx, in.Id, account.TenantId, account.Id, true)
	if err != nil {
		return nil, err
	}
	return s.collectHistoryTaskView(ctx, taskId, account.TenantId, account.Id)
}

func (s *sSysPublish) CollectHistoryTaskList(ctx context.Context, in *sysin.CollectHistoryTaskListInp) ([]*sysin.CollectHistoryTaskModel, int, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.CollectHistoryTaskListInp{}
	}
	mod := pdao.YoubanPublishCollectHistoryTask.DB().Model(pdao.YoubanPublishCollectHistoryTask.Table()+" t").Safe().Ctx(ctx).
		LeftJoin(pdao.YoubanPublishCollectSource.Table()+" s", "s.id=t.source_id").
		Where("t.tenant_id", account.TenantId).
		Where("t.account_id", account.Id)
	if in.SourceId > 0 {
		mod = mod.Where("t.source_id", in.SourceId)
	}
	if status := strings.TrimSpace(in.Status); status != "" {
		mod = mod.Where("t.status", status)
	}
	totalCount, err := mod.Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计历史采集任务失败")
	}
	var list []*sysin.CollectHistoryTaskModel
	fields := "t.*,s.title AS source_title,s.source_username AS source_username"
	err = mod.Fields(fields).Page(in.Page, in.PerPage).OrderDesc("t.id").Scan(&list)
	return list, totalCount, gerror.Wrap(err, "获取历史采集任务失败")
}

func (s *sSysPublish) CollectHistoryLogList(ctx context.Context, in *sysin.CollectHistoryLogListInp) ([]*sysin.CollectHistoryLogModel, int, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil || in.TaskId <= 0 {
		return nil, 0, gerror.New("历史采集任务ID不能为空")
	}
	task, err := s.collectHistoryTaskView(ctx, in.TaskId, account.TenantId, account.Id)
	if err != nil {
		return nil, 0, err
	}
	mod := pdao.YoubanPublishCollectHistoryLog.Ctx(ctx).
		Where("task_id", task.Id).
		Where("tenant_id", account.TenantId).
		Where("account_id", account.Id)
	totalCount, err := mod.Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "统计历史采集日志失败")
	}
	var list []*sysin.CollectHistoryLogModel
	err = mod.Page(in.Page, in.PerPage).OrderDesc("id").Scan(&list)
	return list, totalCount, gerror.Wrap(err, "获取历史采集日志失败")
}

func (s *sSysPublish) maybeCreateCollectHistoryTask(ctx context.Context, sourceId int64, tenantId int64, accountId int64) {
	taskId, err := s.createCollectHistoryTask(ctx, sourceId, tenantId, accountId, false)
	if err != nil {
		g.Log().Warningf(ctx, "创建历史采集任务失败 source:%d err:%+v", sourceId, err)
		return
	}
	if taskId > 0 {
		s.appendCollectHistoryLog(ctx, taskId, tenantId, accountId, "info", "created", "保存采集源后自动创建历史采集任务", nil)
	}
}

func (s *sSysPublish) collectHistoryTaskView(ctx context.Context, taskId int64, tenantId int64, accountId int64) (*sysin.CollectHistoryTaskModel, error) {
	var task *sysin.CollectHistoryTaskModel
	err := pdao.YoubanPublishCollectHistoryTask.Ctx(ctx).
		Where("id", taskId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Scan(&task)
	if err != nil {
		return nil, gerror.Wrap(err, "读取历史采集任务失败")
	}
	if task == nil || task.Id <= 0 {
		return nil, gerror.New("历史采集任务不存在")
	}
	return task, nil
}

func (s *sSysPublish) createCollectHistoryTask(ctx context.Context, sourceId int64, tenantId int64, accountId int64, manual bool) (int64, error) {
	source, err := s.collectHistorySource(ctx, sourceId, tenantId, accountId)
	if err != nil {
		return 0, err
	}
	if !manual && source.HistoryCollectEnabled != 1 {
		return 0, nil
	}
	existingId, err := s.activeCollectHistoryTaskId(ctx, sourceId, tenantId, accountId)
	if err != nil || existingId > 0 {
		if manual && existingId > 0 {
			return 0, gerror.New("历史消息已在采集，请稍后查看进度")
		}
		return existingId, err
	}
	now := gtime.Now()
	taskId, err := pdao.YoubanPublishCollectHistoryTask.Ctx(ctx).Data(g.Map{
		"tenant_id":      tenantId,
		"account_id":     accountId,
		"source_id":      source.Id,
		"tg_account_id":  source.TgAccountId,
		"source_chat_id": source.SourceChatId,
		"mode":           source.HistoryCollectMode,
		"days":           source.HistoryCollectDays,
		"status":         sysin.CollectHistoryTaskStatusPending,
		"created_at":     now,
		"updated_at":     now,
	}).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建历史采集任务失败")
	}
	if err = s.enqueueCollectHistoryTask(ctx, taskId, 0); err != nil {
		return taskId, gerror.Wrap(err, "投递历史采集任务失败")
	}
	s.appendCollectHistoryLog(ctx, taskId, tenantId, accountId, "info", "created", "历史采集任务已创建并投递队列", g.Map{
		"sourceId": source.Id,
		"mode":     source.HistoryCollectMode,
		"days":     source.HistoryCollectDays,
	})
	return taskId, nil
}

func (s *sSysPublish) activeCollectHistoryTaskId(ctx context.Context, sourceId int64, tenantId int64, accountId int64) (int64, error) {
	value, err := pdao.YoubanPublishCollectHistoryTask.Ctx(ctx).
		Where("source_id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("status", []string{
			sysin.CollectHistoryTaskStatusPending,
			sysin.CollectHistoryTaskStatusRunning,
			sysin.CollectHistoryTaskStatusPaused,
		}).
		Fields("id").
		OrderDesc("id").
		Limit(1).
		Value()
	if err != nil {
		return 0, gerror.Wrap(err, "读取历史采集任务失败")
	}
	return value.Int64(), nil
}

func (s *sSysPublish) collectHistorySource(ctx context.Context, sourceId int64, tenantId int64, accountId int64) (*sysin.CollectSourceModel, error) {
	var source *sysin.CollectSourceModel
	err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("source_type", sysin.CollectSourceTypeAccount).
		WhereNull("deleted_at").
		Scan(&source)
	if err != nil {
		return nil, gerror.Wrap(err, "读取账号采集源失败")
	}
	if source == nil || source.Id <= 0 {
		return nil, gerror.New("账号采集源不存在")
	}
	if source.TgAccountId <= 0 || strings.TrimSpace(source.SourceChatId) == "" {
		return nil, gerror.New("账号采集源配置不完整")
	}
	return source, nil
}

func (s *sSysPublish) appendCollectHistoryLog(ctx context.Context, taskId int64, tenantId int64, accountId int64, level string, stage string, message string, meta g.Map) {
	metaJSON := ""
	if meta != nil {
		data, _ := json.Marshal(meta)
		metaJSON = string(data)
	}
	_, _ = pdao.YoubanPublishCollectHistoryLog.Ctx(ctx).Data(g.Map{
		"task_id":    taskId,
		"tenant_id":  tenantId,
		"account_id": accountId,
		"level":      strings.TrimSpace(level),
		"stage":      strings.TrimSpace(stage),
		"message":    strings.TrimSpace(message),
		"meta_json":  metaJSON,
		"created_at": gtime.Now(),
	}).Insert()
}

func updateCollectHistoryTask(ctx context.Context, taskId int64, data g.Map) error {
	data["updated_at"] = gtime.Now()
	_, err := pdao.YoubanPublishCollectHistoryTask.Ctx(ctx).
		Where("id", taskId).
		WhereNot("status", sysin.CollectHistoryTaskStatusCanceled).
		Data(data).
		Update()
	return gerror.Wrap(err, "更新历史采集任务失败")
}

func updateCollectHistoryTaskTx(ctx context.Context, tx gdb.TX, taskId int64, data g.Map) error {
	data["updated_at"] = gtime.Now()
	_, err := tx.Model(pdao.YoubanPublishCollectHistoryTask.Table()).Ctx(ctx).
		Where("id", taskId).
		WhereNot("status", sysin.CollectHistoryTaskStatusCanceled).
		Data(data).
		Update()
	return gerror.Wrap(err, "更新历史采集任务失败")
}
