package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/dao"
)

func (s *sSysPublish) syncProfilePublishState(ctx context.Context, profileId int64, status int, visibility string, publishedAt *gtime.Time) (bool, error) {
	if profileId <= 0 {
		return false, nil
	}
	columns := dao.ContentProfile.Columns()
	now := gtime.Now()
	data := g.Map{
		columns.Status:     status,
		columns.Visibility: visibility,
		columns.UpdatedAt:  now,
	}
	if status == 1 {
		if publishedAt == nil {
			publishedAt = now
		}
		data[columns.PublishedAt] = publishedAt
	}
	result, err := dao.ContentProfile.Ctx(ctx).
		Where(columns.Id, profileId).
		WhereNot(columns.Status, status).
		Data(data).
		Update()
	if err != nil {
		return false, gerror.Wrap(err, "同步资料状态失败")
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}
