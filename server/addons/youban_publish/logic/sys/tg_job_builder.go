package sys

import (
	"context"
	"encoding/json"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) ensureTgJobs(ctx context.Context, taskId int64) error {
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
	for _, channel := range channels {
		jobId, err := s.ensureTgChannelJob(ctx, task, channel)
		if err != nil {
			return err
		}
		if err = s.enqueueTelegramJob(ctx, jobId, 0); err != nil {
			return gerror.Wrap(err, "TG任务入队失败")
		}
	}
	return nil
}

func (s *sSysPublish) telegramJobChannels(ctx context.Context, task gdb.Record) ([]telegramJobChannel, error) {
	channelIds := decodeInt64JSON(task["channel_id_json"].String())
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
	var channels []telegramJobChannel
	if err := mod.Fields("id,target_chat_id,bot_id_json,cycle_publish_enabled,cycle_publish_days,cycle_publish_time").OrderAsc("id").Scan(&channels); err != nil {
		return nil, gerror.Wrap(err, "读取TG推送频道失败")
	}
	if len(channels) == 0 {
		return nil, gerror.New("未配置可推送频道")
	}
	return channels, nil
}

func (s *sSysPublish) ensureTgChannelJob(ctx context.Context, task gdb.Record, channel telegramJobChannel) (int64, error) {
	value, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("task_id", task["id"].Int64()).
		Where("channel_id", channel.Id).
		Fields("id").
		Value()
	if err != nil {
		return 0, gerror.Wrap(err, "读取TG频道任务失败")
	}
	jobId := value.Int64()
	if jobId > 0 {
		return jobId, nil
	}
	botId := firstPositiveId(decodeBotIds(channel.BotIdJson))
	now := gtime.Now()
	jobId, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(g.Map{
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
		return 0, gerror.Wrap(err, "创建TG频道任务失败")
	}
	return jobId, nil
}

func decodeInt64JSON(raw string) []int64 {
	var ids []int64
	_ = json.Unmarshal([]byte(raw), &ids)
	return uniqueIds(ids)
}

func firstPositiveId(ids []int64) int64 {
	for _, id := range ids {
		if id > 0 {
			return id
		}
	}
	return 0
}

func defaultCycleDays(days int) int {
	if days <= 0 {
		return 4
	}
	return days
}

type telegramJobChannel struct {
	Id                  int64  `json:"id"`
	TargetChatId        string `json:"targetChatId"`
	BotIdJson           string `json:"botIdJson"`
	CyclePublishEnabled int    `json:"cyclePublishEnabled"`
	CyclePublishDays    int    `json:"cyclePublishDays"`
	CyclePublishTime    string `json:"cyclePublishTime"`
}
