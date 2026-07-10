package sys

import (
	"context"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
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
	channelIds := decodeInt64JSON(task["channel_id_json"].String())
	if len(channelIds) == 0 {
		return gerror.New("采集规则未配置目标频道")
	}
	operationNo := task["tg_operation_no"].String()
	if operationNo == "" {
		operationNo = newTelegramOperationNo(telegramPublishBizCollect, taskId)
	}
	if err = s.markTaskPublishQueued(ctx, taskId, task["tenant_id"].Int64(), task["account_id"].Int64(), operationNo); err != nil {
		return err
	}
	if err = s.supersedeCollectTgJobsOutsideChannels(ctx, taskId, operationNo, channelIds); err != nil {
		return err
	}
	if err = s.submitTelegramPublish(ctx, telegramPublishRequest{
		TaskId:               taskId,
		OperationNo:          operationNo,
		OperationPrefix:      telegramPublishBizCollect,
		ChannelIds:           channelIds,
		OnlySelectedChannels: true,
	}); err != nil {
		_ = s.markTaskPublishFailed(ctx, taskId, task["tenant_id"].Int64(), task["account_id"].Int64(), err)
		return gerror.Wrap(err, "采集TG任务创建失败")
	}
	return nil
}

func (s *sSysPublish) supersedeCollectTgJobsOutsideChannels(ctx context.Context, taskId int64, operationNo string, channelIds []int64) error {
	if taskId <= 0 || operationNo == "" || len(channelIds) == 0 {
		return nil
	}
	var jobs []telegramResubmitJob
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		Where("operation_no", operationNo).
		WhereIn("status", []string{"pending", "sending", "failed_retry", "failed", "sent"}).
		WhereNotIn("channel_id", channelIds).
		Scan(&jobs)
	if err != nil {
		return gerror.Wrap(err, "读取采集错投TG任务失败")
	}
	for _, job := range jobs {
		if job.Id <= 0 {
			continue
		}
		if job.Status == "sent" {
			if err = s.deleteTelegramJobMessagesForResubmit(ctx, job); err != nil {
				return err
			}
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "publish", "superseded", "采集规则目标频道已校正，错投频道任务已废弃")
		if err = s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) repairCollectTgJobChannels(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 100
	}
	if err := s.supersedeMismatchedCollectTgJobs(ctx, limit*4); err != nil {
		return err
	}
	rows, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Fields("id,channel_id_json,tg_operation_no").
		WhereLike("client_request_id", "collect:%").
		WhereIn("status", []string{sysin.PublishTaskStatusPending, sysin.PublishTaskStatusPublishing}).
		WhereNull("deleted_at").
		OrderAsc("id").
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取采集TG频道修复任务失败")
	}
	repaired := 0
	for _, row := range rows {
		channelIds := decodeInt64JSON(row["channel_id_json"].String())
		if len(channelIds) == 0 {
			continue
		}
		needsRepair, err := s.collectTgJobChannelsNeedRepair(ctx, row["id"].Int64(), row["tg_operation_no"].String(), channelIds)
		if err != nil {
			return err
		}
		if !needsRepair {
			continue
		}
		if err = s.ensureCollectTgJobs(ctx, row["id"].Int64()); err != nil {
			g.Log().Warningf(ctx, "修复采集TG目标频道失败 task:%d err:%+v", row["id"].Int64(), err)
			continue
		}
		repaired++
	}
	if repaired > 0 {
		g.Log().Infof(ctx, "已修复采集TG目标频道任务：%d条", repaired)
	}
	return nil
}

func (s *sSysPublish) supersedeMismatchedCollectTgJobs(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 400
	}
	rows, err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		LeftJoin(publishTaskTable+" t", "t.id=j.task_id").
		Fields("j.*,t.channel_id_json").
		WhereLike("t.client_request_id", "collect:%").
		WhereIn("j.status", []string{"pending", "sending", "failed_retry", "failed", "sent"}).
		WhereNull("t.deleted_at").
		OrderAsc("j.id").
		Limit(limit).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取采集错投TG任务失败")
	}
	fixed := 0
	for _, row := range rows {
		channelIds := decodeInt64JSON(row["channel_id_json"].String())
		if len(channelIds) == 0 || containsInt64(channelIds, row["channel_id"].Int64()) {
			continue
		}
		job := collectTelegramResubmitJob(row)
		if job.Status == "sent" {
			if err = s.deleteTelegramJobMessagesForResubmit(ctx, job); err != nil {
				return err
			}
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "publish", "superseded", "采集规则目标频道已校正，错投频道任务已废弃")
		if err = s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			return err
		}
		fixed++
	}
	if fixed > 0 {
		g.Log().Infof(ctx, "已废弃采集错投TG任务：%d条", fixed)
	}
	return nil
}

func (s *sSysPublish) collectTgJobChannelsNeedRepair(ctx context.Context, taskId int64, operationNo string, channelIds []int64) (bool, error) {
	if taskId <= 0 || operationNo == "" || len(channelIds) == 0 {
		return false, nil
	}
	var jobs []telegramResubmitJob
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("task_id", taskId).
		Where("operation_no", operationNo).
		WhereIn("status", []string{"pending", "sending", "failed_retry", "failed", "sent"}).
		Scan(&jobs)
	if err != nil {
		return false, gerror.Wrap(err, "读取采集TG任务频道失败")
	}
	hasSelected := false
	for _, job := range jobs {
		if containsInt64(channelIds, job.ChannelId) {
			hasSelected = true
			continue
		}
		return true, nil
	}
	return !hasSelected, nil
}

func collectTelegramResubmitJob(row gdb.Record) telegramResubmitJob {
	return telegramResubmitJob{
		Id:           row["id"].Int64(),
		TaskId:       row["task_id"].Int64(),
		OperationNo:  row["operation_no"].String(),
		TenantId:     row["tenant_id"].Int64(),
		AccountId:    row["account_id"].Int64(),
		ProfileId:    row["profile_id"].Int64(),
		ChannelId:    row["channel_id"].Int64(),
		BotId:        row["bot_id"].Int64(),
		TargetChatId: row["target_chat_id"].String(),
		Status:       row["status"].String(),
	}
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
	rows, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields("event_id").
		Where("task_id", taskId).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing}).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取采集分发事件失败")
	}
	_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("task_id", taskId).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing}).
		Data(g.Map{
			"status":        sysin.CollectDispatchStatusSent,
			"error_message": "",
			"finished_at":   gtime.Now(),
			"updated_at":    gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新采集分发发送状态失败")
	}
	for _, row := range rows {
		eventId := row["event_id"].Int64()
		if eventId <= 0 {
			continue
		}
		if _, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).
			Where("id", eventId).
			Where("status", sysin.CollectEventStatusDispatched).
			Data(g.Map{
				"status":        sysin.CollectEventStatusProcessed,
				"error_message": "",
				"processed_at":  gtime.Now(),
				"updated_at":    gtime.Now(),
			}).
			Update(); err != nil {
			return gerror.Wrap(err, "更新采集事件完成状态失败")
		}
	}
	return nil
}

func (s *sSysPublish) markCollectDispatchFailedByTask(ctx context.Context, taskId int64, message string) error {
	if taskId <= 0 {
		return nil
	}
	rows, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields("event_id").
		Where("task_id", taskId).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing}).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取采集失败分发事件失败")
	}
	_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("task_id", taskId).
		WhereIn("status", []string{sysin.CollectDispatchStatusPending, sysin.CollectDispatchStatusReviewing}).
		Data(g.Map{
			"status":        sysin.CollectDispatchStatusFailed,
			"error_message": message,
			"finished_at":   gtime.Now(),
			"updated_at":    gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新采集分发失败状态失败")
	}
	return s.markCollectEventsFailedByDispatchRows(ctx, rows, message)
}

func (s *sSysPublish) markCollectDispatchFailed(ctx context.Context, dispatchId int64, message string) error {
	if dispatchId <= 0 {
		return nil
	}
	rows, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields("event_id").
		Where("id", dispatchId).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取采集失败分发事件失败")
	}
	_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("id", dispatchId).
		Data(g.Map{
			"status":        sysin.CollectDispatchStatusFailed,
			"error_message": message,
			"finished_at":   gtime.Now(),
			"updated_at":    gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新采集分发失败状态失败")
	}
	return s.markCollectEventsFailedByDispatchRows(ctx, rows, message)
}

func (s *sSysPublish) markCollectEventsFailedByDispatchRows(ctx context.Context, rows gdb.Result, message string) error {
	now := gtime.Now()
	for _, row := range rows {
		eventId := row["event_id"].Int64()
		if eventId <= 0 {
			continue
		}
		if _, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
			Where("id", eventId).
			Where("status", sysin.CollectEventStatusDispatched).
			Data(g.Map{
				"status":        sysin.CollectEventStatusFailed,
				"error_message": message,
				"processed_at":  now,
				"updated_at":    now,
			}).
			Update(); err != nil {
			return gerror.Wrap(err, "更新采集事件失败状态失败")
		}
	}
	return nil
}
