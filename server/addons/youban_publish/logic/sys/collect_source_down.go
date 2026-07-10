package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const collectSourceDownMessage = "采集源一键下架"

func (s *sSysPublish) CollectSourceDown(ctx context.Context, in *sysin.CollectSourceDownInp) (*sysin.CollectSourceDownModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.Id <= 0 {
		return nil, gerror.New("采集源ID不能为空")
	}
	source, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", in.Id).
		Where("tenant_id", account.TenantId).
		Where("account_id", account.Id).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集源失败")
	}
	if source.IsEmpty() {
		return nil, gerror.New("采集源不存在")
	}
	now := gtime.Now()
	_, err = pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", in.Id).
		Data(g.Map{"collect_enabled": 0, "updated_by": account.Id, "updated_at": now}).
		Update()
	if err != nil {
		return nil, gerror.Wrap(err, "停止采集源失败")
	}
	taskIds, err := s.collectSourceTaskIds(ctx, in.Id, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	res, err := s.collectSourceDownPreview(ctx, in.Id, account.TenantId, account.Id, taskIds)
	if err != nil {
		return nil, err
	}
	if len(taskIds) == 0 {
		return res, nil
	}
	if err = s.supersedeCollectSourcePendingJobs(ctx, taskIds, account.TenantId); err != nil {
		return nil, err
	}
	if err = s.enqueueCollectSourceDown(ctx, collectSourceDownQueuePayload{
		AccountId: account.Id,
		SourceId:  in.Id,
		TenantId:  account.TenantId,
	}, 0); err != nil {
		return nil, gerror.Wrap(err, "投递采集源下架任务失败")
	}
	res.Queued = 1
	return res, nil
}

func (s *sSysPublish) ExecuteCollectSourceDown(ctx context.Context, sourceId int64, tenantId int64, accountId int64) (*sysin.CollectSourceDownModel, error) {
	taskIds, err := s.collectSourceTaskIds(ctx, sourceId, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	res, err := s.collectSourceDownPreview(ctx, sourceId, tenantId, accountId, taskIds)
	if err != nil {
		return nil, err
	}
	if len(taskIds) == 0 {
		return res, nil
	}
	jobs, err := s.collectSourceDownJobs(ctx, taskIds, tenantId)
	if err != nil {
		return nil, err
	}
	for _, job := range jobs {
		if job.Status == "sent" || res.MessageCount > 0 {
			if err = s.deleteTelegramMessageSet(ctx, job.telegramJobRecord(), collectSourceDownMessage); err != nil {
				return nil, err
			}
		}
		if err = s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			return nil, err
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "down", "deleted", "采集源一键下架，目标频道消息已删除或任务已废弃")
	}
	if err = s.finishCollectSourceDownTasks(ctx, taskIds, tenantId); err != nil {
		return nil, err
	}
	if err = s.finishCollectSourceDownDispatch(ctx, sourceId, taskIds, tenantId, accountId); err != nil {
		return nil, err
	}
	return res, nil
}

func (s *sSysPublish) collectSourceTaskIds(ctx context.Context, sourceId int64, tenantId int64, accountId int64) ([]int64, error) {
	var rows []struct {
		TaskId int64 `json:"taskId"`
	}
	err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields("DISTINCT task_id").
		Where("source_id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereGT("task_id", 0).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集源任务失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.TaskId)
	}
	return uniqueIds(ids), nil
}

func (s *sSysPublish) collectSourceDownPreview(ctx context.Context, sourceId int64, tenantId int64, accountId int64, taskIds []int64) (*sysin.CollectSourceDownModel, error) {
	res := &sysin.CollectSourceDownModel{SourceId: sourceId, TaskCount: len(taskIds)}
	if len(taskIds) == 0 {
		return res, nil
	}
	jobCount, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("task_id", taskIds).
		WhereIn("status", collectSourceDownJobStatuses()).
		Count()
	if err != nil {
		return nil, gerror.Wrap(err, "统计采集源TG任务失败")
	}
	messageCount, err := g.DB().Model(publishTgMessageTable+" m").Safe().Ctx(ctx).
		LeftJoin(publishTgJobTable+" j", "j.id=m.job_id").
		Where("j.tenant_id", tenantId).
		Where("j.account_id", accountId).
		WhereIn("j.task_id", taskIds).
		Where("m.status", "sent").
		Count()
	if err != nil {
		return nil, gerror.Wrap(err, "统计采集源TG消息失败")
	}
	res.JobCount = jobCount
	res.MessageCount = messageCount
	return res, nil
}

func (s *sSysPublish) collectSourceDownJobs(ctx context.Context, taskIds []int64, tenantId int64) ([]telegramResubmitJob, error) {
	var jobs []telegramResubmitJob
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("task_id", taskIds).
		WhereIn("status", collectSourceDownJobStatuses()).
		OrderAsc("id").
		Scan(&jobs)
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集源TG任务失败")
	}
	return jobs, nil
}

func (s *sSysPublish) supersedeCollectSourcePendingJobs(ctx context.Context, taskIds []int64, tenantId int64) error {
	if len(taskIds) == 0 {
		return nil
	}
	jobs, err := s.collectSourcePendingDownJobs(ctx, taskIds, tenantId)
	if err != nil {
		return err
	}
	if len(jobs) == 0 {
		return nil
	}
	jobIds := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		jobIds = append(jobIds, job.Id)
	}
	_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("id", uniqueIds(jobIds)).
		Data(g.Map{
			"status":              "superseded",
			"dispatch_status":     tgDispatchStatusDone,
			"next_retry_at":       nil,
			"next_cycle_at":       nil,
			"error_message":       collectSourceDownMessage,
			"last_dispatch_error": collectSourceDownMessage,
			"updated_at":          gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "废弃采集源待发送任务失败")
	}
	for _, job := range jobs {
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "down", "superseded", "采集源一键下架，待发送任务已废弃")
	}
	return nil
}

func (s *sSysPublish) collectSourcePendingDownJobs(ctx context.Context, taskIds []int64, tenantId int64) ([]telegramResubmitJob, error) {
	var jobs []telegramResubmitJob
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("task_id", taskIds).
		WhereIn("status", []string{"pending", "failed_retry", "sending"}).
		OrderAsc("id").
		Scan(&jobs)
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集源待发送任务失败")
	}
	return jobs, nil
}

func (s *sSysPublish) finishCollectSourceDownTasks(ctx context.Context, taskIds []int64, tenantId int64) error {
	_, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("id", taskIds).
		Data(g.Map{
			"status":        sysin.PublishTaskStatusCanceled,
			"tg_status":     sysin.PublishTaskStatusCanceled,
			"error_message": collectSourceDownMessage,
			"updated_at":    gtime.Now(),
		}).
		Update()
	return gerror.Wrap(err, "更新采集源任务下架状态失败")
}

func (s *sSysPublish) finishCollectSourceDownDispatch(ctx context.Context, sourceId int64, taskIds []int64, tenantId int64, accountId int64) error {
	_, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("source_id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("task_id", taskIds).
		Data(g.Map{
			"status":        sysin.CollectDispatchStatusSkipped,
			"error_message": collectSourceDownMessage,
			"finished_at":   gtime.Now(),
			"updated_at":    gtime.Now(),
		}).
		Update()
	return gerror.Wrap(err, "更新采集分发下架状态失败")
}

func collectSourceDownJobStatuses() []string {
	return []string{"pending", "sending", "failed_retry", "failed", "sent", "superseded"}
}
