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
	var indexedChannels []struct {
		ChannelId int64 `orm:"channel_id"`
	}
	if err := g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).
		Fields("channel_id").
		Where("tenant_id", profile.TenantId).
		Where("profile_id", profile.Id).
		Where("status", "active").
		Scan(&indexedChannels); err != nil {
		return nil, err
	}
	indexedChannelIds := make([]int64, 0, len(indexedChannels))
	for _, item := range indexedChannels {
		indexedChannelIds = append(indexedChannelIds, item.ChannelId)
	}
	channelIds = uniqueIds(append(channelIds, indexedChannelIds...))
	if len(channelIds) == 0 {
		return []*sysin.ProfilePushChannelModel{}, nil
	}
	if err := ensurePublishChannelColumns(ctx); err != nil {
		return nil, err
	}
	var channelRefs []struct {
		Id           int64  `orm:"id"`
		TargetChatId string `orm:"target_chat_id"`
	}
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,target_chat_id").
		Where("tenant_id", profile.TenantId).
		WhereIn("id", channelIds).
		Scan(&channelRefs); err != nil {
		return nil, err
	}
	targetChatIds := make([]string, 0, len(channelRefs))
	for _, item := range channelRefs {
		if item.TargetChatId != "" {
			targetChatIds = append(targetChatIds, item.TargetChatId)
		}
	}
	var channels []*sysin.ProfilePushChannelModel
	mod := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id AS channel_id,channel_title,channel_username,status,cycle_publish_enabled,cycle_publish_days,cycle_next_run_at AS next_push_at").
		Where("tenant_id", profile.TenantId).
		Where("publish_direction", "up").
		WhereNull("deleted_at")
	if len(targetChatIds) > 0 {
		mod = mod.Where("(id IN(?) OR target_chat_id IN(?))", channelIds, uniqueStrings(targetChatIds))
	} else {
		mod = mod.WhereIn("id", channelIds)
	}
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
