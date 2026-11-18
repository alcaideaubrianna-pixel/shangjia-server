package sys

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/utility/dbtime"
)

func (s *sSysPublish) profilePushChannels(ctx context.Context, profile *sysin.ProfileModel) ([]*sysin.ProfilePushChannelModel, error) {
	if profile == nil {
		return []*sysin.ProfilePushChannelModel{}, nil
	}
	channelIds, err := s.profileChannelIdsOrDefaults(ctx, profile.TenantId, profile.AccountId, profile.Id)
	if err != nil {
		return nil, err
	}
	if len(channelIds) == 0 {
		return []*sysin.ProfilePushChannelModel{}, nil
	}
	var schedules []*profilePushChannelSchedule
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id AS channel_id,channel_title,channel_username,status,cycle_publish_enabled,cycle_publish_mode,cycle_publish_days,cycle_publish_time,cycle_batch_size,cycle_batch_time,cycle_active_run_id,cycle_next_run_at AS next_batch_at").
		Where("tenant_id", profile.TenantId).
		Where("publish_direction", "up").WhereNull("deleted_at").
		WhereIn("id", channelIds).OrderAsc("id").Scan(&schedules); err != nil {
		return nil, err
	}
	channels := make([]*sysin.ProfilePushChannelModel, 0, len(schedules))
	if len(schedules) == 0 {
		return channels, nil
	}
	type pushHistoryRow struct {
		ChannelId   int64       `orm:"channel_id"`
		FirstPushAt *gtime.Time `orm:"first_push_at"`
		LastPushAt  *gtime.Time `orm:"last_push_at"`
	}
	var history []*pushHistoryRow
	if err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		Fields("j.channel_id,MIN(j.sent_at) AS first_push_at,MAX(j.sent_at) AS last_push_at").
		Where("j.status", telegramJobStatusSent).WhereNotNull("j.sent_at").
		Where("j.tenant_id", profile.TenantId).Where("j.profile_id", profile.Id).
		WhereIn("j.channel_id", channelIds).Group("j.channel_id").Scan(&history); err != nil {
		return nil, err
	}
	historyByChannel := make(map[int64]*pushHistoryRow, len(history))
	for _, item := range history {
		if item != nil {
			item.FirstPushAt = dbtime.FromTimestamp(item.FirstPushAt)
			item.LastPushAt = dbtime.FromTimestamp(item.LastPushAt)
			historyByChannel[item.ChannelId] = item
		}
	}
	type cycleDueRow struct {
		ChannelId  int64       `orm:"channel_id"`
		CycleDueAt *gtime.Time `orm:"cycle_due_at"`
	}
	var cycleDueRows []*cycleDueRow
	// Use the same plan projection as the scheduler, rather than the historical
	// Job's cycle_enabled flag (which is deliberately off for batch channels).
	if err := g.DB().Model(publishChannelProfileTable+" cp").Safe().Ctx(ctx).
		Fields("cp.channel_id,cp.next_cycle_at AS cycle_due_at").
		Where("cp.tenant_id", profile.TenantId).Where("cp.account_id", profile.AccountId).
		Where("cp.profile_id", profile.Id).Where("cp.status", "active").
		WhereIn("cp.channel_id", channelIds).Scan(&cycleDueRows); err != nil {
		return nil, err
	}
	cycleDueByChannel := make(map[int64]*gtime.Time, len(cycleDueRows))
	for _, item := range cycleDueRows {
		if item != nil {
			cycleDueByChannel[item.ChannelId] = dbtime.FromTimestamp(item.CycleDueAt)
		}
	}
	estimateCtx, cancel := context.WithTimeout(ctx, profilePushEstimateTimeout)
	defer cancel()
	now := gtime.New(gtime.Now().Time.In(dbtime.Location()))
	for _, schedule := range schedules {
		if schedule == nil {
			continue
		}
		channel := &schedule.ProfilePushChannelModel
		channels = append(channels, channel)
		channel.CyclePublishMode = normalizedProfilePushCycleMode(channel.CyclePublishMode)
		channel.CyclePublishDays = s.effectiveChannelCycleDays(ctx, profile.TenantId, channel.CyclePublishDays)
		var lastPushAt *gtime.Time
		if item := historyByChannel[channel.ChannelId]; item != nil {
			channel.FirstPushAt, lastPushAt = item.FirstPushAt, item.LastPushAt
		}
		if channel.CyclePublishEnabled != 1 || channel.Status != 1 || profile.Status != 1 || profile.Visibility != consts.ContentVisibilityPublic {
			channel.NextBatchAt = nil
			continue
		}
		if channel.CyclePublishMode == "batch" {
			channel.NextBatchAt = profilePushNextBatchAt(channel.NextBatchAt, now)
			channel.NextPushAt, err = s.estimateProfileBatchPushAt(estimateCtx, profile, schedule, now)
			channel.NextPushAtEstimated = channel.NextPushAt != nil
			if err != nil {
				g.Log().Warning(ctx, "资料批次循环时间预估失败", g.Map{
					"event": "profile_push_estimate_failed", "tenantId": profile.TenantId,
					"accountId": profile.AccountId, "profileId": profile.Id,
					"channelId": channel.ChannelId, "error": err.Error(),
				})
				return nil, err
			}
			continue
		}
		channel.NextBatchAt = nil
		if channel.CyclePublishMode == "time" {
			channel.NextPushAt = cycleDueByChannel[channel.ChannelId]
			if channel.NextPushAt == nil && lastPushAt != nil {
				channel.NextPushAt = nextProfileCycleAt(ctx, channel.CyclePublishDays, channel.CyclePublishTime, lastPushAt, profile.Id, channel.ChannelId)
				channel.NextPushAtEstimated = channel.NextPushAt != nil
			}
		}
	}
	return channels, nil
}
