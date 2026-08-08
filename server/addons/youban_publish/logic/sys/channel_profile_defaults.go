package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

type profileChannelStateRow struct {
	Id            int64  `orm:"id"`
	ChannelIdJSON string `orm:"channel_id_json"`
}

func (s *sSysPublish) syncDefaultSelectedChannelProfiles(ctx context.Context, tenantId int64, profileIds ...[]int64) error {
	if tenantId <= 0 {
		return nil
	}
	defaultChannelIds, err := s.defaultSelectedPublishChannelIds(ctx, tenantId)
	if err != nil {
		return err
	}
	if len(defaultChannelIds) == 0 {
		return nil
	}

	mod := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Fields("id,channel_id_json").
		Where("tenant_id", tenantId).
		WhereNull("deleted_at")
	if len(profileIds) > 0 && len(profileIds[0]) > 0 {
		mod = mod.WhereIn("profile_id", uniqueIds(profileIds[0]))
	}
	var rows []profileChannelStateRow
	if err = mod.Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取资料默认频道配置失败")
	}
	for _, row := range rows {
		current := decodeInt64JSON(row.ChannelIdJSON)
		merged := mergeDefaultChannelIds(current, defaultChannelIds)
		if sameInt64Slice(current, merged) {
			continue
		}
		channelJSON, encodeErr := encodeBotIds(merged)
		if encodeErr != nil {
			return encodeErr
		}
		if _, err = g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
			Where("id", row.Id).
			Data(g.Map{"channel_id_json": channelJSON, "updated_at": gtime.Now()}).Update(); err != nil {
			return gerror.Wrap(err, "同步资料默认频道配置失败")
		}
	}
	return nil
}

func (s *sSysPublish) defaultSelectedPublishChannelIds(ctx context.Context, tenantId int64) ([]int64, error) {
	var rows []struct {
		Id int64 `json:"id"`
	}
	err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id").
		Where("tenant_id", tenantId).
		Where("publish_direction", "up").
		Where("status", 1).
		Where("is_default_selected", 1).
		WhereNull("deleted_at").
		OrderAsc("id").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取默认推送频道失败")
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.Id > 0 {
			ids = append(ids, row.Id)
		}
	}
	return uniqueIds(ids), nil
}

func mergeDefaultChannelIds(current, defaults []int64) []int64 {
	merged := append([]int64(nil), current...)
	seen := make(map[int64]struct{}, len(merged)+len(defaults))
	for _, id := range merged {
		if id > 0 {
			seen[id] = struct{}{}
		}
	}
	for _, id := range defaults {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		merged = append(merged, id)
		seen[id] = struct{}{}
	}
	return uniqueIds(merged)
}
