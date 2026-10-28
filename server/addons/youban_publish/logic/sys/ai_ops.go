package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

// AIOpsRecoverIgnoredCollectVerifications re-enters only verification videos
// that the realtime classifier previously marked as unmatched. Pair selection
// still goes through the canonical runtime matcher before any queue is written.
func (s *sSysPublish) AIOpsRecoverIgnoredCollectVerifications(ctx context.Context, sourceID int64, limit int, dryRun bool) (int, []int64, error) {
	if sourceID <= 0 || limit <= 0 || limit > 5000 {
		return 0, nil, gerror.New("验证恢复范围不合法")
	}
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("source_id", sourceID).
		Where("status", sysin.CollectEventStatusIgnored).
		Where("material_role", collectMaterialRoleVerify).
		Where("error_message", collectMaterialVerifyUnmatchedMessage).
		OrderAsc("id").Limit(limit + 1).All()
	if err != nil {
		return 0, nil, gerror.Wrap(err, "读取待恢复验证事件失败")
	}
	if len(rows) > limit {
		return 0, nil, gerror.Newf("待恢复验证事件超过安全上限%d", limit)
	}
	processed := 0
	failed := make([]int64, 0)
	for _, verify := range rows {
		verifyID := verify["id"].Int64()
		display, findErr := s.findStoredCollectDisplayEvent(ctx, verify)
		if findErr != nil || display.IsEmpty() {
			failed = append(failed, verifyID)
			continue
		}
		canonical, canonicalErr := s.isCanonicalCollectMaterialPair(ctx, display, verify)
		if canonicalErr != nil || !canonical {
			failed = append(failed, verifyID)
			continue
		}
		if dryRun {
			processed++
			continue
		}
		paired, pairErr := s.bindCollectMaterialPair(ctx, display, verify)
		if pairErr != nil || !paired {
			failed = append(failed, verifyID)
			continue
		}
		if processErr := s.processCollectEvent(ctx, verifyID, verify["tenant_id"].Int64(), verify["account_id"].Int64()); processErr != nil && !isCollectProcessRetryError(processErr) {
			failed = append(failed, verifyID)
			continue
		}
		processed++
	}
	return processed, failed, nil
}

// AIOpsRepublishProfileChannels republishes one repaired profile to the exact
// channels that contained its previous publication. operationNo must be stable
// so rerunning an interrupted repair cannot create duplicate jobs.
func (s *sSysPublish) AIOpsRepublishProfileChannels(ctx context.Context, profileId int64, channelIds []int64, operationNo string) error {
	channelIds = uniqueIds(channelIds)
	operationNo = strings.TrimSpace(operationNo)
	if profileId <= 0 || len(channelIds) == 0 {
		return gerror.New("资料ID和原发布频道不能为空")
	}
	if !strings.HasPrefix(operationNo, "verify-misbinding-repair:") {
		return gerror.New("历史验证修复操作号不合法")
	}
	profile, err := s.profilePublishSource(ctx, profileId, 0, 0, false)
	if err != nil {
		return err
	}
	complete, err := collectProfileMediaComplete(ctx, profileId)
	if err != nil {
		return err
	}
	if !complete {
		return gerror.Newf("资料%d媒体尚未恢复完整，禁止重新上架", profileId)
	}
	// Do not call updateProfileStatus here: its online branch creates another
	// default-channel operation. The explicit operation below becomes the sole
	// publication and the normal sent projection restores the online state.
	return s.submitProfilePublish(ctx, profileId, profile["tenant_id"].Int64(), profile["account_id"].Int64(), 0, operationNo, channelIds, false)
}

// AIOpsQueueCollectVerification republishes the complete profile after its
// verification media is restored. The legacy method name is retained for API
// compatibility; it never creates a verification-only Telegram job.
func (s *sSysPublish) AIOpsQueueCollectVerification(ctx context.Context, eventIds []int64, dryRun bool) (int, error) {
	eventIds = uniqueIds(eventIds)
	if len(eventIds) == 0 {
		return 0, gerror.New("展示采集事件ID不能为空")
	}
	processed := 0
	for _, eventId := range eventIds {
		event, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", eventId).One()
		if err != nil {
			return processed, gerror.Wrapf(err, "读取展示采集事件失败 eventId:%d", eventId)
		}
		if event.IsEmpty() || event["material_role"].String() != collectMaterialRoleDisplay {
			return processed, gerror.Newf("采集事件不是展示资料 eventId:%d", eventId)
		}
		dispatch, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where("event_id", eventId).One()
		if err != nil {
			return processed, gerror.Wrapf(err, "读取采集分发失败 eventId:%d", eventId)
		}
		profileId := dispatch["profile_id"].Int64()
		if profileId <= 0 {
			return processed, gerror.Newf("展示采集事件没有关联资料 eventId:%d", eventId)
		}
		verifyCount, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
			Where("profile_id", profileId).
			Where("purpose", "verify").
			Where("media_type", "video").
			WhereNull("deleted_at").Count()
		if err != nil {
			return processed, gerror.Wrapf(err, "读取资料验证视频失败 profileId:%d", profileId)
		}
		if verifyCount == 0 {
			return processed, gerror.Newf("资料没有可用验证视频 profileId:%d", profileId)
		}
		if !dryRun {
			if err = s.republishSentCollectDispatchWithVerification(ctx, event, profileId); err != nil {
				return processed, err
			}
		}
		processed++
	}
	return processed, nil
}

func (s *sSysPublish) AIOpsRebuildProfileMedia(ctx context.Context, profileIds []int64, dryRun bool) (*sysin.CollectProfileMediaRebuildResult, error) {
	profileIds = uniqueIds(profileIds)
	if len(profileIds) == 0 || len(profileIds) > 20 {
		return nil, gerror.New("单次只能处理1到20条资料")
	}
	result, err := s.rebuildBotProfileMedia(ctx, profileIds, dryRun)
	if err == nil && result.Candidates == 0 {
		result, err = RebuildCollectProfileMedia(ctx, CollectProfileMediaRebuildOptions{ProfileIDs: profileIds, Limit: len(profileIds), DryRun: dryRun})
	}
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
		// Republish is a replacement operation: enqueue cleanup of all prior
		// Telegram display messages before creating the new publication jobs.
		if _, err = s.AIOpsOfflineProfiles(ctx, []int64{profileId}); err != nil {
			return nil, gerror.Wrapf(err, "资料%d清理旧上架消息失败", profileId)
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

func (s *sSysPublish) AIOpsOfflineProfiles(ctx context.Context, profileIds []int64) (*sysin.ProfileStatusModel, error) {
	ids := uniqueIds(profileIds)
	if len(ids) == 0 || len(ids) > 100 {
		return nil, gerror.New("单次只能下架1到100条资料")
	}
	// Keep the AI operation on the same tenant/account-aware path as the
	// normal offline action. Passing zero here silently skipped
	// enqueueProfileDownRun, leaving Telegram messages visible.
	var owners []struct {
		ProfileId int64 `json:"profile_id"`
		TenantId  int64 `json:"tenant_id"`
		AccountId int64 `json:"account_id"`
	}
	if err := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).
		Fields("profile_id,tenant_id,account_id").WhereIn("profile_id", ids).
		WhereNull("deleted_at").Scan(&owners); err != nil {
		return nil, gerror.Wrap(err, "读取资料归属失败")
	}
	groups := make(map[[2]int64][]int64)
	for _, owner := range owners {
		if owner.ProfileId > 0 && owner.TenantId > 0 && owner.AccountId > 0 {
			key := [2]int64{owner.TenantId, owner.AccountId}
			groups[key] = append(groups[key], owner.ProfileId)
		}
	}
	if len(groups) == 0 {
		return nil, gerror.New("资料归属信息不存在")
	}
	var result *sysin.ProfileStatusModel
	var err error
	for owner, groupIds := range groups {
		result, err = s.updateProfileStatus(ctx, &sysin.ProfileStatusInp{Ids: uniqueIds(groupIds), Status: 2}, owner[0], owner[1])
		if err != nil {
			return nil, err
		}
	}
	g.Log().Info(ctx, "AI运维资料下架", g.Map{"profileIds": ids, "result": result, "error": err})
	return result, err
}

func (s *sSysPublish) AIOpsDeleteImportedProfiles(ctx context.Context, tenantId, accountId int64, profileIds []int64, dryRun bool) ([]int64, error) {
	ids := uniqueIds(profileIds)
	if tenantId <= 0 || accountId <= 0 {
		return nil, gerror.New("租户和账号不能为空")
	}
	if len(ids) == 0 || len(ids) > 100 {
		return nil, gerror.New("单次只能删除1到100条资料")
	}
	var rows []struct {
		ProfileId int64 `json:"profileId"`
	}
	err := g.DB().Model(publishProfileStateTable+" ps").Safe().Ctx(ctx).
		Fields("ps.profile_id").
		InnerJoin("hg_content_profile p", "p.id=ps.profile_id AND p.deleted_at IS NULL").
		Where("ps.tenant_id", tenantId).
		Where("ps.account_id", accountId).
		WhereIn("p.source_type", []string{"youban_publish", "youban_collect"}).
		WhereIn("ps.profile_id", ids).
		WhereNull("ps.deleted_at").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "校验TG导入资料失败")
	}
	validated := make([]int64, 0, len(rows))
	for _, row := range rows {
		validated = append(validated, row.ProfileId)
	}
	validated = uniqueIds(validated)
	if len(validated) != len(ids) {
		return nil, gerror.Newf("待删除资料校验失败：请求%d条，仅%d条属于指定账号的可管理资料", len(ids), len(validated))
	}
	if !dryRun {
		if err = s.deleteProfiles(ctx, &sysin.ProfileDeleteInp{Ids: validated}, tenantId, accountId); err != nil {
			return nil, err
		}
	}
	g.Log().Info(ctx, "AI运维资料删除", g.Map{"tenantId": tenantId, "accountId": accountId, "profileIds": validated, "dryRun": dryRun})
	return validated, nil
}

func collectProfileMediaComplete(ctx context.Context, profileId int64) (bool, error) {
	profile, err := g.DB().Model("hg_content_profile p").Safe().Ctx(ctx).
		Fields("p.source_type,COUNT(DISTINCT m.id) AS media_count,COUNT(DISTINCT CASE WHEN COALESCE(m.file_url,'')='' AND COALESCE(m.storage_path,'')='' THEN m.id END) AS missing_media_count").
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
	if expectedMedia <= 0 {
		complete := profile["media_count"].Int() > 0 && profile["missing_media_count"].Int() == 0
		g.Log().Warningf(ctx, "采集事件已清理，按当前资料媒体检查完整性 profileId:%d actual:%d missing:%d complete:%t", profileId, profile["media_count"].Int(), profile["missing_media_count"].Int(), complete)
		return complete, nil
	}
	complete := expectedMedia > 0 && profileMedia >= expectedMedia
	g.Log().Infof(ctx, "资料媒体完整性检查 profileId:%d expected:%d actual:%d complete:%t", profileId, expectedMedia, profileMedia, complete)
	return complete, nil
}
