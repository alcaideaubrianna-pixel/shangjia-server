package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) AIOpsRebuildProfileMedia(ctx context.Context, profileIds []int64, dryRun bool) (*sysin.CollectProfileMediaRebuildResult, error) {
	profileIds = uniqueIds(profileIds)
	if len(profileIds) == 0 || len(profileIds) > 20 {
		return nil, gerror.New("单次只能处理1到20条资料")
	}
	result, err := RebuildCollectProfileMedia(ctx, CollectProfileMediaRebuildOptions{ProfileIDs: profileIds, Limit: len(profileIds), DryRun: dryRun})
	g.Log().Info(ctx, "AI运维资料媒体操作", g.Map{"profileIds": profileIds, "dryRun": dryRun, "result": result, "error": err})
	return result, err
}

func (s *sSysPublish) AIOpsRepublishProfiles(ctx context.Context, in *sysin.ProfileStatusInp) (*sysin.ProfileStatusModel, error) {
	if in == nil {
		return nil, gerror.New("资料参数不能为空")
	}
	ids := uniqueIds(in.Ids)
	if len(ids) == 0 || len(ids) > 20 {
		return nil, gerror.New("单次只能重新上架1到20条资料")
	}
	for _, profileId := range ids {
		profile, err := s.profilePublishSource(ctx, profileId, 0, 0, false)
		if err != nil {
			return nil, err
		}
		if profile["status"].Int() != 1 {
			if _, err = s.updateProfileStatus(ctx, &sysin.ProfileStatusInp{Ids: []int64{profileId}, Status: 1}, 0, 0); err != nil {
				return nil, err
			}
		}
		complete, err := collectProfileMediaComplete(ctx, profileId)
		if err != nil {
			return nil, err
		}
		if !complete {
			return nil, gerror.Newf("资料%d媒体尚未恢复完整，禁止重新上架", profileId)
		}
		if err = s.submitProfilePublish(ctx, profileId, profile["tenant_id"].Int64(), profile["account_id"].Int64(), 0, newTelegramOperationNo("ai-republish", profileId), nil, false); err != nil {
			return nil, gerror.Wrapf(err, "资料%d提交TG发布任务失败", profileId)
		}
	}
	result := &sysin.ProfileStatusModel{Message: "资料已提交TG发布任务"}
	g.Log().Info(ctx, "AI运维资料重新上架", g.Map{"profileIds": ids, "result": result})
	return result, nil
}

func collectProfileMediaComplete(ctx context.Context, profileId int64) (bool, error) {
	profile, err := g.DB().Model("hg_content_profile p").Safe().Ctx(ctx).
		Fields("p.source_type,COUNT(DISTINCT m.id) AS media_count").
		LeftJoin("hg_youban_publish_media m", "m.profile_id=p.id AND m.deleted_at IS NULL").
		Where("p.id", profileId).WhereNull("p.deleted_at").
		Group("p.id,p.source_type").One()
	if err != nil {
		return false, gerror.Wrap(err, "读取资料媒体信息失败")
	}
	if profile.IsEmpty() {
		return false, nil
	}
	if profile["source_type"].String() != "youban_collect" {
		complete := profile["media_count"].Int() > 0
		g.Log().Infof(ctx, "非采集资料媒体完整性检查 profileId:%d sourceType:%s media:%d complete:%t", profileId, profile["source_type"].String(), profile["media_count"].Int(), complete)
		return complete, nil
	}
	// collect_dispatch has no deleted_at column. Safe() would append a
	// non-existent d.deleted_at predicate when the table is aliased.
	row, err := g.DB().Model("hg_youban_publish_collect_dispatch d").Ctx(ctx).
		Fields("COALESCE(MAX(CASE WHEN e.material_role='display' THEN e.media_count END),MAX(e.media_count)) AS expected_media,COUNT(DISTINCT m.id) AS profile_media").
		InnerJoin("hg_youban_publish_collect_event e", "e.id=d.event_id").
		LeftJoin("hg_youban_publish_media m", "m.profile_id=d.profile_id AND m.deleted_at IS NULL").
		Where("d.profile_id", profileId).
		One()
	if err != nil {
		return false, gerror.Wrap(err, "检查资料媒体完整性失败")
	}
	if row.IsEmpty() {
		g.Log().Warningf(ctx, "资料媒体完整性检查无采集记录 profileId:%d", profileId)
		return false, nil
	}
	expectedMedia := row["expected_media"].Int()
	profileMedia := row["profile_media"].Int()
	complete := expectedMedia > 0 && profileMedia >= expectedMedia
	g.Log().Infof(ctx, "资料媒体完整性检查 profileId:%d expected:%d actual:%d complete:%t", profileId, expectedMedia, profileMedia, complete)
	return complete, nil
}
