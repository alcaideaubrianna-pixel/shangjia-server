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
	if err := ensureTelegramOperationColumns(ctx); err != nil {
		return err
	}
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
	pushDelay := s.collectRealtimePushDelay(ctx, task)
	for _, channel := range channels {
		jobId, err := s.ensureTelegramPublishChannelJob(ctx, task, channel, operationNo)
		if err != nil {
			return err
		}
		if err = s.enqueueTelegramJob(ctx, jobId, pushDelay); err != nil {
			return gerror.Wrap(err, "TG任务入队失败")
		}
	}
	return nil
}

func (s *sSysPublish) ensureTelegramPublishChannelJob(ctx context.Context, task gdb.Record, channel telegramJobChannel, operationNo string) (int64, error) {
	value, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("task_id", task["id"].Int64()).
		Where("operation_no", operationNo).
		Where("channel_id", channel.Id).
		Fields("id").
		Value()
	if err != nil {
		return 0, gerror.Wrap(err, "读取TG频道任务失败")
	}
	if jobId := value.Int64(); jobId > 0 {
		if err = s.updateTelegramJobCycleSetting(ctx, jobId, channel); err != nil {
			return 0, err
		}
		_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("id", jobId).
			Data(collectTelegramOrderDataFromTask(task)).
			Update()
		if err != nil {
			return 0, gerror.Wrap(err, "更新TG频道任务采集顺序失败")
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
		"priority":                  s.telegramJobPriority(telegramJobRecord{OperationNo: operationNo, CycleEnabled: channel.CyclePublishEnabled}),
		"queue_name":                telegramQueueNameByPriority(s.telegramJobPriority(telegramJobRecord{OperationNo: operationNo, CycleEnabled: channel.CyclePublishEnabled})),
		"dispatch_status":           tgDispatchStatusIdle,
		"cycle_enabled":             channel.CyclePublishEnabled,
		"cycle_days":                defaultCycleDays(channel.CyclePublishDays),
		"cycle_publish_time":        channel.CyclePublishTime,
		"created_at":                now,
		"updated_at":                now,
	}).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建TG频道任务失败")
	}
	return jobId, nil
}

func (s *sSysPublish) updateTelegramJobCycleSetting(ctx context.Context, jobId int64, channel telegramJobChannel) error {
	if jobId <= 0 {
		return nil
	}
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).
		Data(g.Map{
			"cycle_enabled":      channel.CyclePublishEnabled,
			"cycle_days":         defaultCycleDays(channel.CyclePublishDays),
			"cycle_publish_time": channel.CyclePublishTime,
			"updated_at":         gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新TG任务循环设置失败")
	}
	return nil
}
