package sys

import (
	"context"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) ensureCollectTgJobs(ctx context.Context, taskId int64) error {
	if taskId <= 0 {
		return nil
	}
	task, err := s.getPublishWorkflowTask(ctx, taskId, 0, 0)
	if err != nil {
		return err
	}
	operationNo := task["tg_operation_no"].String()
	if operationNo == "" {
		operationNo = newTelegramOperationNo(telegramPublishBizCollect, taskId)
	}
	if err = s.markTaskPublishQueued(ctx, taskId, task["tenant_id"].Int64(), task["account_id"].Int64(), operationNo); err != nil {
		return err
	}
	if err = s.enqueuePublishSubmitTask(ctx, publishSubmitQueuePayload{
		TaskId:      taskId,
		TenantId:    task["tenant_id"].Int64(),
		AccountId:   task["account_id"].Int64(),
		OperatorId:  task["account_id"].Int64(),
		OperationNo: operationNo,
	}, 0); err != nil {
		_ = s.markTaskPublishFailed(ctx, taskId, task["tenant_id"].Int64(), task["account_id"].Int64(), err)
		return gerror.Wrap(err, "采集上架任务加入队列失败")
	}
	return nil
}

func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (s *sSysPublish) markCollectDispatchQueued(ctx context.Context, dispatchId int64, taskId int64) error {
	if dispatchId <= 0 || taskId <= 0 {
		return nil
	}
	_, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("id", dispatchId).
		Data(g.Map{
			"task_id":       taskId,
			"status":        sysin.CollectDispatchStatusPending,
			"error_message": "",
			"updated_at":    gtime.Now(),
		}).
		Update()
	return gerror.Wrap(err, "更新采集分发排队状态失败")
}

func (s *sSysPublish) markCollectDispatchSentByTask(ctx context.Context, taskId int64) error {
	if taskId <= 0 {
		return nil
	}
	_, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("task_id", taskId).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing}).
		Data(g.Map{
			"status":        sysin.CollectDispatchStatusSent,
			"error_message": "",
			"finished_at":   gtime.Now(),
			"updated_at":    gtime.Now(),
		}).
		Update()
	return gerror.Wrap(err, "更新采集分发发送状态失败")
}

func (s *sSysPublish) markCollectDispatchFailedByTask(ctx context.Context, taskId int64, message string) error {
	if taskId <= 0 {
		return nil
	}
	_, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("task_id", taskId).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing}).
		Data(g.Map{
			"status":        sysin.CollectDispatchStatusFailed,
			"error_message": message,
			"finished_at":   gtime.Now(),
			"updated_at":    gtime.Now(),
		}).
		Update()
	return gerror.Wrap(err, "更新采集分发失败状态失败")
}

func (s *sSysPublish) markCollectDispatchFailed(ctx context.Context, dispatchId int64, message string) error {
	if dispatchId <= 0 {
		return nil
	}
	_, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("id", dispatchId).
		Data(g.Map{
			"status":        sysin.CollectDispatchStatusFailed,
			"error_message": message,
			"finished_at":   gtime.Now(),
			"updated_at":    gtime.Now(),
		}).
		Update()
	return gerror.Wrap(err, "更新采集分发失败状态失败")
}
