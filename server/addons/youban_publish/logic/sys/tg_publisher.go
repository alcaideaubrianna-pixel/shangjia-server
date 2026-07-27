package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) submitTelegramPublish(ctx context.Context, req telegramPublishRequest) error {
	if err := ensureCollectTelegramOrderColumns(ctx); err != nil {
		return err
	}
	task, err := s.telegramJobTask(ctx, req.TaskId)
	if err != nil {
		return err
	}
	if task["tg_push_enabled"].Int() != 1 {
		return nil
	}
	channels, err := s.telegramJobChannels(ctx, task, req.ChannelIds)
	if err != nil {
		return err
	}
	operationNo := req.OperationNo
	if operationNo == "" {
		operationNo = task["tg_operation_no"].String()
	}
	if operationNo == "" {
		if !req.AllowCreateOperationNo {
			return gerror.New("TG操作号不能为空")
		}
		operationNo = newTelegramOperationNo(req.normalizedOperationPrefix(), req.TaskId)
		if _, err = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", req.TaskId).Data(g.Map{
			"tg_operation_no": operationNo,
			"updated_at":      gtime.Now(),
		}).Update(); err != nil {
			return gerror.Wrap(err, "更新TG操作号失败")
		}
	}
	if task["tg_operation_no"].String() != "" && task["tg_operation_no"].String() != operationNo {
		return nil
	}
	if err = s.prepareTelegramTaskForResubmit(ctx, task, channels, operationNo, req.OnlySelectedChannels); err != nil {
		return err
	}
	jobIds := make([]int64, 0, len(channels))
	for _, channel := range channels {
		jobId, createErr := s.ensureTelegramPublishChannelJob(ctx, task, channel, operationNo)
		if createErr != nil {
			return createErr
		}
		jobIds = append(jobIds, jobId)
	}

	// The database jobs are the durable source of truth. Redis only wakes the
	// workers; a failed enqueue is recovered by the database scheduler.
	pushDelay := s.collectRealtimePushDelay(ctx, task)
	for _, jobId := range jobIds {
		if err = s.enqueueTelegramJob(ctx, jobId, pushDelay); err != nil {
			message := "Redis调度失败，等待数据库调度器恢复：" + err.Error()
			_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobId).Data(g.Map{
				"dispatch_status":     tgDispatchStatusIdle,
				"last_dispatch_error": message,
				"updated_at":          gtime.Now(),
			}).Update()
			g.Log().Warningf(ctx, "TG任务入队失败，等待数据库调度器恢复 jobId:%d err:%+v", jobId, err)
		}
	}
	return nil
}

func (s *sSysPublish) ensureTelegramPublishChannelJob(ctx context.Context, task gdb.Record, channel telegramJobChannel, operationNo string) (int64, error) {
	existing, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("task_id", task["id"].Int64()).
		Where("operation_no", operationNo).
		Where("channel_id", channel.Id).
		Fields("id,status,bot_id,target_chat_id").
		One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取TG频道任务失败")
	}
	if jobId := existing["id"].Int64(); jobId > 0 {
		_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("id", jobId).
			Data(collectTelegramOrderDataFromTask(task)).
			Update()
		if err != nil {
			return 0, gerror.Wrap(err, "更新TG频道任务采集顺序失败")
		}
		status := existing["status"].String()
		if status == "sent" {
			status = "success"
		}
		if recordErr := s.upsertPublishJobRecord(ctx, telegramJobRecord{
			Id:           jobId,
			TaskId:       task["id"].Int64(),
			OperationNo:  operationNo,
			TenantId:     task["tenant_id"].Int64(),
			AccountId:    task["account_id"].Int64(),
			ProfileId:    task["profile_id"].Int64(),
			ChannelId:    channel.Id,
			BotId:        existing["bot_id"].Int64(),
			Status:       status,
			TargetChatId: existing["target_chat_id"].String(),
		}, status, ""); recordErr != nil {
			g.Log().Warningf(ctx, "补写发布记录失败 jobId:%d err:%+v", jobId, recordErr)
		}
		return jobId, nil
	}
	botId, err := telegramChannelSenderBotId(channel)
	if err != nil {
		return 0, err
	}
	now := gtime.Now()
	jobId, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
		"task_id":                   task["id"].Int64(),
		"operation_no":              operationNo,
		"tenant_id":                 task["tenant_id"].Int64(),
		"merchant_id":               task["tenant_id"].Int64(),
		"account_id":                task["account_id"].Int64(),
		"profile_id":                task["profile_id"].Int64(),
		"channel_id":                channel.Id,
		"bot_id":                    botId,
		"target_chat_id":            normalizeTelegramChannelChatID(channel.TargetChatId),
		"collect_event_id":          task["collect_event_id"].Int64(),
		"collect_source_id":         task["collect_source_id"].Int64(),
		"collect_source_chat_id":    strings.TrimSpace(task["collect_source_chat_id"].String()),
		"collect_source_message_id": task["collect_source_message_id"].Int64(),
		"status":                    "pending",
		"priority":                  s.telegramJobPriority(telegramJobRecord{OperationNo: operationNo}),
		"queue_name":                telegramQueueNameByPriority(s.telegramJobPriority(telegramJobRecord{OperationNo: operationNo})),
		"dispatch_status":           tgDispatchStatusIdle,
		"created_at":                now,
		"updated_at":                now,
	}).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建TG频道任务失败")
	}
	job := telegramJobRecord{
		Id:           jobId,
		TaskId:       task["id"].Int64(),
		OperationNo:  operationNo,
		TenantId:     task["tenant_id"].Int64(),
		AccountId:    task["account_id"].Int64(),
		ProfileId:    task["profile_id"].Int64(),
		ChannelId:    channel.Id,
		BotId:        botId,
		Status:       "pending",
		TargetChatId: normalizeTelegramChannelChatID(channel.TargetChatId),
	}
	if recordErr := s.upsertPublishJobRecord(ctx, job, "pending", ""); recordErr != nil {
		g.Log().Warningf(ctx, "保存待发送发布记录失败 jobId:%d err:%+v", jobId, recordErr)
	}
	return jobId, nil
}
