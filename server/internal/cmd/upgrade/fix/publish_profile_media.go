package fix

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const publishProfileMediaBackfillBatchSize = 200

// BackfillYoubanPublishProfileMedia creates the profile-owned media set.
// Collection task media remains unchanged.
func BackfillYoubanPublishProfileMedia(ctx context.Context) error {
	lastProfileId := int64(0)
	processed := 0
	for {
		var states []struct {
			ProfileId int64 `orm:"profile_id"`
		}
		if err := g.DB().Model("hg_youban_publish_profile_state").Safe().Ctx(ctx).
			Fields("profile_id").WhereGT("profile_id", lastProfileId).
			WhereNull("deleted_at").OrderAsc("profile_id").
			Limit(publishProfileMediaBackfillBatchSize).Scan(&states); err != nil {
			return gerror.Wrap(err, "读取资料媒体迁移范围失败")
		}
		if len(states) == 0 {
			break
		}
		for _, state := range states {
			if err := backfillOnePublishProfileMedia(ctx, state.ProfileId); err != nil {
				return err
			}
			processed++
			lastProfileId = state.ProfileId
		}
		g.Log().Infof(ctx, "资料当前媒体回填进度：lastProfileId=%d processed=%d", lastProfileId, processed)
	}
	g.Log().Infof(ctx, "资料当前媒体回填完成：processed=%d", processed)
	return nil
}

func backfillOnePublishProfileMedia(ctx context.Context, profileId int64) error {
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		count, err := tx.Model("hg_youban_publish_media").Ctx(ctx).
			Where("profile_id", profileId).WhereNull("task_id").WhereNull("deleted_at").Count()
		if err != nil || count > 0 {
			return err
		}
		latestTaskId, err := tx.Model("hg_youban_publish_task t").Ctx(ctx).
			Where("t.profile_id", profileId).
			WhereNull("t.deleted_at").
			Where("EXISTS (SELECT 1 FROM hg_youban_publish_media tm WHERE tm.task_id=t.id AND tm.deleted_at IS NULL)").
			OrderDesc("t.id").
			Fields("t.id").
			Value()
		if err != nil {
			return gerror.Wrap(err, "读取资料最新发布事件失败")
		}
		if latestTaskId.Int64() <= 0 {
			return nil
		}
		var rows []gdb.Record
		if err = tx.Model("hg_youban_publish_media").Ctx(ctx).
			Where("task_id", latestTaskId.Int64()).Where("profile_id", profileId).
			WhereNull("deleted_at").OrderAsc("sort_index").OrderAsc("id").Scan(&rows); err != nil {
			return gerror.Wrap(err, "读取发布事件媒体失败")
		}
		now := gtime.Now()
		for _, row := range rows {
			data := row.Map()
			delete(data, "id")
			data["task_id"] = nil
			data["tg_file_id"] = ""
			data["tg_thumb_file_id"] = ""
			data["tg_cache_asset_hash"] = ""
			data["tg_cache_status"] = "invalid"
			data["created_at"] = now
			data["updated_at"] = now
			data["deleted_at"] = nil
			if _, err = tx.Model("hg_youban_publish_media").Ctx(ctx).Data(data).Insert(); err != nil {
				return gerror.Wrap(err, "创建资料当前媒体失败")
			}
		}
		return nil
	})
}
