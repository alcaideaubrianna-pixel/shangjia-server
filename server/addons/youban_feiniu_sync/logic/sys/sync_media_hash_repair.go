package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func needMediaPerceptualHashRepair(ctx context.Context, profileId int64, taskId int64) (bool, error) {
	if profileId <= 0 || taskId <= 0 {
		return true, nil
	}
	count, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileId).
		Where("task_id", taskId).
		WhereNull("deleted_at").
		Where("perceptual_hash", "").
		Where("(file_url LIKE ? OR poster_url LIKE ?)", "http://%", "https://%").
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查媒体感知哈希状态失败")
	}
	return count > 0, nil
}
