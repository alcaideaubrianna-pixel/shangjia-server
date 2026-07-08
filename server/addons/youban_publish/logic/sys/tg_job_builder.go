package sys

import (
	"context"
	"encoding/json"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) ensureTgJobs(ctx context.Context, taskId int64, operationNo string) error {
	return s.submitTelegramPublish(ctx, telegramPublishRequest{
		TaskId:          taskId,
		OperationNo:     operationNo,
		OperationPrefix: telegramPublishBizProfile,
	})
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
	if err := mod.Fields("id,target_chat_id,bot_id_json").OrderAsc("id").Scan(&channels); err != nil {
		return nil, gerror.Wrap(err, "读取TG推送频道失败")
	}
	if len(channels) == 0 {
		return nil, gerror.New("未配置可推送频道")
	}
	return channels, nil
}

func (s *sSysPublish) telegramJobCycleSetting(ctx context.Context, task gdb.Record) (accountCycleSetting, error) {
	setting, err := s.accountSetting(ctx, task["tenant_id"].Int64(), task["account_id"].Int64())
	if err != nil {
		return accountCycleSetting{}, err
	}
	return accountCycleSetting{
		Enabled:     setting.CyclePublishEnabled,
		Days:        setting.CyclePublishDays,
		PublishTime: setting.CyclePublishTime,
	}, nil
}

func (s *sSysPublish) updateTelegramJobCycleSetting(ctx context.Context, jobId int64, cycle accountCycleSetting) error {
	if jobId <= 0 {
		return nil
	}
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).
		Data(g.Map{
			"cycle_enabled":      cycle.Enabled,
			"cycle_days":         defaultCycleDays(cycle.Days),
			"cycle_publish_time": cycle.PublishTime,
			"updated_at":         gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新TG任务循环设置失败")
	}
	return nil
}

type accountCycleSetting struct {
	Enabled     int
	Days        int
	PublishTime string
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
	Id           int64  `json:"id"`
	TargetChatId string `json:"targetChatId"`
	BotIdJson    string `json:"botIdJson"`
}
