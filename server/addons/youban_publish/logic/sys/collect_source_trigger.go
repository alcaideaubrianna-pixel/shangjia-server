package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) CollectSourceTrigger(ctx context.Context, in *sysin.CollectSourceTriggerInp) (*sysin.CollectSourceTriggerModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.Id <= 0 {
		return nil, gerror.New("采集源ID不能为空")
	}
	if err = s.ensureCollectSourceExists(ctx, in.Id, account.TenantId, account.Id); err != nil {
		return nil, err
	}
	if err = s.enqueueCollectSourceTrigger(ctx, collectTriggerQueuePayload{
		AccountId: account.Id,
		SourceId:  in.Id,
		TenantId:  account.TenantId,
	}, 0); err != nil {
		return nil, err
	}
	count, err := s.collectSourceTriggerEventCount(ctx, in.Id, account.TenantId, account.Id)
	if err != nil {
		return nil, err
	}
	return &sysin.CollectSourceTriggerModel{QueuedCount: count}, nil
}

func (s *sSysPublish) ExecuteCollectSourceTrigger(ctx context.Context, sourceId int64, tenantId int64, accountId int64) (*sysin.CollectSourceTriggerModel, error) {
	if sourceId <= 0 || tenantId <= 0 || accountId <= 0 {
		return nil, gerror.New("采集源重试参数不完整")
	}
	if err := s.ensureCollectSourceExists(ctx, sourceId, tenantId, accountId); err != nil {
		return nil, err
	}
	eventIds, err := s.collectSourceTriggerEventIds(ctx, sourceId, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	res := &sysin.CollectSourceTriggerModel{}
	for _, eventId := range eventIds {
		if err = s.syncCollectEventMediaSnapshot(ctx, eventId); err != nil {
			res.FailedCount++
			s.appendCollectEventLog(ctx, eventId, "manual", "failed", "重试前刷新媒体快照失败", err.Error())
			continue
		}
		if err = s.resetCollectEventForTrigger(ctx, eventId); err != nil {
			res.FailedCount++
			s.appendCollectEventLog(ctx, eventId, "manual", "failed", "重试前重置事件失败", err.Error())
			continue
		}
		if err = s.processCollectEvent(ctx, eventId, tenantId, accountId); err != nil {
			res.FailedCount++
			s.appendCollectEventLog(ctx, eventId, "manual", "failed", "重试采集推送失败", err.Error())
			continue
		}
		res.ProcessedCount++
		s.appendCollectEventLog(ctx, eventId, "manual", "triggered", "重试采集推送完成", "")
	}
	return res, nil
}

func (s *sSysPublish) CollectSourceReset(ctx context.Context, in *sysin.CollectSourceResetInp) (*sysin.CollectSourceResetModel, error) {
	if !collectSourceResetAllowed(ctx) {
		return nil, gerror.New("仅开发模式允许重置采集推送状态")
	}
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil || in.Id <= 0 {
		return nil, gerror.New("采集源ID不能为空")
	}
	if err = s.ensureCollectSourceExists(ctx, in.Id, account.TenantId, account.Id); err != nil {
		return nil, err
	}
	return s.resetCollectSourceForDev(ctx, in.Id, account.TenantId, account.Id)
}

func collectSourceResetAllowed(ctx context.Context) bool {
	mode := g.Cfg().MustGet(ctx, "system.mode", "").String()
	return mode == "" || mode == "develop" || mode == "testing" || mode == "not-set"
}

func (s *sSysPublish) ensureCollectSourceExists(ctx context.Context, sourceId int64, tenantId int64, accountId int64) error {
	exists, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereNull("deleted_at").
		Fields("id").
		Value()
	if err != nil {
		return gerror.Wrap(err, "读取采集源失败")
	}
	if exists.Int64() <= 0 {
		return gerror.New("采集源不存在")
	}
	return nil
}

func (s *sSysPublish) collectSourceTriggerEventCount(ctx context.Context, sourceId int64, tenantId int64, accountId int64) (int, error) {
	count, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("source_id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("status", collectSourceTriggerStatuses()).
		Count()
	return count, gerror.Wrap(err, "统计待重试采集事件失败")
}

func (s *sSysPublish) collectSourceTriggerEventIds(ctx context.Context, sourceId int64, tenantId int64, accountId int64) ([]int64, error) {
	var rows []struct {
		Id int64 `json:"id"`
	}
	err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields("id").
		Where("source_id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("status", collectSourceTriggerStatuses()).
		OrderAsc("received_at").
		OrderAsc("id").
		Limit(200).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取待重试采集事件失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.Id > 0 {
			ids = append(ids, row.Id)
		}
	}
	return ids, nil
}

func collectSourceTriggerStatuses() []string {
	return []string{
		sysin.CollectEventStatusPending,
		sysin.CollectEventStatusWaitingOrder,
		sysin.CollectEventStatusPrechecked,
		sysin.CollectEventStatusMediaPending,
		sysin.CollectEventStatusMediaReady,
		sysin.CollectEventStatusDispatched,
		"matched",
		sysin.CollectEventStatusProcessed,
		sysin.CollectEventStatusIgnored,
		sysin.CollectEventStatusFailed,
	}
}

func (s *sSysPublish) resetCollectEventForTrigger(ctx context.Context, eventId int64) error {
	if err := s.clearCollectEventWorkflow(ctx, eventId); err != nil {
		return err
	}
	_, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", eventId).Data(g.Map{
		"status":        sysin.CollectEventStatusPending,
		"error_message": "",
		"processed_at":  nil,
		"updated_at":    gtime.Now(),
	}).Update()
	return gerror.Wrap(err, "重置采集事件状态失败")
}

func (s *sSysPublish) clearCollectEventWorkflow(ctx context.Context, eventId int64) error {
	if eventId <= 0 {
		return nil
	}
	if _, err := pdao.YoubanPublishCollectReview.Ctx(ctx).Where("event_id", eventId).Delete(); err != nil {
		return gerror.Wrap(err, "清理采集审核失败")
	}
	if _, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where("event_id", eventId).Delete(); err != nil {
		return gerror.Wrap(err, "清理采集分发失败")
	}
	return nil
}

func (s *sSysPublish) resetCollectSourceForDev(ctx context.Context, sourceId int64, tenantId int64, accountId int64) (*sysin.CollectSourceResetModel, error) {
	eventCount, err := s.collectSourceResetEventCount(ctx, sourceId, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	res := &sysin.CollectSourceResetModel{EventCount: eventCount}
	if eventCount == 0 {
		return res, nil
	}
	taskIds, err := s.collectSourceTaskIds(ctx, sourceId, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	if err = s.resetCollectSourceTasksForDev(ctx, taskIds, tenantId); err != nil {
		return nil, err
	}
	reviewCount, err := pdao.YoubanPublishCollectReview.Ctx(ctx).
		Where("source_id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Delete()
	if err != nil {
		return nil, gerror.Wrap(err, "清理采集审核失败")
	}
	dispatchCount, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("source_id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Delete()
	if err != nil {
		return nil, gerror.Wrap(err, "清理采集分发失败")
	}
	_, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("source_id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Data(g.Map{
			"status":        sysin.CollectEventStatusPending,
			"error_message": "",
			"processed_at":  nil,
			"updated_at":    gtime.Now(),
		}).
		Update()
	if err != nil {
		return nil, gerror.Wrap(err, "重置采集事件失败")
	}
	if affected, err := reviewCount.RowsAffected(); err == nil {
		res.ReviewCount = int(affected)
	}
	if affected, err := dispatchCount.RowsAffected(); err == nil {
		res.DispatchCount = int(affected)
	}
	return res, nil
}

func (s *sSysPublish) resetCollectSourceTasksForDev(ctx context.Context, taskIds []int64, tenantId int64) error {
	taskIds = uniqueIds(taskIds)
	if len(taskIds) == 0 {
		return nil
	}
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("task_id", taskIds).
		WhereIn("status", []string{"pending", "sending", "failed_retry", "failed", "sent"}).
		Data(g.Map{
			"status":          "superseded",
			"dispatch_status": tgDispatchStatusDone,
			"next_retry_at":   nil,
			"next_cycle_at":   nil,
			"error_message":   "开发模式重置采集源推送状态",
			"updated_at":      gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "废弃采集源TG任务失败")
	}
	_, err = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("id", taskIds).
		Data(g.Map{
			"status":        sysin.PublishTaskStatusCanceled,
			"tg_status":     sysin.PublishTaskStatusCanceled,
			"error_message": "开发模式重置采集源推送状态",
			"updated_at":    gtime.Now(),
		}).
		Update()
	return gerror.Wrap(err, "重置采集源上架任务失败")
}

func (s *sSysPublish) collectSourceResetEventCount(ctx context.Context, sourceId int64, tenantId int64, accountId int64) (int, error) {
	count, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("source_id", sourceId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Count()
	return count, gerror.Wrap(err, "统计采集源事件失败")
}

func (s *sSysPublish) enqueueCollectSourceTrigger(ctx context.Context, payload collectTriggerQueuePayload, delay time.Duration) error {
	if payload.SourceId <= 0 || payload.TenantId <= 0 || payload.AccountId <= 0 {
		return nil
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	task := asynq.NewTask(tgTaskTypeCollectTrigger, body)
	options := []asynq.Option{
		asynq.Queue(tgQueueNameBulk),
		asynq.MaxRetry(0),
		asynq.Timeout(30 * time.Minute),
		asynq.Unique(30 * time.Second),
	}
	if delay > 0 {
		options = append(options, asynq.ProcessIn(delay))
	}
	_, err = client.EnqueueContext(ctx, task, options...)
	if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

func decodeCollectTriggerQueuePayload(task *asynq.Task) (collectTriggerQueuePayload, error) {
	var payload collectTriggerQueuePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return payload, fmt.Errorf("解析采集源重试任务失败: %w", err)
	}
	if payload.SourceId <= 0 || payload.TenantId <= 0 || payload.AccountId <= 0 {
		return payload, fmt.Errorf("采集源重试任务参数不完整")
	}
	return payload, nil
}
