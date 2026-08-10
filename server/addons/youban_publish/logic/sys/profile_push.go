package sys

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
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
	if err := ensurePublishChannelColumns(ctx); err != nil {
		return nil, err
	}
	var channels []*sysin.ProfilePushChannelModel
	mod := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id AS channel_id,channel_title,channel_username,status,cycle_publish_enabled,cycle_publish_days").
		Where("tenant_id", profile.TenantId).
		Where("publish_direction", "up").
		WhereNull("deleted_at")
	mod = mod.WhereIn("id", channelIds)
	if err := mod.
		OrderAsc("id").
		Scan(&channels); err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return channels, nil
	}
	type firstPushRow struct {
		ChannelId   int64       `orm:"channel_id"`
		FirstPushAt *gtime.Time `orm:"first_push_at"`
	}
	var firstPushes []*firstPushRow
	if err := g.DB().Model(publishSuccessRecordTable+" r").Safe().Ctx(ctx).
		InnerJoin(publishTgJobTable+" j", "j.id=r.job_id AND j.sent_at IS NOT NULL").
		Fields("r.channel_id,MIN(j.sent_at) AS first_push_at").
		Where("r.tenant_id", profile.TenantId).
		Where("r.profile_id", profile.Id).
		Where("r.status", "success").
		WhereIn("r.channel_id", channelIds).
		Group("r.channel_id").
		Scan(&firstPushes); err != nil {
		return nil, err
	}
	firstPushByChannel := make(map[int64]*gtime.Time, len(firstPushes))
	for _, item := range firstPushes {
		if item != nil {
			firstPushByChannel[item.ChannelId] = item.FirstPushAt
		}
	}
	type cycleDueRow struct {
		ChannelId  int64       `orm:"channel_id"`
		CycleDueAt *gtime.Time `orm:"cycle_due_at"`
	}
	var cycleDueRows []*cycleDueRow
	if err := g.DB().Model(publishChannelProfileTable+" cp").Safe().Ctx(ctx).
		InnerJoin(publishTgJobTable+" j", "j.id=cp.last_job_id AND j.status IN ('sent','superseded') AND j.cycle_enabled=1").
		Fields("cp.channel_id,j.next_cycle_at AS cycle_due_at").
		Where("cp.profile_id", profile.Id).
		Where("cp.status", "active").
		WhereIn("cp.channel_id", channelIds).
		Scan(&cycleDueRows); err != nil {
		return nil, err
	}
	cycleDueByChannel := make(map[int64]*gtime.Time, len(cycleDueRows))
	for _, item := range cycleDueRows {
		if item != nil {
			cycleDueByChannel[item.ChannelId] = item.CycleDueAt
		}
	}
	for _, channel := range channels {
		if channel == nil {
			continue
		}
		channel.FirstPushAt = firstPushByChannel[channel.ChannelId]
		if channel.CyclePublishEnabled != 1 {
			channel.NextPushAt = nil
		} else {
			channel.NextPushAt = cycleDueByChannel[channel.ChannelId]
		}
	}
	return channels, nil
}
