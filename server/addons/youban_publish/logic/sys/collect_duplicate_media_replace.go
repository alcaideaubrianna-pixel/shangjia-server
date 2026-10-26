package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	iservice "hotgo/internal/service"
)

const duplicateMediaReplacedEventIDColumn = "dedupe_media_replaced_event_id"

func duplicateCollectMediaReplaceEnabled(ctx context.Context) bool {
	return g.Cfg().MustGet(ctx, "youbanPublish.collect.duplicateMediaReplaceEnabled", false).Bool()
}

type duplicateMediaReplaceDecision int

type duplicateMediaSnapshot struct {
	MediaType string
	Identity  string
}

type duplicateMediaReplacePlan struct {
	DisplayChanged bool
	VerifyChanged  bool
}

const (
	duplicateMediaReplaceAllowed duplicateMediaReplaceDecision = iota
	duplicateMediaReplaceRetryPublish
	duplicateMediaReplaceAlreadyUsed
)

func decideDuplicateMediaReplace(markerEventID, currentEventID int64) duplicateMediaReplaceDecision {
	if markerEventID <= 0 {
		return duplicateMediaReplaceAllowed
	}
	if markerEventID == currentEventID {
		return duplicateMediaReplaceRetryPublish
	}
	return duplicateMediaReplaceAlreadyUsed
}

func preparedDuplicateMediaByPurpose(items []collectPreparedMedia, purpose string) []duplicateMediaSnapshot {
	result := make([]duplicateMediaSnapshot, 0, len(items))
	for _, item := range items {
		if !strings.EqualFold(strings.TrimSpace(item.Purpose), purpose) {
			continue
		}
		result = append(result, duplicateMediaSnapshot{
			MediaType: strings.ToLower(strings.TrimSpace(item.MediaType)),
			Identity:  duplicateMediaIdentity(item.MD5, item.StoragePath, item.FileURL),
		})
	}
	return result
}

func storedDuplicateMediaByPurpose(rows gdb.Result, purpose string) []duplicateMediaSnapshot {
	result := make([]duplicateMediaSnapshot, 0, len(rows))
	for _, row := range rows {
		if !strings.EqualFold(strings.TrimSpace(row["purpose"].String()), purpose) {
			continue
		}
		result = append(result, duplicateMediaSnapshot{
			MediaType: strings.ToLower(strings.TrimSpace(row["media_type"].String())),
			Identity:  duplicateMediaIdentity(row["md5"].String(), row["storage_path"].String(), row["file_url"].String()),
		})
	}
	return result
}

func duplicateMediaIdentity(md5Value, storagePath, fileURL string) string {
	if value := strings.ToLower(strings.TrimSpace(md5Value)); value != "" {
		return "md5:" + value
	}
	if value := strings.TrimSpace(storagePath); value != "" {
		return "path:" + value
	}
	if value := strings.TrimSpace(fileURL); value != "" {
		return "url:" + value
	}
	return ""
}

func duplicateMediaSnapshotsEqual(left, right []duplicateMediaSnapshot) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Identity == "" || right[index].Identity == "" || left[index] != right[index] {
			return false
		}
	}
	return true
}

func buildDuplicateMediaReplacePlan(existing gdb.Result, prepared []collectPreparedMedia) duplicateMediaReplacePlan {
	preparedVerify := preparedDuplicateMediaByPurpose(prepared, collectMaterialRoleVerify)
	return duplicateMediaReplacePlan{
		DisplayChanged: !duplicateMediaSnapshotsEqual(
			storedDuplicateMediaByPurpose(existing, collectMaterialRoleDisplay),
			preparedDuplicateMediaByPurpose(prepared, collectMaterialRoleDisplay),
		),
		// A display-only duplicate must retain the existing verification media.
		VerifyChanged: len(preparedVerify) > 0 && !duplicateMediaSnapshotsEqual(
			storedDuplicateMediaByPurpose(existing, collectMaterialRoleVerify), preparedVerify,
		),
	}
}

func collectDuplicateReason(duplicate *profileFingerprintDuplicateError, replaced bool) string {
	if duplicate == nil {
		return "资料库已存在相同资料"
	}
	reason := fmt.Sprintf("资料库已存在相同资料 profileId:%d channelId:%d layer:%s", duplicate.ProfileID, duplicate.ChannelID, duplicate.Layer)
	if replaced {
		reason += "，已使用本次采集媒体覆盖"
	}
	return reason
}

// replaceDuplicateCollectProfileMedia lets an account-based collector repair a
// deduplicated profile once without changing its identity, text, or channels.
func (s *sSysPublish) replaceDuplicateCollectProfileMedia(
	ctx context.Context,
	event gdb.Record,
	content *collectContentResult,
	profileID int64,
) (bool, error) {
	if !duplicateCollectMediaReplaceEnabled(ctx) {
		return false, nil
	}
	if profileID <= 0 || event.IsEmpty() || content == nil || len(content.Media) == 0 {
		return false, nil
	}
	if !strings.EqualFold(strings.TrimSpace(event["source_type"].String()), "account") {
		return false, nil
	}
	tenantID := event["tenant_id"].Int64()
	accountID := event["account_id"].Int64()
	eventID := event["id"].Int64()
	if tenantID <= 0 || accountID <= 0 || eventID <= 0 {
		return false, gerror.New("重复资料媒体覆盖缺少租户归属")
	}
	state, err := g.DB().Model(publishProfileStateTable).Ctx(ctx).
		Fields("id,"+duplicateMediaReplacedEventIDColumn).
		Where("profile_id", profileID).
		Where("tenant_id", tenantID).
		Where("account_id", accountID).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return false, gerror.Wrap(err, "读取重复资料媒体覆盖状态失败")
	}
	if state.IsEmpty() {
		return false, nil
	}
	decision := decideDuplicateMediaReplace(state[duplicateMediaReplacedEventIDColumn].Int64(), eventID)
	if decision == duplicateMediaReplaceAlreadyUsed {
		return false, nil
	}
	channelIDs, err := s.profileChannelIdsOrDefaults(ctx, tenantID, accountID, profileID)
	if err != nil {
		return false, err
	}
	replaced := decision == duplicateMediaReplaceRetryPublish
	var oldFingerprints, fingerprints []profileFingerprint
	if decision == duplicateMediaReplaceAllowed {
		prepared, prepareErr := s.prepareCollectMaterial(ctx, event, content)
		if prepareErr != nil {
			return false, prepareErr
		}
		now := gtime.Now()
		err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			state, txErr := tx.Model(publishProfileStateTable).Ctx(ctx).
				Fields("id,tenant_id,account_id,profile_id,"+duplicateMediaReplacedEventIDColumn).
				Where("profile_id", profileID).
				Where("tenant_id", tenantID).
				Where("account_id", accountID).
				WhereNull("deleted_at").
				LockUpdate().One()
			if txErr != nil {
				return gerror.Wrap(txErr, "锁定重复资料媒体覆盖状态失败")
			}
			if state.IsEmpty() || decideDuplicateMediaReplace(state[duplicateMediaReplacedEventIDColumn].Int64(), eventID) != duplicateMediaReplaceAllowed {
				return nil
			}
			release, txErr := s.lockProfileMediaSyncTx(ctx, tx, profileID)
			if txErr != nil {
				return txErr
			}
			defer release()
			existingMedia, txErr := tx.Model(publishMediaTable).Ctx(ctx).
				Fields("id,media_type,purpose,md5,storage_path,file_url,sort_index").
				Where("profile_id", profileID).WhereNull("deleted_at").
				OrderAsc("purpose").OrderAsc("sort_index").OrderAsc("id").All()
			if txErr != nil {
				return gerror.Wrap(txErr, "读取重复资料原媒体失败")
			}
			plan := buildDuplicateMediaReplacePlan(existingMedia, prepared.Media)
			if !plan.DisplayChanged && !plan.VerifyChanged {
				return nil
			}
			changedPurposes := make([]string, 0, 2)
			if plan.DisplayChanged {
				changedPurposes = append(changedPurposes, collectMaterialRoleDisplay)
			}
			if plan.VerifyChanged {
				changedPurposes = append(changedPurposes, collectMaterialRoleVerify)
			}
			if _, txErr = tx.Model(publishMediaTable).Ctx(ctx).
				Where("profile_id", profileID).WhereIn("purpose", changedPurposes).WhereNull("deleted_at").
				Data(g.Map{
					"tg_file_id": "", "tg_thumb_file_id": "", "tg_cache_asset_hash": "",
					"tg_cache_status": tgCacheStatusInvalid, "deleted_by": accountID,
					"deleted_at": now, "updated_at": now,
				}).Update(); txErr != nil {
				return gerror.Wrap(txErr, "清理重复资料旧媒体失败")
			}
			for _, media := range prepared.Media {
				if (media.Purpose == collectMaterialRoleDisplay && !plan.DisplayChanged) ||
					(media.Purpose == collectMaterialRoleVerify && !plan.VerifyChanged) {
					continue
				}
				if _, txErr = tx.Model(publishMediaTable).Ctx(ctx).Data(g.Map{
					"tenant_id": tenantID, "merchant_id": tenantID, "account_id": accountID, "profile_id": profileID,
					"media_type": media.MediaType, "purpose": media.Purpose,
					"name":       fmt.Sprintf("collect-replace-%d-%s-%d", event["id"].Int64(), media.Purpose, media.SortIndex),
					"tg_file_id": media.FileId, "file_url": media.FileURL, "storage_path": media.StoragePath,
					"size": media.Size, "md5": media.MD5,
					"tg_cache_asset_hash": mediaAssetHash(media.StoragePath, media.FileURL), "tg_cache_status": tgCacheStatusValid,
					"poster_url": media.PosterURL, "poster_storage_path": media.PosterStoragePath,
					"perceptual_hash": strings.TrimSpace(media.PerceptualHash), "sort_index": media.SortIndex,
					"status": 1, "created_at": now, "updated_at": now, "created_by": accountID, "updated_by": accountID,
				}).Insert(); txErr != nil {
					return gerror.Wrap(txErr, "写入重复资料新媒体失败")
				}
			}
			if txErr = s.syncOwnedMediaToProfile(ctx, tx, state, profileID); txErr != nil {
				return txErr
			}
			if oldFingerprints, fingerprints, txErr = s.replaceProfileFingerprintProjectionTx(ctx, tx, tenantID, accountID, profileID, channelIDs); txErr != nil {
				return txErr
			}
			result, txErr := tx.Model(publishProfileStateTable).Ctx(ctx).
				Where("id", state["id"].Int64()).Where(duplicateMediaReplacedEventIDColumn, 0).
				Data(g.Map{duplicateMediaReplacedEventIDColumn: eventID, "updated_at": now}).Update()
			if txErr != nil {
				return gerror.Wrap(txErr, "标记重复资料媒体已覆盖失败")
			}
			affected, txErr := result.RowsAffected()
			if txErr != nil {
				return gerror.Wrap(txErr, "读取重复资料媒体覆盖结果失败")
			}
			replaced = affected == 1
			return nil
		})
		if err != nil || !replaced {
			return replaced, err
		}
	}
	if err = s.deleteMediaPHashBucketByProfileId(ctx, profileID); err != nil {
		g.Log().Warningf(ctx, "重复资料媒体覆盖后清理PHash失败 profileId:%d err:%+v", profileID, err)
	} else if err = s.syncMediaPHashBucketsByProfileId(ctx, profileID); err != nil {
		g.Log().Warningf(ctx, "重复资料媒体覆盖后重建PHash失败 profileId:%d err:%+v", profileID, err)
	}
	if err = s.syncProfileNoteIndex(ctx, profileID); err != nil {
		g.Log().Warningf(ctx, "重复资料媒体覆盖后刷新资料索引失败 profileId:%d err:%+v", profileID, err)
	}
	clearProfileFingerprintCache(ctx, tenantID, accountID, oldFingerprints)
	warmProfileFingerprintCache(ctx, tenantID, accountID, profileID, fingerprints)
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	meta := telegramProfilePublishMeta{
		CollectEventId:         eventID,
		CollectSourceId:        event["source_id"].Int64(),
		CollectSourceChatId:    strings.TrimSpace(event["source_chat_id"].String()),
		CollectSourceMessageId: event["source_message_id"].Int64(),
	}
	operationNo := fmt.Sprintf("collect_media_replace:%d", eventID)
	if err = s.submitProfilePublishWithMeta(ctx, profileID, tenantID, accountID, accountID, operationNo, channelIDs, false, meta); err != nil {
		return true, gerror.Wrap(err, "提交重复资料媒体覆盖发布失败")
	}
	s.appendCollectEventLogForRecord(ctx, event, "dedupe_media_replace", "completed", "重复资料媒体已覆盖并重新提交发布", fmt.Sprintf("profileId=%d", profileID))
	return true, nil
}
