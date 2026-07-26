package fix

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/service"
)

const noteIndexBackfillBatchSize = 200

// BackfillYoubanPublishNoteIndex rebuilds the note read model from task/profile
// source data. It is intentionally a standalone maintenance command so normal
// plugin upgrades never perform a large write transaction.
func BackfillYoubanPublishNoteIndex(ctx context.Context) error {
	lastProfileId := int64(0)
	processed := 0
	for {
		var rows []struct {
			ProfileId int64 `orm:"profile_id"`
		}
		if err := g.DB().Model("hg_youban_publish_task").Safe().Ctx(ctx).
			Fields("profile_id").WhereGT("profile_id", lastProfileId).
			Where("profile_id > 0").Distinct().OrderAsc("profile_id").
			Limit(noteIndexBackfillBatchSize).Scan(&rows); err != nil {
			return gerror.Wrap(err, "读取资料索引回填范围失败")
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			if err := service.SysPublish().RefreshNoteIndex(ctx, row.ProfileId); err != nil {
				return err
			}
			processed++
			lastProfileId = row.ProfileId
		}
		g.Log().Infof(ctx, "上架资料索引回填进度：lastProfileId=%d processed=%d", lastProfileId, processed)
	}
	g.Log().Infof(ctx, "上架资料索引回填完成：processed=%d", processed)
	return nil
}
