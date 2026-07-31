package sys

import (
	"context"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const publishChannelProfileTable = "hg_youban_publish_channel_profile"

func (s *sSysPublish) syncChannelCycleAfterSave(ctx context.Context, tenantId int64, channelId int64, enabled int, days int, publishTime string) error {
	if tenantId <= 0 || channelId <= 0 {
		return nil
	}
	data := g.Map{
		"cycle_active_run_id":      0,
		"cycle_last_error_message": "",
		"updated_at":               gtime.Now(),
	}
	data["cycle_next_run_at"] = nil
	_, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Where("id", channelId).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Data(data).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新频道循环上架时间失败")
	}
	if err = s.enqueueCycleReschedule(ctx, channelId, 0); err != nil {
		g.Log().Warningf(ctx, "提交频道循环重算任务失败，定时调度将自动恢复 channel:%d err:%+v", channelId, err)
	}
	return nil
}

func parseCycleClock(value string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) < 2 {
		return 0, 0, false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, false
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, false
	}
	return hour, minute, true
}
