package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

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
