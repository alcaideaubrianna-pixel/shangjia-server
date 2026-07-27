package fix

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const publishProfileMediaBackfillBatchSize = 200

// BackfillYoubanPublishProfileMedia normalizes media ownership from profile state.
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
		state, err := tx.Model("hg_youban_publish_profile_state").Ctx(ctx).
			Fields("tenant_id,account_id").Where("profile_id", profileId).WhereNull("deleted_at").One()
		if err != nil {
			return gerror.Wrap(err, "读取资料归属失败")
		}
		if state.IsEmpty() {
			return nil
		}
		_, err = tx.Model("hg_youban_publish_media").Ctx(ctx).
			Where("profile_id", profileId).WhereNull("deleted_at").
			Data(g.Map{"tenant_id": state["tenant_id"].Int64(), "merchant_id": state["tenant_id"].Int64(), "account_id": state["account_id"].Int64(), "task_id": nil, "updated_at": gtime.Now()}).Update()
		if err != nil {
			return gerror.Wrap(err, "归一化资料媒体归属失败")
		}
		return nil
	})
}
