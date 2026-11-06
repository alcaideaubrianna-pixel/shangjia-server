package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/library/contexts"
)

type profileChannelMappingRow struct {
	ChannelId int64 `orm:"channel_id"`
}

func profileChannelIds(ctx context.Context, tenantId, profileId int64) ([]int64, error) {
	if tenantId <= 0 || profileId <= 0 {
		return []int64{}, nil
	}
	mod := g.DB().Model(publishProfileChannelTable).Safe().Ctx(ctx).
		Fields("channel_id").
		Where("tenant_id", tenantId).
		Where("profile_id", profileId).
		Where("is_manual", 1).
		WhereNull("deleted_at")
	var rows []profileChannelMappingRow
	if err := mod.OrderAsc("channel_id").Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取资料推送频道配置失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.ChannelId > 0 {
			ids = append(ids, row.ChannelId)
		}
	}
	return uniqueIds(ids), nil
}

// activePublishChannelIds 过滤已删除、停用或非上架方向的频道映射。
func activePublishChannelIds(ctx context.Context, tenantId int64, channelIds []int64) ([]int64, error) {
	channelIds = uniqueIds(channelIds)
	if tenantId <= 0 || len(channelIds) == 0 {
		return []int64{}, nil
	}
	var rows []profileChannelMappingRow
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id AS channel_id").
		Where("tenant_id", tenantId).
		WhereIn("id", channelIds).
		Where("status", 1).
		Where("publish_direction", "up").
		WhereNull("deleted_at").
		OrderAsc("id").
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "过滤有效推送频道失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.ChannelId > 0 {
			ids = append(ids, row.ChannelId)
		}
	}
	return uniqueIds(ids), nil
}

func replaceProfileChannelMappings(ctx context.Context, tx gdb.TX, tenantId, accountId, profileId int64, channelIds []int64) error {
	if tenantId <= 0 || accountId <= 0 || profileId <= 0 {
		return nil
	}
	if _, err := tx.Model(publishProfileChannelTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("profile_id", profileId).
		Unscoped().Delete(); err != nil {
		return gerror.Wrap(err, "清理资料推送频道配置失败")
	}
	channelIds = uniqueIds(channelIds)
	if len(channelIds) == 0 {
		return nil
	}
	now := gtime.Now()
	data := make([]g.Map, 0, len(channelIds))
	for _, channelId := range channelIds {
		if channelId <= 0 {
			continue
		}
		data = append(data, g.Map{
			"tenant_id": tenantId, "account_id": accountId, "profile_id": profileId,
			"channel_id": channelId, "is_manual": 1, "created_by": contexts.GetUserId(ctx),
			"updated_by": contexts.GetUserId(ctx), "created_at": now, "updated_at": now,
		})
	}
	if len(data) == 0 {
		return nil
	}
	if _, err := tx.Model(publishProfileChannelTable).Safe().Ctx(ctx).Data(data).Insert(); err != nil {
		return gerror.Wrap(err, "保存资料推送频道配置失败")
	}
	return nil
}

func (s *sSysPublish) profileChannelIdsOrDefaults(ctx context.Context, tenantId, accountId, profileId int64) ([]int64, error) {
	ids, err := profileChannelIds(ctx, tenantId, profileId)
	if err != nil {
		return nil, err
	}
	if len(ids) > 0 {
		ids, err = activePublishChannelIds(ctx, tenantId, ids)
		if err != nil {
			return nil, err
		}
		if len(ids) > 0 {
			return ids, nil
		}
	}
	return s.defaultSelectedPublishChannelIds(ctx, tenantId)
}
