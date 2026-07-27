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
	channelIds := decodeInt64JSON(profile.ChannelIdJson)
	if len(channelIds) == 0 {
		return []*sysin.ProfilePushChannelModel{}, nil
	}
	if err := ensurePublishChannelColumns(ctx); err != nil {
		return nil, err
	}
	var channels []*sysin.ProfilePushChannelModel
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id AS channel_id,channel_title,channel_username,status,cycle_publish_enabled,cycle_publish_days,cycle_next_run_at AS next_push_at").
		Where("tenant_id", profile.TenantId).
		WhereIn("id", channelIds).
		WhereNull("deleted_at").
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
	if err := g.DB().Model(publishSuccessRecordTable).Safe().Ctx(ctx).
		Fields("channel_id,MIN(created_at) AS first_push_at").
		Where("tenant_id", profile.TenantId).
		Where("profile_id", profile.Id).
		Where("status", "success").
		WhereIn("channel_id", channelIds).
		Group("channel_id").
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
		if channel.CyclePublishEnabled != 1 {
			channel.NextPushAt = nil
		}
	}
	return channels, nil
}
