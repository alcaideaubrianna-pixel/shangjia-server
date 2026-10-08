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
	if options.Limit <= 0 || options.Limit > 10000 {
		options.Limit = 1000
	}
	result := &CollectDuplicateVerifyRepairResult{ProfileIDs: make([]int64, 0)}
	service := NewSysPublish()
	lastEventID := int64(0)
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
		for _, event := range rows {
			lastEventID = event["id"].Int64()
			result.Candidates++
			profileID := duplicateProfileIDFromMessage(event["error_message"].String())
			if profileID <= 0 {
				result.Skipped++
				continue
			}
			content, contentErr := service.collectContentFromEvent(ctx, event)
			if contentErr != nil {
				return result, gerror.Wrapf(contentErr, "读取重复资料采集快照失败 eventId:%d", lastEventID)
			}
			service.enrichCollectContentMediaMetadata(ctx, content)
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
			if options.DryRun {
				continue
			}
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
		return nil
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
	}
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
