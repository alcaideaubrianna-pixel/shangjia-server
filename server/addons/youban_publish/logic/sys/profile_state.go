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
	}
	if status == 1 {
		if publishedAt == nil {
			publishedAt = now
		}
		data[columns.PublishedAt] = publishedAt
	}
	condition := "(" + columns.Status + " <> ? OR " + columns.Visibility + " <> ?)"
	args := []interface{}{status, visibility}
	if status == 1 {
		condition = "(" + columns.Status + " <> ? OR " + columns.Visibility + " <> ? OR " + columns.PublishedAt + " IS NULL)"
	}
	result, err := dao.ContentProfile.Ctx(ctx).
		Where(columns.Id, profileId).
		Where(condition, args...).
		Data(data).
		Update()
	if err != nil {
		return false, gerror.Wrap(err, "同步资料状态失败")
	}
	affected, _ := result.RowsAffected()
	if status != 1 {
		if err = s.cancelProfilePublishOperation(ctx, profileId); err != nil {
			return false, err
		}
	}
	return affected > 0, nil
}
