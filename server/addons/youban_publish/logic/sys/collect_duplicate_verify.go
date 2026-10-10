package sys

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/internal/dao"
)

var duplicateProfileIDPattern = regexp.MustCompile(`profileId:(\d+)`)

type CollectDuplicateVerifyRepairOptions struct {
	Limit  int
	DryRun bool
}

type CollectDuplicateVerifyRepairResult struct {
	Candidates  int
	Recoverable int
	Repaired    int
	Skipped     int
	ProfileIDs  []int64
}

// RepairCollectDuplicateVerification backfills verification media that was
// cached after an existing profile had already been selected by deduplication.
func RepairCollectDuplicateVerification(ctx context.Context, options CollectDuplicateVerifyRepairOptions) (*CollectDuplicateVerifyRepairResult, error) {
	if options.Limit <= 0 || options.Limit > 100000 {
		options.Limit = 50000
	}
	result := &CollectDuplicateVerifyRepairResult{ProfileIDs: make([]int64, 0)}
	service := NewSysPublish()
	lastEventID := int64(0)
	seenProfiles := make(map[int64]struct{})
	for result.Candidates < options.Limit {
		batchSize := options.Limit - result.Candidates
		if batchSize > 200 {
			batchSize = 200
		}
		rows, err := collectDuplicateVerifyRepairCandidates(ctx, lastEventID, batchSize)
		if err != nil {
			return result, err
		}
		if len(rows) == 0 {
			break
		}
		existingProfiles, err := collectProfilesWithVerificationMedia(ctx, duplicateProfileIDsFromEvents(rows))
		if err != nil {
			return result, err
		}
		for _, event := range rows {
			lastEventID = event["id"].Int64()
			result.Candidates++
			profileID := duplicateProfileIDFromMessage(event["error_message"].String())
			if profileID <= 0 {
				result.Skipped++
				continue
			}
			if _, exists := existingProfiles[profileID]; exists {
				result.Skipped++
				continue
			}
			if _, seen := seenProfiles[profileID]; seen {
				result.Skipped++
				continue
			}
			seenProfiles[profileID] = struct{}{}
			if options.DryRun {
				result.Recoverable++
				result.ProfileIDs = append(result.ProfileIDs, profileID)
				continue
			}
			content, contentErr := service.collectContentFromEvent(ctx, event)
			if contentErr != nil {
				return result, gerror.Wrapf(contentErr, "读取重复资料采集快照失败 eventId:%d", lastEventID)
			}
			content, contentErr = service.canonicalCollectProfileMedia(ctx, event, content)
			if contentErr != nil {
				return result, gerror.Wrapf(contentErr, "整理重复资料验证媒体失败 eventId:%d", lastEventID)
			}
			if !hasUsableCollectVerificationMedia(content) {
				result.Skipped++
				continue
			}
			result.Recoverable++
			result.ProfileIDs = append(result.ProfileIDs, profileID)
			before, countErr := collectProfileVerificationMediaCount(ctx, profileID)
			if countErr != nil {
				return result, countErr
			}
			if repairErr := service.repairDuplicateProfileVerification(ctx, profileID, event, content); repairErr != nil {
				return result, gerror.Wrapf(repairErr, "回填重复资料验证媒体失败 eventId:%d profileId:%d", lastEventID, profileID)
			}
			after, countErr := collectProfileVerificationMediaCount(ctx, profileID)
			if countErr != nil {
				return result, countErr
			}
			if before == 0 && after > 0 {
				result.Repaired++
			} else {
				result.Skipped++
			}
		}
	}
	g.Log().Infof(ctx, "重复资料验证媒体修复完成 dryRun:%t candidates:%d recoverable:%d repaired:%d skipped:%d profiles:%v", options.DryRun, result.Candidates, result.Recoverable, result.Repaired, result.Skipped, result.ProfileIDs)
	return result, nil
}

func duplicateProfileIDsFromEvents(rows gdb.Result) []int64 {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if profileID := duplicateProfileIDFromMessage(row["error_message"].String()); profileID > 0 {
			ids = append(ids, profileID)
		}
	}
	return ids
}

func collectProfilesWithVerificationMedia(ctx context.Context, profileIDs []int64) (map[int64]struct{}, error) {
	result := make(map[int64]struct{})
	if len(profileIDs) == 0 {
		return result, nil
	}
	rows, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Fields("profile_id").WhereIn("profile_id", profileIDs).
		Where("purpose", collectMaterialRoleVerify).WhereNull("deleted_at").Group("profile_id").All()
	if err != nil {
		return nil, gerror.Wrap(err, "批量检查资料验证媒体失败")
	}
	for _, row := range rows {
		if profileID := row["profile_id"].Int64(); profileID > 0 {
			result[profileID] = struct{}{}
		}
	}
	return result, nil
}

func collectDuplicateVerifyRepairCandidates(ctx context.Context, lastEventID int64, limit int) (gdb.Result, error) {
	rows, err := g.DB().Model("hg_youban_publish_collect_event e").Safe().Ctx(ctx).
		Fields("e.*").
		WhereGT("e.id", lastEventID).
		Where("e.status", "ignored").
		WhereLike("e.error_message", "资料库已存在相同资料 profileId:%").
		Where("EXISTS (SELECT 1 FROM hg_youban_publish_collect_event v INNER JOIN hg_youban_publish_collect_event_media vm ON vm.event_id=v.id WHERE v.material_parent_event_id=e.id AND v.material_role=? AND vm.cache_status=? AND (COALESCE(vm.storage_path,'')<>'' OR COALESCE(vm.file_url,'')<>''))", collectMaterialRoleVerify, "ready").
		OrderAsc("e.id").Limit(limit).All()
	return rows, gerror.Wrap(err, "读取重复资料验证媒体修复候选失败")
}

func duplicateProfileIDFromMessage(message string) int64 {
	match := duplicateProfileIDPattern.FindStringSubmatch(message)
	if len(match) != 2 {
		return 0
	}
	profileID, _ := strconv.ParseInt(match[1], 10, 64)
	return profileID
}

func collectProfileVerificationMediaCount(ctx context.Context, profileID int64) (int, error) {
	count, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileID).Where("purpose", collectMaterialRoleVerify).WhereNull("deleted_at").Count()
	return count, gerror.Wrapf(err, "检查资料验证媒体失败 profileId:%d", profileID)
}

func (s *sSysPublish) repairExistingCollectedProfileVerification(ctx context.Context, displayEventID int64) error {
	display, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", displayEventID).One()
	if err != nil || display.IsEmpty() {
		return gerror.Wrap(err, "读取待补验证视频的展示事件失败")
	}
	rows, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields("profile_id").Where("event_id", displayEventID).WhereGT("profile_id", 0).Group("profile_id").All()
	if err != nil {
		return gerror.Wrap(err, "读取待补验证视频的资料失败")
	}
	if len(rows) == 0 {
		return nil
	}
	content, err := s.collectContentFromEvent(ctx, display)
	if err != nil {
		return gerror.Wrap(err, "读取待补验证视频的采集快照失败")
	}
	content, err = s.canonicalCollectProfileMedia(ctx, display, content)
	if err != nil {
		return gerror.Wrap(err, "整理待补验证视频的采集媒体失败")
	}
	for _, row := range rows {
		if err = s.repairDuplicateProfileVerification(ctx, row["profile_id"].Int64(), display, content); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) repairDuplicateProfileVerification(ctx context.Context, profileID int64, event gdb.Record, content *collectContentResult) error {
	if profileID <= 0 || event.IsEmpty() || !hasUsableCollectVerificationMedia(content) {
		return nil
	}
	existing, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileID).Where("purpose", collectMaterialRoleVerify).WhereNull("deleted_at").Count()
	if err != nil {
		return gerror.Wrap(err, "检查重复资料验证媒体失败")
	}
	if existing > 0 {
		return s.republishProfileChannelsMissingVerification(ctx, profileID, event)
	}
	verifySnapshot := *content
	verifySnapshot.Media = collectMediaWithPurpose(content.Media, collectMaterialRoleVerify)
	verifySnapshot.MediaCount = len(verifySnapshot.Media)
	prepared, err := s.prepareCollectMaterialSnapshot(ctx, event, &verifySnapshot)
	if err != nil {
		return gerror.Wrap(err, "准备重复资料验证媒体失败")
	}
	verifyItems := preparedCollectVerificationMedia(prepared)
	if len(verifyItems) == 0 {
		return nil
	}
	tenantID := event["tenant_id"].Int64()
	accountID := event["account_id"].Int64()
	columns := dao.ContentProfile.Columns()
	inserted := 0
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		profile, txErr := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).
			Fields(columns.Id, columns.VideoCount, columns.HasVerificationVideo).
			Where(columns.Id, profileID).
			WhereNull(columns.DeletedAt).
			LockUpdate().One()
		if txErr != nil {
			return gerror.Wrap(txErr, "锁定重复资料失败")
		}
		if profile.IsEmpty() {
			return gerror.Newf("重复资料不存在 profileId:%d", profileID)
		}
		owned, txErr := tx.Model(publishProfileStateTable).Ctx(ctx).
			Where("profile_id", profileID).Where("tenant_id", tenantID).Where("account_id", accountID).WhereNull("deleted_at").Count()
		if txErr != nil {
			return gerror.Wrap(txErr, "检查重复资料归属失败")
		}
		if owned == 0 {
			return gerror.Newf("重复资料不属于当前账号 profileId:%d", profileID)
		}
		existing, txErr = tx.Model(publishMediaTable).Safe().Ctx(ctx).
			Where("profile_id", profileID).
			Where("purpose", collectMaterialRoleVerify).
			WhereNull("deleted_at").Count()
		if txErr != nil {
			return gerror.Wrap(txErr, "检查重复资料验证媒体失败")
		}
		if existing > 0 {
			return nil
		}
		now := gtime.Now()
		for _, media := range verifyItems {
			if _, txErr = tx.Model(publishMediaTable).Safe().Ctx(ctx).Data(g.Map{
				"tenant_id": tenantID, "merchant_id": tenantID, "account_id": accountID, "profile_id": profileID,
				"media_type": media.MediaType, "purpose": collectMaterialRoleVerify,
				"name":       fmt.Sprintf("collect-%d-verify-%d", event["id"].Int64(), media.SortIndex),
				"tg_file_id": media.FileId, "file_url": media.FileURL, "storage_path": media.StoragePath,
				"size": media.Size, "md5": media.MD5, "tg_cache_asset_hash": mediaAssetHash(media.StoragePath, media.FileURL),
				"tg_cache_status": tgCacheStatusValid, "poster_url": media.PosterURL,
				"poster_storage_path": media.PosterStoragePath, "perceptual_hash": strings.TrimSpace(media.PerceptualHash),
				"sort_index": media.SortIndex, "status": 1, "created_at": now, "updated_at": now,
				"created_by": accountID, "updated_by": accountID,
			}).Insert(); txErr != nil {
				return gerror.Wrap(txErr, "补充重复资料验证媒体失败")
			}
			inserted++
		}
		_, txErr = tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(columns.Id, profileID).Data(g.Map{
			columns.HasVerificationVideo: 1,
			columns.VideoCount:           profile[columns.VideoCount].Int() + inserted,
			columns.UpdatedAt:            now,
		}).Update()
		return gerror.Wrap(txErr, "更新重复资料验证状态失败")
	})
	if err != nil {
		return err
	}
	if inserted > 0 {
		g.Log().Infof(ctx, "重复资料已补充验证媒体 profileId:%d eventId:%d count:%d", profileID, event["id"].Int64(), inserted)
		s.appendCollectEventLogForRecord(ctx, event, "dedupe", "verification_repaired", "重复资料缺少验证视频，已从本次采集补充", fmt.Sprintf("profileId=%d count=%d", profileID, inserted))
		if err = s.republishProfileChannelsMissingVerification(ctx, profileID, event); err != nil {
			return err
		}
	}
	return nil
}

func duplicateVerificationRepairOperationNo(profileID, eventID int64) string {
	return fmt.Sprintf("verify-repair:%d:profile:%d", eventID, profileID)
}

func (s *sSysPublish) republishProfileChannelsMissingVerification(ctx context.Context, profileID int64, event gdb.Record) error {
	if profileID <= 0 || event.IsEmpty() {
		return nil
	}
	tenantID := event["tenant_id"].Int64()
	accountID := event["account_id"].Int64()
	eventID := event["id"].Int64()
	if tenantID <= 0 || accountID <= 0 || eventID <= 0 {
		return gerror.Newf("验证视频补推参数不完整 profileId:%d eventId:%d tenantId:%d accountId:%d", profileID, eventID, tenantID, accountID)
	}
	rows, err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		Fields("j.channel_id").
		Where("j.profile_id", profileID).
		Where("j.tenant_id", tenantID).
		Where("j.account_id", accountID).
		Where("j.status", "sent").
		Where("EXISTS (SELECT 1 FROM " + publishTgMessageTable + " display_message WHERE display_message.job_id=j.id AND display_message.purpose='display' AND display_message.status='sent')").
		Where("NOT EXISTS (SELECT 1 FROM " + publishTgMessageTable + " verify_message WHERE verify_message.job_id=j.id AND verify_message.purpose='verify' AND verify_message.status='sent')").
		Group("j.channel_id").
		OrderAsc("j.channel_id").All()
	if err != nil {
		return gerror.Wrapf(err, "读取缺少验证视频的已发布频道失败 profileId:%d", profileID)
	}
	channelIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		if channelID := row["channel_id"].Int64(); channelID > 0 {
			channelIDs = append(channelIDs, channelID)
		}
	}
	if len(channelIDs) == 0 {
		return nil
	}
	operationNo := duplicateVerificationRepairOperationNo(profileID, eventID)
	g.Log().Infof(ctx, "重复资料验证媒体触发频道补推 profileId:%d eventId:%d tenantId:%d accountId:%d channelIds:%v operationNo:%s", profileID, eventID, tenantID, accountID, channelIDs, operationNo)
	if err = s.submitProfilePublish(ctx, profileID, tenantID, accountID, accountID, operationNo, channelIDs, false); err != nil {
		return gerror.Wrapf(err, "提交验证视频补推失败 profileId:%d eventId:%d channelIds:%v", profileID, eventID, channelIDs)
	}
	s.appendCollectEventLogForRecord(ctx, event, "publish", "verification_republish_submitted", "验证视频补齐后已提交频道重新上架", fmt.Sprintf("profileId=%d channelIds=%v operationNo=%s", profileID, channelIDs, operationNo))
	return nil
}

func hasUsableCollectVerificationMedia(content *collectContentResult) bool {
	if content == nil {
		return false
	}
	for _, media := range content.Media {
		if !strings.EqualFold(strings.TrimSpace(media.Purpose), collectMaterialRoleVerify) {
			continue
		}
		if strings.TrimSpace(media.StoragePath) == "" && strings.TrimSpace(media.FileUrl) == "" {
			continue
		}
		return true
	}
	return false
}

func preparedCollectVerificationMedia(prepared *collectPreparedMaterial) []collectPreparedMedia {
	if prepared == nil {
		return nil
	}
	items := make([]collectPreparedMedia, 0)
	for _, media := range prepared.Media {
		if strings.EqualFold(strings.TrimSpace(media.Purpose), collectMaterialRoleVerify) {
			items = append(items, media)
		}
	}
	return items
}
