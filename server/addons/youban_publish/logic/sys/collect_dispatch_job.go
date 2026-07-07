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

func (s *sSysPublish) ensureCollectTgJobs(ctx context.Context, taskId int64, rule gdb.Record) error {
	task, err := s.telegramJobTask(ctx, taskId)
	if err != nil {
		return err
	}
	if task["tg_push_enabled"].Int() != 1 {
		return nil
	}
	channels, err := s.telegramJobChannels(ctx, task)
	if err != nil {
		return err
	}
	if err = s.prepareTelegramTaskForResubmit(ctx, task, channels); err != nil {
		return err
	}
	ruleBotIds := decodeBotIds(rule["bot_id_json"].String())
	for _, channel := range channels {
		jobId, err := s.ensureCollectTgChannelJob(ctx, task, channel, ruleBotIds)
		if err != nil {
			return err
		}
		if err = s.enqueueTelegramJob(ctx, jobId, 0); err != nil {
			return gerror.Wrap(err, "采集TG任务入队失败")
		}
	}
	return nil
}

func (s *sSysPublish) ensureCollectTgChannelJob(ctx context.Context, task gdb.Record, channel telegramJobChannel, ruleBotIds []int64) (int64, error) {
	value, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("task_id", task["id"].Int64()).
		Where("channel_id", channel.Id).
		Fields("id").
		Value()
	if err != nil {
		return 0, gerror.Wrap(err, "读取采集TG频道任务失败")
	}
	if jobId := value.Int64(); jobId > 0 {
		return jobId, nil
	}
	botId, err := collectChannelBotId(channel, ruleBotIds)
	if err != nil {
		return 0, err
	}
	now := gtime.Now()
	jobId, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
		"task_id":            task["id"].Int64(),
		"tenant_id":          task["tenant_id"].Int64(),
		"merchant_id":        task["tenant_id"].Int64(),
		"account_id":         task["account_id"].Int64(),
		"profile_id":         task["profile_id"].Int64(),
		"channel_id":         channel.Id,
		"bot_id":             botId,
		"target_chat_id":     normalizeTelegramChannelChatID(channel.TargetChatId),
		"status":             "pending",
		"cycle_enabled":      channel.CyclePublishEnabled,
		"cycle_days":         defaultCycleDays(channel.CyclePublishDays),
		"cycle_publish_time": channel.CyclePublishTime,
		"created_at":         now,
		"updated_at":         now,
	}).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建采集TG频道任务失败")
	}
	return jobId, nil
}

func collectChannelBotId(channel telegramJobChannel, ruleBotIds []int64) (int64, error) {
	channelBotIds := decodeBotIds(channel.BotIdJson)
	if len(channelBotIds) == 0 {
		botId := firstPositiveId(ruleBotIds)
		if botId <= 0 {
			return 0, gerror.New("目标频道未配置可用推送BOT")
		}
		return botId, nil
	}
	for _, ruleBotId := range ruleBotIds {
		if containsInt64(channelBotIds, ruleBotId) {
			return ruleBotId, nil
		}
	}
	if len(ruleBotIds) > 0 {
		return 0, gerror.New("规则选择的推送BOT未绑定到目标频道")
	}
	botId := firstPositiveId(channelBotIds)
	if botId <= 0 {
		return 0, gerror.New("目标频道未配置可用推送BOT")
	}
	return botId, nil
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
