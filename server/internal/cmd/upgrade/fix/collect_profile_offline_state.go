package fix

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/service"
)

const collectProfileOfflineStateBatchSize = 200

func NormalizeYoubanPublishCollectProfileOfflineState(ctx context.Context) error {
	lastProfileId := int64(0)
	processed := 0
	for {
		profileIds, err := collectProfileZeroStatusIDs(ctx, lastProfileId)
		if err != nil {
			return err
		}
		if len(profileIds) == 0 {
			break
		}
		lastProfileId = profileIds[len(profileIds)-1]
		if _, err = g.DB().Model("hg_content_profile").Safe().Ctx(ctx).
			WhereIn("id", profileIds).
			Where("status", 0).
			Data(g.Map{
				"status":     2,
				"visibility": "private",
				"updated_at": gtime.Now(),
			}).Update(); err != nil {
			return gerror.Wrap(err, "统一采集资料未上架状态失败")
		}
		for _, profileId := range profileIds {
			if err = service.SysPublish().RefreshNoteIndex(ctx, profileId); err != nil {
				return err
			}
			processed++
		}
		g.Log().Infof(ctx, "采集资料未上架状态修复进度：lastProfileId=%d processed=%d", lastProfileId, processed)
	}
	g.Log().Infof(ctx, "采集资料未上架状态修复完成：processed=%d", processed)
	return nil
}

func collectProfileZeroStatusIDs(ctx context.Context, lastProfileId int64) ([]int64, error) {
	var rows []struct {
		ProfileId int64 `orm:"profile_id"`
	}
	err := g.DB().Model("hg_content_profile p").Safe().Ctx(ctx).
		InnerJoin("hg_youban_publish_profile_state ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		Fields("p.id AS profile_id").
		WhereGT("p.id", lastProfileId).
		Where("p.source_type", "youban_collect").
		Where("p.status", 0).
		WhereNull("p.deleted_at").
		Distinct().
		OrderAsc("p.id").
		Limit(collectProfileOfflineStateBatchSize).
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取待统一状态的采集资料失败")
	}
	profileIds := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.ProfileId > 0 {
			profileIds = append(profileIds, row.ProfileId)
		}
	}
	return profileIds, nil
}
