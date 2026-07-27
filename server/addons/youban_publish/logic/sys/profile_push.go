package sys

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

type profilePushJob struct {
	ChannelId           int64       `orm:"channel_id" json:"channelId"`
	CyclePublishEnabled int         `orm:"cycle_enabled" json:"cyclePublishEnabled"`
	CycleDays           int         `orm:"cycle_days" json:"cycleDays"`
	CyclePublishTime    string      `orm:"cycle_publish_time" json:"cyclePublishTime"`
	NextCycleAt         *gtime.Time `orm:"next_cycle_at" json:"nextCycleAt"`
}

type profilePushPlan struct {
	ChannelId       int64       `orm:"channel_id" json:"channelId"`
	IntervalSeconds int         `orm:"interval_seconds" json:"intervalSeconds"`
	PublishTime     string      `orm:"publish_time" json:"publishTime"`
	NextRunAt       *gtime.Time `orm:"next_run_at" json:"nextRunAt"`
}

func (s *sSysPublish) profilePushChannels(ctx context.Context, profile *sysin.ProfileModel) ([]*sysin.ProfilePushChannelModel, error) {
	if profile == nil {
		return []*sysin.ProfilePushChannelModel{}, nil
	}
	channelIds := decodeInt64JSON(profile.ChannelIdJson)
	if len(channelIds) == 0 {
		return []*sysin.ProfilePushChannelModel{}, nil
	}

	var channels []*sysin.ProfilePushChannelModel
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id AS channel_id,channel_title,channel_username,status,cycle_publish_enabled,cycle_publish_days").
		Where("tenant_id", profile.TenantId).
		WhereIn("id", channelIds).
		WhereNull("deleted_at").
		OrderAsc("id").
		Scan(&channels); err != nil {
		return nil, err
	}
	if len(channels) == 0 || profile.TaskId <= 0 {
		return channels, nil
	}

	var jobs []*profilePushJob
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("channel_id,cycle_enabled,cycle_days,cycle_publish_time,next_cycle_at").
		Where("tenant_id", profile.TenantId).
		Where("task_id", profile.TaskId).
		Where("profile_id", profile.Id).
		WhereIn("channel_id", channelIds).
		WhereNotIn("status", []string{"superseded", "deleted"}).
		OrderDesc("id").
		Scan(&jobs); err != nil {
		return nil, err
	}

	jobByChannel := make(map[int64]*profilePushJob, len(jobs))
	for _, job := range jobs {
		if job != nil {
			if _, exists := jobByChannel[job.ChannelId]; !exists {
				jobByChannel[job.ChannelId] = job
			}
		}
	}

	var plans []*profilePushPlan
	if err := g.DB().Model(publishCyclePlanTable).Safe().Ctx(ctx).
		Fields("channel_id,interval_seconds,publish_time,next_run_at").
		Where("tenant_id", profile.TenantId).
		Where("profile_id", profile.Id).
		Where("task_id", profile.TaskId).
		Where("enabled", 1).
		Where("status", cyclePlanStatusActive).
		WhereNull("deleted_at").
		WhereIn("channel_id", channelIds).
		OrderDesc("id").
		Scan(&plans); err != nil {
		return nil, err
	}
	planByChannel := make(map[int64]*profilePushPlan, len(plans))
	for _, plan := range plans {
		if plan != nil {
			if _, exists := planByChannel[plan.ChannelId]; !exists {
				planByChannel[plan.ChannelId] = plan
			}
		}
	}

	type firstPushRow struct {
		ChannelId   int64       `orm:"channel_id" json:"channelId"`
		FirstPushAt *gtime.Time `orm:"first_push_at" json:"firstPushAt"`
	}
	var firstPushes []*firstPushRow
	if err := g.DB().Model(publishTgJobLogTable+" l").Safe().Ctx(ctx).
		Fields("j.channel_id,MIN(l.created_at) AS first_push_at").
		InnerJoin(publishTgJobTable+" j", "j.id=l.job_id").
		Where("l.tenant_id", profile.TenantId).
		Where("l.task_id", profile.TaskId).
		Where("l.profile_id", profile.Id).
		WhereIn("l.status", []string{"sent", "success"}).
		WhereIn("j.channel_id", channelIds).
		Group("j.channel_id").
		Scan(&firstPushes); err != nil {
		return nil, err
	}
	firstPushByChannel := make(map[int64]*gtime.Time, len(firstPushes))
	for _, item := range firstPushes {
		if item != nil {
			firstPushByChannel[item.ChannelId] = item.FirstPushAt
		}
	}

	for _, channel := range channels {
		if channel == nil {
			continue
		}
		channel.FirstPushAt = firstPushByChannel[channel.ChannelId]
		if job := jobByChannel[channel.ChannelId]; job != nil {
			channel.CyclePublishEnabled = job.CyclePublishEnabled
			if job.CycleDays > 0 {
				channel.CyclePublishDays = job.CycleDays
			}
			channel.NextPushAt = s.profilePushNextRunAt(ctx, job.NextCycleAt, job.CycleDays, job.CyclePublishTime)
		}
		if plan := planByChannel[channel.ChannelId]; plan != nil {
			channel.CyclePublishEnabled = 1
			channel.NextPushAt = s.profilePushNextRunAt(ctx, plan.NextRunAt, plan.IntervalSeconds, plan.PublishTime)
		}
	}
	return channels, nil
}

func (s *sSysPublish) profilePushNextRunAt(ctx context.Context, nextRunAt *gtime.Time, intervalSeconds int, publishTime string) *gtime.Time {
	if nextRunAt == nil {
		if intervalSeconds <= 0 {
			return nil
		}
		return s.nextCycleRunAt(ctx, cyclePlanRecord{
			IntervalSeconds: intervalSeconds,
			PublishTime:     publishTime,
		}, gtime.Now())
	}
	now := gtime.Now()
	if nextRunAt.After(now) {
		return nextRunAt
	}
	return s.nextCycleRunAt(ctx, cyclePlanRecord{
		IntervalSeconds: intervalSeconds,
		PublishTime:     publishTime,
	}, now)
}
