package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) bootstrapCyclePlans(ctx context.Context) error {
	var rows []struct {
		TenantId    int64  `orm:"tenant_id" json:"tenantId"`
		AccountId   int64  `orm:"account_id" json:"accountId"`
		ProfileId   int64  `orm:"profile_id" json:"profileId"`
		TaskId      int64  `orm:"task_id" json:"taskId"`
		ChannelId   int64  `orm:"channel_id" json:"channelId"`
		Days        int    `orm:"days" json:"days"`
		PublishTime string `orm:"publish_time" json:"publishTime"`
	}
	err := g.DB().Model(publishChannelTable+" c").Safe().Ctx(ctx).
		Fields("c.tenant_id,j.account_id,j.profile_id,j.task_id,c.id AS channel_id,c.cycle_publish_days AS days,c.cycle_publish_time AS publish_time").
		LeftJoin(publishTgJobTable+" j", "j.channel_id=c.id AND j.status='sent'").
		Where("c.publish_direction", "up").
		Where("c.cycle_publish_enabled", 1).
		Where("c.status", 1).
		Where("c.deleted_at IS NULL").
		Group("c.tenant_id,j.account_id,j.profile_id,j.task_id,c.id,c.cycle_publish_days,c.cycle_publish_time").
		Scan(&rows)
	if err != nil {
		return gerror.Wrap(err, "读取已开启循环上架频道失败")
	}
	for _, row := range rows {
		if row.TenantId <= 0 || row.AccountId <= 0 || row.ProfileId <= 0 || row.TaskId <= 0 || row.ChannelId <= 0 {
			continue
		}
		if err = s.ensureChannelCyclePlan(ctx, row.TenantId, row.AccountId, row.ProfileId, row.TaskId, row.ChannelId, cyclePlanSetting{
			Enabled:     1,
			Days:        row.Days,
			PublishTime: row.PublishTime,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) ensureChannelCyclePlan(ctx context.Context, tenantId int64, accountId int64, profileId int64, taskId int64, channelId int64, cycle cyclePlanSetting) error {
	if tenantId <= 0 || accountId <= 0 || profileId <= 0 || taskId <= 0 || channelId <= 0 {
		return nil
	}
	if cycle.Enabled != 1 {
		return nil
	}
	intervalSeconds := s.cycleIntervalSeconds(ctx, cycle.Days)
	data := g.Map{
		"tenant_id":          tenantId,
		"account_id":         accountId,
		"profile_id":         profileId,
		"channel_id":         channelId,
		"task_id":            taskId,
		"enabled":            1,
		"interval_seconds":   intervalSeconds,
		"publish_time":       cycle.PublishTime,
		"status":             cyclePlanStatusActive,
		"source":             "channel",
		"last_error_message": "",
	}
	return s.upsertCyclePlan(ctx, data)
}

func (s *sSysPublish) syncChannelCycleAfterSave(ctx context.Context, tenantId int64, channelId int64, enabled int, days int, publishTime string) error {
	if tenantId <= 0 || channelId <= 0 {
		return nil
	}
	now := gtime.Now()
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("channel_id", channelId).
		WhereIn("status", []string{"pending", "sending", "failed_retry", "failed", "sent"}).
		Data(g.Map{
			"cycle_enabled":      enabled,
			"cycle_days":         defaultCycleDays(days),
			"cycle_publish_time": publishTime,
			"updated_at":         now,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "同步频道TG循环配置失败")
	}
	if enabled != 1 {
		_, err = g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
			Where("tenant_id", tenantId).
			Where("channel_id", channelId).
			WhereNull("deleted_at").
			Data(g.Map{
				"enabled":    0,
				"status":     cyclePlanStatusDisabled,
				"locked_at":  nil,
				"updated_at": now,
			}).
			Update()
		if err != nil {
			return gerror.Wrap(err, "关闭频道循环计划失败")
		}
		return nil
	}
	var rows []struct {
		TenantId  int64 `orm:"tenant_id" json:"tenantId"`
		AccountId int64 `orm:"account_id" json:"accountId"`
		ProfileId int64 `orm:"profile_id" json:"profileId"`
		TaskId    int64 `orm:"task_id" json:"taskId"`
	}
	err = g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		LeftJoin(publishTaskTable+" t", "t.id=j.task_id AND t.deleted_at IS NULL").
		Fields("j.tenant_id,j.account_id,j.profile_id,j.task_id").
		Where("j.tenant_id", tenantId).
		Where("j.channel_id", channelId).
		Where("j.status", "sent").
		Where("t.status", sysin.PublishTaskStatusPublished).
		Group("j.tenant_id,j.account_id,j.profile_id,j.task_id").
		Scan(&rows)
	if err != nil {
		return gerror.Wrap(err, "读取频道已上架循环资料失败")
	}
	setting := cyclePlanSetting{Enabled: 1, Days: days, PublishTime: publishTime}
	for _, row := range rows {
		if err = s.syncChannelCycleSettingToPlans(ctx, row.TenantId, row.AccountId, row.ProfileId, row.TaskId, channelId, setting); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) ensureCyclePlanForJob(ctx context.Context, job telegramJobRecord) error {
	if job.TaskId <= 0 || job.ProfileId <= 0 || job.AccountId <= 0 || job.TenantId <= 0 {
		return nil
	}
	channel, err := s.channelById(ctx, job.TenantId, job.ChannelId)
	if err != nil {
		return err
	}
	if channel.CyclePublishEnabled != 1 {
		return s.disableCyclePlanForProfile(ctx, job.TenantId, job.AccountId, job.ProfileId, job.ChannelId)
	}
	task, err := s.cycleTaskForJob(ctx, job)
	if err != nil {
		return err
	}
	if task.IsEmpty() || task["status"].String() != sysin.PublishTaskStatusPublished {
		return nil
	}
	return s.upsertCyclePlan(ctx, g.Map{
		"tenant_id":          job.TenantId,
		"account_id":         job.AccountId,
		"profile_id":         job.ProfileId,
		"channel_id":         job.ChannelId,
		"task_id":            job.TaskId,
		"enabled":            1,
		"interval_seconds":   s.cycleIntervalSeconds(ctx, channel.CyclePublishDays),
		"publish_time":       channel.CyclePublishTime,
		"next_run_at":        s.nextCycleRunAt(ctx, cyclePlanRecord{IntervalSeconds: s.cycleIntervalSeconds(ctx, channel.CyclePublishDays), PublishTime: channel.CyclePublishTime}, gtime.Now()),
		"status":             cyclePlanStatusActive,
		"source":             "channel",
		"last_error_message": "",
	})
}

type cyclePlanSetting struct {
	Enabled     int
	Days        int
	PublishTime string
}

func (s *sSysPublish) syncChannelCycleSettingToPlans(ctx context.Context, tenantId int64, accountId int64, profileId int64, taskId int64, channelId int64, cycle cyclePlanSetting) error {
	if tenantId <= 0 || accountId <= 0 || profileId <= 0 || taskId <= 0 || channelId <= 0 {
		return nil
	}
	now := gtime.Now()
	intervalSeconds := s.cycleIntervalSeconds(ctx, cycle.Days)
	if cycle.Enabled != 1 {
		_, err := g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
			Where("tenant_id", tenantId).
			Where("account_id", accountId).
			Where("profile_id", profileId).
			Where("channel_id", channelId).
			WhereNull("deleted_at").
			Data(g.Map{
				"enabled":    0,
				"status":     cyclePlanStatusDisabled,
				"locked_at":  nil,
				"updated_at": now,
			}).
			Update()
		if err != nil {
			return gerror.Wrap(err, "关闭频道循环上架计划失败")
		}
		return nil
	}
	_, err := g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("profile_id", profileId).
		Where("channel_id", channelId).
		WhereNull("deleted_at").
		Data(g.Map{
			"enabled":    0,
			"status":     cyclePlanStatusDisabled,
			"locked_at":  nil,
			"updated_at": now,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "重置频道循环上架计划失败")
	}
	plan := cyclePlanRecord{IntervalSeconds: intervalSeconds, PublishTime: cycle.PublishTime}
	if err = s.upsertCyclePlan(ctx, g.Map{
		"tenant_id":          tenantId,
		"account_id":         accountId,
		"profile_id":         profileId,
		"channel_id":         channelId,
		"task_id":            taskId,
		"enabled":            1,
		"interval_seconds":   intervalSeconds,
		"publish_time":       cycle.PublishTime,
		"next_run_at":        s.nextCycleRunAt(ctx, plan, now),
		"status":             cyclePlanStatusActive,
		"source":             "channel_setting",
		"last_error_message": "",
	}); err != nil {
		return err
	}
	return nil
}

func (s *sSysPublish) upsertCyclePlan(ctx context.Context, data g.Map) error {
	now := gtime.Now()
	data["updated_at"] = now
	count, err := g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
		Where("tenant_id", data["tenant_id"]).
		Where("account_id", data["account_id"]).
		Where("profile_id", data["profile_id"]).
		Where("channel_id", data["channel_id"]).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查循环上架计划失败")
	}
	if count > 0 {
		delete(data, "next_run_at")
		_, err = g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
			Where("tenant_id", data["tenant_id"]).
			Where("account_id", data["account_id"]).
			Where("profile_id", data["profile_id"]).
			Where("channel_id", data["channel_id"]).
			WhereNull("deleted_at").
			Data(data).
			Update()
	} else {
		if _, ok := data["next_run_at"]; !ok {
			intervalSeconds := 0
			switch value := data["interval_seconds"].(type) {
			case int:
				intervalSeconds = value
			case int64:
				intervalSeconds = int(value)
			case float64:
				intervalSeconds = int(value)
			}
			data["next_run_at"] = s.nextCycleRunAt(ctx, cyclePlanRecord{
				IntervalSeconds: intervalSeconds,
				PublishTime:     fmt.Sprint(data["publish_time"]),
			}, now)
		}
		data["created_at"] = now
		_, err = g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存循环上架计划失败")
	}
	return nil
}

func (s *sSysPublish) disableCyclePlanForProfile(ctx context.Context, tenantId int64, accountId int64, profileId int64, channelId int64) error {
	if tenantId <= 0 || accountId <= 0 || profileId <= 0 {
		return nil
	}
	mod := g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("profile_id", profileId)
	if channelId > 0 {
		mod = mod.Where("channel_id", channelId)
	}
	_, err := mod.WhereNull("deleted_at").
		Data(g.Map{
			"enabled":    0,
			"status":     cyclePlanStatusDisabled,
			"locked_at":  nil,
			"updated_at": gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "关闭资料循环上架计划失败")
	}
	return nil
}

func (s *sSysPublish) cycleIntervalSeconds(ctx context.Context, days int) int {
	days = defaultCycleDays(days)
	if isDevelopMode(ctx) {
		return days
	}
	return int((time.Duration(days) * 24 * time.Hour).Seconds())
}

func (s *sSysPublish) nextCycleRunAt(ctx context.Context, plan cyclePlanRecord, now *gtime.Time) *gtime.Time {
	if now == nil {
		now = gtime.Now()
	}
	interval := plan.IntervalSeconds
	if interval <= 0 {
		interval = s.cycleIntervalSeconds(ctx, 4)
	}
	next := now.Add(time.Duration(interval) * time.Second)
	if isDevelopMode(ctx) {
		return next
	}
	hour, minute, ok := parseCycleClock(plan.PublishTime)
	if !ok {
		return next
	}
	base := next.Time
	return gtime.New(time.Date(base.Year(), base.Month(), base.Day(), hour, minute, 0, 0, base.Location()))
}
