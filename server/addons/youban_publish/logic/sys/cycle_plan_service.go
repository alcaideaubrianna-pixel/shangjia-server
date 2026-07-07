package sys

import (
	"context"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) bootstrapCyclePlans(ctx context.Context) error {
	var rows []struct {
		TenantId    int64  `orm:"tenant_id" json:"tenantId"`
		AccountId   int64  `orm:"account_id" json:"accountId"`
		Days        int    `orm:"days" json:"days"`
		PublishTime string `orm:"publish_time" json:"publishTime"`
	}
	err := g.DB().Model(publishAccountSettingTable).Safe().Ctx(ctx).
		Fields("tenant_id,account_id,cycle_publish_days AS days,cycle_publish_time AS publish_time").
		Where("cycle_publish_enabled", 1).
		WhereNull("deleted_at").
		Scan(&rows)
	if err != nil {
		return gerror.Wrap(err, "读取已开启循环上架账号失败")
	}
	for _, row := range rows {
		if row.TenantId <= 0 || row.AccountId <= 0 {
			continue
		}
		if err = s.syncAccountCycleSettingToPlans(ctx, row.TenantId, row.AccountId, accountCycleSetting{
			Enabled:     1,
			Days:        row.Days,
			PublishTime: row.PublishTime,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) ensureCyclePlanForJob(ctx context.Context, job telegramJobRecord) error {
	if job.TaskId <= 0 || job.ProfileId <= 0 || job.AccountId <= 0 || job.TenantId <= 0 {
		return nil
	}
	setting, err := s.accountSetting(ctx, job.TenantId, job.AccountId)
	if err != nil {
		return err
	}
	if setting.CyclePublishEnabled != 1 {
		return s.disableCyclePlanForProfile(ctx, job.TenantId, job.AccountId, job.ProfileId)
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
		"task_id":            job.TaskId,
		"enabled":            1,
		"interval_seconds":   s.cycleIntervalSeconds(ctx, setting.CyclePublishDays),
		"publish_time":       setting.CyclePublishTime,
		"next_run_at":        s.nextCycleRunAt(ctx, cyclePlanRecord{IntervalSeconds: s.cycleIntervalSeconds(ctx, setting.CyclePublishDays), PublishTime: setting.CyclePublishTime}, gtime.Now()),
		"status":             cyclePlanStatusActive,
		"source":             "publish",
		"last_error_message": "",
	})
}

func (s *sSysPublish) syncAccountCycleSettingToPlans(ctx context.Context, tenantId int64, accountId int64, cycle accountCycleSetting) error {
	if tenantId <= 0 || accountId <= 0 {
		return nil
	}
	now := gtime.Now()
	intervalSeconds := s.cycleIntervalSeconds(ctx, cycle.Days)
	if cycle.Enabled != 1 {
		_, err := g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
			Where("tenant_id", tenantId).
			Where("account_id", accountId).
			WhereNull("deleted_at").
			Data(g.Map{
				"enabled":    0,
				"status":     cyclePlanStatusDisabled,
				"locked_at":  nil,
				"updated_at": now,
			}).
			Update()
		if err != nil {
			return gerror.Wrap(err, "关闭账号循环上架计划失败")
		}
		return nil
	}
	_, err := g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereNull("deleted_at").
		Data(g.Map{
			"enabled":    0,
			"status":     cyclePlanStatusDisabled,
			"locked_at":  nil,
			"updated_at": now,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "重置账号循环上架计划失败")
	}
	var rows []gdb.Record
	err = g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		LeftJoin(publishTaskTable+" t", "t.id=j.task_id").
		Fields("j.tenant_id,j.account_id,j.profile_id,MAX(j.task_id) AS task_id").
		Where("j.tenant_id", tenantId).
		Where("j.account_id", accountId).
		Where("j.status", "sent").
		Where("t.status", sysin.PublishTaskStatusPublished).
		WhereNull("t.deleted_at").
		Group("j.tenant_id,j.account_id,j.profile_id").
		Scan(&rows)
	if err != nil {
		return gerror.Wrap(err, "读取账号循环上架资料失败")
	}
	for _, row := range rows {
		plan := cyclePlanRecord{IntervalSeconds: intervalSeconds, PublishTime: cycle.PublishTime}
		if err = s.upsertCyclePlan(ctx, g.Map{
			"tenant_id":          row["tenant_id"].Int64(),
			"account_id":         row["account_id"].Int64(),
			"profile_id":         row["profile_id"].Int64(),
			"task_id":            row["task_id"].Int64(),
			"enabled":            1,
			"interval_seconds":   intervalSeconds,
			"publish_time":       cycle.PublishTime,
			"next_run_at":        s.nextCycleRunAt(ctx, plan, now),
			"status":             cyclePlanStatusActive,
			"source":             "account_setting",
			"last_error_message": "",
		}); err != nil {
			return err
		}
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
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查循环上架计划失败")
	}
	if count > 0 {
		_, err = g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
			Where("tenant_id", data["tenant_id"]).
			Where("account_id", data["account_id"]).
			Where("profile_id", data["profile_id"]).
			WhereNull("deleted_at").
			Data(data).
			Update()
	} else {
		data["created_at"] = now
		_, err = g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return gerror.Wrap(err, "保存循环上架计划失败")
	}
	return nil
}

func (s *sSysPublish) disableCyclePlanForProfile(ctx context.Context, tenantId int64, accountId int64, profileId int64) error {
	if tenantId <= 0 || accountId <= 0 || profileId <= 0 {
		return nil
	}
	_, err := g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("profile_id", profileId).
		WhereNull("deleted_at").
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
