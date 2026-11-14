package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/location"
	"hotgo/internal/library/profileextractor"
	"hotgo/internal/service"
)

func (s *sSysPublish) saveProfile(ctx context.Context, in *sysin.ProfileSaveInp, tenantId int64, accountId int64) (res *sysin.ProfileSaveModel, err error) {
	startedAt := time.Now()
	if in == nil {
		return nil, gerror.New("资料信息不能为空")
	}
	trace := in.DiagnosticTraceId
	logStage := func(stage string, stageStartedAt time.Time, stageErr error) {
		fields := g.Map{"trace": trace, "stage": stage, "elapsedMs": time.Since(stageStartedAt).Milliseconds(), "tenantId": tenantId, "accountId": accountId}
		if stageErr != nil {
			fields["error"] = stageErr.Error()
			g.Log().Warning(ctx, "资料保存阶段完成", fields)
			return
		}
		g.Log().Info(ctx, "资料保存阶段完成", fields)
	}
	stageStartedAt := time.Now()
	if err = in.Filter(ctx); err != nil {
		logStage("validate_input", stageStartedAt, err)
		return nil, err
	}
	logStage("validate_input", stageStartedAt, nil)
	draftOnly := in.DraftOnly && in.Id <= 0 && normalizeProfileUUID(in.Uuid) == ""
	stageStartedAt = time.Now()
	if draftOnly {
		in.Province = safeProfileRegionValue(in.Province)
		in.City = safeProfileRegionValue(in.City)
	} else {
		in.Province, in.City = normalizeProfileRegionsBestEffort(ctx, in.PlainText, in.Province, in.City)
	}
	if tenantId <= 0 || accountId <= 0 {
		return nil, gerror.New("上架账号信息不完整")
	}
	if in.Id <= 0 && normalizeProfileUUID(in.Uuid) != "" {
		stageStartedAt = time.Now()
		if in.Id, err = s.resolveProfileId(ctx, 0, in.Uuid, tenantId, accountId); err != nil {
			logStage("resolve_profile_uuid", stageStartedAt, err)
			return nil, err
		}
		logStage("resolve_profile_uuid", stageStartedAt, nil)
	}
	var manualChannelIds []int64
	if !draftOnly && in.ChannelIds != nil {
		stageStartedAt = time.Now()
		in.ChannelIds, err = s.availableProfileChannelIds(ctx, in.ChannelIds, tenantId)
		if err != nil {
			logStage("validate_manual_channels", stageStartedAt, err)
			return nil, err
		}
		logStage("validate_manual_channels", stageStartedAt, nil)
		manualChannelIds = uniqueIds(in.ChannelIds)
	}
	var publishAt *gtime.Time
	if in.PublishAt != "" {
		publishAt = gtime.NewFromStr(in.PublishAt)
		if publishAt == nil {
			return nil, gerror.New("定时上架时间不合法")
		}
	}
	profileId := in.Id
	isNewProfile := profileId == 0
	var removedMediaIds []int64
	var oldFingerprints, newFingerprints []profileFingerprint
	effectiveChannelIds := manualChannelIds
	if !draftOnly && in.ChannelIds == nil && profileId > 0 {
		stageStartedAt = time.Now()
		effectiveChannelIds, err = s.profileChannelIdsOrDefaults(ctx, tenantId, accountId, profileId)
		if err != nil {
			logStage("load_profile_channels", stageStartedAt, err)
			return nil, err
		}
		logStage("load_profile_channels", stageStartedAt, nil)
	} else if !draftOnly && len(effectiveChannelIds) == 0 {
		stageStartedAt = time.Now()
		effectiveChannelIds, err = s.defaultSelectedPublishChannelIds(ctx, tenantId)
		if err != nil {
			logStage("load_default_channels", stageStartedAt, err)
			return nil, err
		}
		logStage("load_default_channels", stageStartedAt, nil)
	}
	preparedAt := time.Now()
	transaction := func() error {
		return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			if in.Id > 0 {
				profileId = in.Id
				stateMod := tx.Model(publishProfileStateTable).Ctx(ctx).
					Where("profile_id", profileId).
					Where("tenant_id", tenantId).
					WhereNull("deleted_at")
				if accountId > 0 {
					stateMod = stateMod.Where("account_id", accountId)
				}
				if count, checkErr := stateMod.Count(); checkErr != nil {
					return gerror.Wrap(checkErr, "检查资料归属失败")
				} else if count == 0 {
					return gerror.New("资料不存在或无权操作")
				}
			}
			if profileId == 0 {
				stepStartedAt := time.Now()
				profileId, err = s.createProfileFromInput(ctx, tx, in, tenantId, accountId)
				logStage("create_profile_row_and_number", stepStartedAt, err)
				if err != nil {
					return err
				}
			} else {
				if err = s.updateProfileFromInput(ctx, tx, in, profileId, accountId, publishAt); err != nil {
					return err
				}
			}
			if err = s.upsertProfileStateTx(ctx, tx, profileId, tenantId, accountId, in.CustomerRemark, in.AntiScanEnabled, publishAt); err != nil {
				return err
			}
			if err = saveProfileRandomMediaTx(ctx, tx, profileId, in.RandomMedia); err != nil {
				return err
			}
			if !draftOnly && (isNewProfile || in.ChannelIds != nil) {
				if err = replaceProfileChannelMappings(ctx, tx, tenantId, accountId, profileId, manualChannelIds); err != nil {
					return err
				}
			}
			if !draftOnly && in.Media != nil && (!isNewProfile || len(in.Media) > 0) {
				removedMediaIds, err = s.syncProfileMediaFromInput(ctx, tx, profileId, tenantId, accountId, in.Media)
				if err != nil {
					return err
				}
			}
			if !draftOnly {
				oldFingerprints, newFingerprints, err = s.replaceProfileFingerprintProjectionTx(ctx, tx, tenantId, accountId, profileId, effectiveChannelIds)
				return err
			}
			return nil
		})
	}
	if profileId > 0 {
		err = s.withProfileLifecycleLock(ctx, tenantId, profileId, transaction)
	} else {
		err = transaction()
	}
	if err != nil {
		logStage("database_transaction", preparedAt, err)
		return nil, err
	}
	transactionDoneAt := time.Now()
	logStage("database_transaction", preparedAt, nil)
	if !draftOnly {
		stageStartedAt = time.Now()
		if err = s.enqueueProfileMaintenance(ctx, profileMaintenancePayload{
			ProfileId: profileId, TenantId: tenantId, AccountId: accountId,
			RemovedMediaIds: removedMediaIds, MediaChanged: in.Media != nil && (!isNewProfile || len(in.Media) > 0),
			OldFingerprints: oldFingerprints, NewFingerprints: newFingerprints,
		}); err != nil {
			g.Log().Warningf(ctx, "资料已保存但后台维护任务提交失败 profileId:%d tenantId:%d accountId:%d err:%+v", profileId, tenantId, accountId, err)
		}
		logStage("enqueue_maintenance", stageStartedAt, err)
	}
	maintenanceQueuedAt := time.Now()
	columns := dao.ContentProfile.Columns()
	profile, err := dao.ContentProfile.Ctx(ctx).Fields(columns.SourceNoteUuid, columns.ProfileNo).
		Where(columns.Id, profileId).WhereNull(columns.DeletedAt).One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料保存结果失败")
	}
	g.Log().Info(ctx, "资料保存完成", g.Map{
		"trace": trace, "profileId": profileId, "tenantId": tenantId, "accountId": accountId, "isNew": isNewProfile,
		"prepareMs": preparedAt.Sub(startedAt).Milliseconds(), "transactionMs": transactionDoneAt.Sub(preparedAt).Milliseconds(),
		"maintenanceEnqueueMs": maintenanceQueuedAt.Sub(transactionDoneAt).Milliseconds(),
		"resultQueryMs":        time.Since(maintenanceQueuedAt).Milliseconds(), "elapsedMs": time.Since(startedAt).Milliseconds(),
	})
	return &sysin.ProfileSaveModel{Id: profileId, Uuid: profile[columns.SourceNoteUuid].String(), ProfileNo: profile[columns.ProfileNo].String()}, nil
}

const profileRegionMaxRunes = 64
const profileRegionLookupTimeout = 3 * time.Second

// normalizeProfileRegionsBestEffort keeps region enrichment from blocking a
// profile save. Region parsing and the province dictionary are auxiliary
// metadata; the original profile text remains the source of truth when either
// step is unavailable. Values that cannot fit the database contract are
// cleared rather than allowing the INSERT to fail.
func normalizeProfileRegionsBestEffort(ctx context.Context, plainText, province, city string) (string, string) {
	// Some clients send labelled values in the province/city fields themselves,
	// for example "北京     城市：北京". Parse those fields first so the next
	// label cannot leak into a varchar(64) value.
	providedProvince, providedCity := profileextractor.RegionLabels(fmt.Sprintf("省份:%s\n城市:%s", province, city))
	if providedProvince != "" {
		province = providedProvince
	}
	if providedCity != "" {
		city = providedCity
	}
	if strings.TrimSpace(province) == "" || strings.TrimSpace(city) == "" {
		parsedProvince, parsedCity := profileextractor.RegionLabels(plainText)
		if strings.TrimSpace(province) == "" {
			province = parsedProvince
		}
		if strings.TrimSpace(city) == "" {
			city = parsedCity
		}
	}
	provinceLength, cityLength := len([]rune(strings.TrimSpace(province))), len([]rune(strings.TrimSpace(city)))
	if provinceLength > profileRegionMaxRunes || cityLength > profileRegionMaxRunes {
		g.Log().Warning(ctx, "资料地区字段超过数据库长度，已忽略异常字段", g.Map{
			"provinceLength": provinceLength,
			"cityLength":     cityLength,
			"maxLength":      profileRegionMaxRunes,
		})
	}
	province = safeProfileRegionValue(province)
	city = safeProfileRegionValue(city)
	if province == "" && city == "" {
		return "", ""
	}

	lookupCtx, cancel := context.WithTimeout(ctx, profileRegionLookupTimeout)
	defer cancel()
	normalizedProvince, normalizedCity, _, err := location.NormalizeRegionCodes(lookupCtx, province, city)
	if err != nil {
		g.Log().Warning(ctx, "资料地区编码转换失败，已降级保存原始资料", g.Map{
			"provinceLength": len([]rune(province)),
			"cityLength":     len([]rune(city)),
			"error":          err.Error(),
		})
		return province, city
	}
	return safeProfileRegionValue(normalizedProvince), safeProfileRegionValue(normalizedCity)
}

func safeProfileRegionValue(value string) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) > profileRegionMaxRunes {
		return ""
	}
	return value
}

func (s *sSysPublish) supersedeProfilePendingTelegramJobsOutsideChannels(ctx context.Context, profileId, tenantId, accountId int64, channelIds []int64) error {
	if profileId <= 0 || tenantId <= 0 {
		return nil
	}
	mod := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("profile_id", profileId).
		WhereIn("status", []string{"pending", "sending", "failed_retry", "unknown"})
	if accountId > 0 {
		mod = mod.Where("account_id", accountId)
	}
	channelIds = uniqueIds(channelIds)
	if len(channelIds) > 0 {
		mod = mod.WhereNotIn("channel_id", channelIds)
	}
	var jobs []telegramResubmitJob
	if err := mod.OrderAsc("id").Scan(&jobs); err != nil {
		return gerror.Wrap(err, "读取资料已移除频道的TG任务失败")
	}
	for _, job := range jobs {
		if err := s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			return err
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "publish", "superseded", "资料频道配置已变更，停止已移除频道的待发送任务")
	}
	return nil
}

func (s *sSysPublish) deleteProfiles(ctx context.Context, in *sysin.ProfileDeleteInp, tenantId int64, accountId int64) (err error) {
	return s.deleteProfilesWithOperationNo(ctx, in, tenantId, accountId, "")
}

func (s *sSysPublish) deleteProfilesWithOperationNo(ctx context.Context, in *sysin.ProfileDeleteInp, tenantId int64, accountId int64, operationNo string) (err error) {
	if in == nil || (len(in.Ids) == 0 && len(in.Uuids) == 0) {
		return gerror.New("请选择要删除的资料")
	}
	targetIds, err := s.allowedProfileTargetIds(ctx, in.Ids, in.Uuids, tenantId, accountId)
	if err != nil {
		return err
	}
	ids, err := s.allowedProfileIds(ctx, targetIds, tenantId, accountId)
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return gerror.New("资料不存在或无权操作")
	}
	claimedIDs := make([]int64, 0, len(ids))
	for _, profileID := range ids {
		if tenantId <= 0 {
			break
		}
		claimed, claimErr := claimProfileOperation(ctx, tenantId, profileID, profileOperationDelete)
		if claimErr != nil {
			for _, claimedID := range claimedIDs {
				releaseProfileOperationClaim(ctx, tenantId, claimedID)
			}
			return claimErr
		}
		if !claimed {
			for _, claimedID := range claimedIDs {
				releaseProfileOperationClaim(ctx, tenantId, claimedID)
			}
			return gerror.New("该资料删除正在处理中，请稍候刷新")
		}
		claimedIDs = append(claimedIDs, profileID)
	}
	defer func() {
		for _, profileID := range claimedIDs {
			releaseProfileOperationClaim(ctx, tenantId, profileID)
		}
	}()
	cleanupQueued, snapshotsByID, err := s.enqueueProfilesTelegramCleanupBeforeDelete(ctx, ids, tenantId, operationNo)
	if err != nil {
		return err
	}
	if err = s.deactivateChannelProfiles(ctx, tenantId, ids); err != nil {
		return err
	}
	if err = s.supersedeProfilePendingTelegramJobs(ctx, ids, tenantId); err != nil {
		return err
	}
	columns := dao.ContentProfile.Columns()
	fingerprintRows, err := g.DB().Model(publishProfileFingerprintTable).Safe().Ctx(ctx).WhereIn("profile_id", ids).All()
	if err != nil {
		return gerror.Wrap(err, "读取待删除资料指纹失败")
	}
	if err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, txErr := detachProfileFingerprintsTx(ctx, tx, ids); txErr != nil {
			return txErr
		}
		if originErr := deleteProfileOriginsTx(ctx, tx, ids); originErr != nil {
			return originErr
		}
		if _, txErr := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).WhereIn(columns.Id, ids).Unscoped().Delete(); txErr != nil {
			return gerror.Wrap(txErr, "删除资料失败")
		}
		if _, txErr := tx.Model(publishCollectChannelProfileTable).Ctx(ctx).WhereIn("profile_id", ids).Delete(); txErr != nil {
			return gerror.Wrap(txErr, "清理频道资料计数失败")
		}
		return nil
	}); err != nil {
		return err
	}
	fingerprintsByScope := make(map[[2]int64][]profileFingerprint)
	for _, row := range fingerprintRows {
		scope := [2]int64{row["tenant_id"].Int64(), row["account_id"].Int64()}
		fingerprintsByScope[scope] = append(fingerprintsByScope[scope], profileFingerprint{ChannelID: row["channel_id"].Int64(), Layer: row["layer"].String(), Signature: row["signature"].String(), ItemTotal: row["item_total"].Int(), SignatureCount: row["signature_count"].Int()})
	}
	for scope, items := range fingerprintsByScope {
		clearProfileFingerprintCache(ctx, scope[0], scope[1], items)
	}
	if err = s.deleteProfileNoteIndex(ctx, ids); err != nil {
		return err
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(operationNo)), botProfileDeleteOperationPrefix) && !cleanupQueued {
		for _, profileID := range ids {
			snapshot := snapshotsByID[profileID]
			job := telegramJobRecord{
				TenantId: tenantId, AccountId: accountId, ProfileId: profileID,
				OperationNo: operationNo,
			}
			if snapshot != nil {
				job.ProfileNoSnapshot = snapshot[columns.ProfileNo].String()
				job.TitleSnapshot = snapshot[columns.Title].String()
			}
			if notifyErr := s.notifyBotProfileUnpublishCompleted(ctx, job, tgQueuePayload{ForceCompletion: true}); notifyErr != nil {
				g.Log().Warningf(ctx, "发送无TG清理任务的Bot资料删除完成通知失败 profileId:%d err:%+v", profileID, notifyErr)
			}
		}
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
}

func (s *sSysPublish) supersedeProfilePendingTelegramJobs(ctx context.Context, profileIds []int64, tenantId int64) error {
	profileIds = uniqueIds(profileIds)
	if tenantId <= 0 || len(profileIds) == 0 {
		return nil
	}
	var jobs []telegramResubmitJob
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		WhereIn("profile_id", profileIds).
		WhereIn("status", []string{"pending", "sending", "failed_retry", "unknown"}).
		OrderAsc("id").Scan(&jobs); err != nil {
		return gerror.Wrap(err, "读取资料待发送TG任务失败")
	}
	for _, job := range jobs {
		if err := s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			return gerror.Wrap(err, "废弃资料待发送TG任务失败")
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "down", "superseded", "资料下架，停止待发送TG任务")
	}
	return nil
}

func (s *sSysPublish) enqueueProfilesTelegramCleanupBeforeDelete(ctx context.Context, ids []int64, tenantId int64, operationNo string) (bool, map[int64]gdb.Record, error) {
	if len(ids) == 0 {
		return false, map[int64]gdb.Record{}, nil
	}
	jobs, err := s.telegramJobsWithActiveMessages(ctx, tenantId, ids, "")
	if err != nil {
		return false, nil, err
	}
	var snapshots []gdb.Record
	if err = g.DB().Model(dao.ContentProfile.Table()).Safe().Ctx(ctx).
		Fields(dao.ContentProfile.Columns().Id+","+dao.ContentProfile.Columns().ProfileNo+","+dao.ContentProfile.Columns().Title).
		WhereIn(dao.ContentProfile.Columns().Id, ids).Scan(&snapshots); err != nil {
		return false, nil, gerror.Wrap(err, "读取删除资料通知上下文失败")
	}
	snapshotByID := make(map[int64]gdb.Record, len(snapshots))
	for _, row := range snapshots {
		snapshotByID[row[dao.ContentProfile.Columns().Id].Int64()] = row
	}
	jobIds := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		if job.Id <= 0 {
			continue
		}
		if snapshot := snapshotByID[job.ProfileId]; snapshot != nil {
			if _, updateErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Data(g.Map{
				"operation_type": "delete", "profile_no_snapshot": snapshot[dao.ContentProfile.Columns().ProfileNo].String(),
				"title_snapshot": snapshot[dao.ContentProfile.Columns().Title].String(), "notify_account_id": job.AccountId,
			}).Update(); updateErr != nil {
				return false, nil, gerror.Wrap(updateErr, "保存删除资料通知上下文失败")
			}
		}
		jobIds = append(jobIds, job.Id)
	}
	for _, job := range jobs {
		if job.Id <= 0 {
			continue
		}
		if err := s.enqueueTelegramCleanupJobWithContext(ctx, job.Id, 0, tgQueuePayload{
			CleanupOperationNo: strings.TrimSpace(operationNo),
			CleanupJobIds:      jobIds,
		}); err != nil {
			return len(jobIds) > 0, snapshotByID, gerror.Wrap(err, "加入资料TG清理队列失败")
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "delete", "queued", "资料已删除，TG历史消息已加入异步清理队列")
	}
	return len(jobIds) > 0, snapshotByID, nil
}

func (s *sSysPublish) updateProfileStatus(ctx context.Context, in *sysin.ProfileStatusInp, tenantId int64, accountId int64) (res *sysin.ProfileStatusModel, err error) {
	if in == nil || (len(in.Ids) == 0 && len(in.Uuids) == 0) {
		return nil, gerror.New("请选择要处理的资料")
	}
	if in.Status != 1 && in.Status != 2 {
		return nil, gerror.New("资料状态不合法")
	}
	targetIds, err := s.allowedProfileTargetIds(ctx, in.Ids, in.Uuids, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	ids, err := s.allowedProfileIds(ctx, targetIds, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, gerror.New("资料不存在或无权操作")
	}
	op := profileOperationPublish
	if in.Status == 2 {
		op = profileOperationUnpublish
	}
	claimedIDs := make([]int64, 0, len(ids))
	for _, profileID := range ids {
		claimed, claimErr := claimProfileOperation(ctx, tenantId, profileID, op)
		if claimErr != nil {
			return nil, claimErr
		}
		if !claimed {
			for _, claimedID := range claimedIDs {
				releaseProfileOperationClaim(ctx, tenantId, claimedID)
			}
			return nil, gerror.New("该资料操作正在处理中，请稍候刷新")
		}
		claimedIDs = append(claimedIDs, profileID)
	}
	defer func() {
		if err != nil {
			for _, profileID := range claimedIDs {
				releaseProfileOperationClaim(ctx, tenantId, profileID)
			}
		}
	}()
	if in.Status == 1 {
		if err = s.submitProfilesByIds(ctx, ids, tenantId, accountId); err != nil {
			return nil, err
		}
		return &sysin.ProfileStatusModel{Message: "资料已提交上架"}, nil
	}
	if err = s.setProfilesOffline(ctx, ids, tenantId); err != nil {
		return nil, err
	}
	if err = s.enqueueProfileDownRun(ctx, tenantId, ids, 0); err != nil {
		return nil, gerror.Wrap(err, "加入资料下架队列失败")
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return &sysin.ProfileStatusModel{Message: "资料已下架，已有TG消息将在后台清理"}, nil
}

// setProfilesOffline applies the shared local state transition used by both
// regular batch operations and collect-source shutdowns. Telegram cleanup is
// intentionally handled by each caller's queue workflow.
func (s *sSysPublish) setProfilesOffline(ctx context.Context, ids []int64, tenantId int64) error {
	ids = uniqueIds(ids)
	for _, id := range ids {
		if err := s.withProfileLifecycleLock(ctx, tenantId, id, func() error {
			_, lockErr := s.syncProfilePublishState(ctx, id, 2, consts.ContentVisibilityPrivate, nil)
			return lockErr
		}); err != nil {
			return gerror.Wrap(err, "更新资料状态失败")
		}
	}
	// Do not report a successful down operation until the authoritative profile
	// row reflects the new state. This protects both the web and Bot entrypoints
	// from returning "submitted" when a concurrent publish or stale lifecycle
	// write left the profile publicly visible.
	columns := dao.ContentProfile.Columns()
	for _, id := range ids {
		var state struct {
			Status     int    `json:"status"`
			Visibility string `json:"visibility"`
		}
		if err := dao.ContentProfile.Ctx(ctx).Fields(columns.Status, columns.Visibility).
			Where(columns.Id, id).Scan(&state); err != nil {
			return gerror.Wrap(err, "确认资料下架状态失败")
		}
		if state.Status != 2 || state.Visibility != consts.ContentVisibilityPrivate {
			return gerror.Newf("资料 %d 下架状态未生效，请稍后重试", id)
		}
	}
	for _, id := range ids {
		if err := s.syncProfileNoteIndex(ctx, id); err != nil {
			return err
		}
	}
	if err := s.deactivateChannelProfiles(ctx, tenantId, ids); err != nil {
		return err
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
}

func (s *sSysPublish) submitProfilesByIds(ctx context.Context, ids []int64, tenantId int64, accountId int64) error {
	if len(ids) == 0 {
		return nil
	}
	for _, profileId := range uniqueIds(ids) {
		if err := s.submitProfilePublish(ctx, profileId, tenantId, accountId, contexts.GetUserId(ctx), "", nil, false); err != nil {
			return err
		}
		if err := s.syncProfileNoteIndex(ctx, profileId); err != nil {
			return err
		}
	}
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return nil
}

func (s *sSysPublish) createProfileFromInput(ctx context.Context, tx gdb.TX, in *sysin.ProfileSaveInp, tenantId int64, accountId int64) (int64, error) {
	columns := dao.ContentProfile.Columns()
	now := gtime.Now()
	extracted := profileextractor.Merge(in.PlainText, 0, 0, in.Tag)
	isVirgin := in.IsVirgin
	if isVirgin == 0 {
		isVirgin = extracted.Virgin
	}
	data := g.Map{
		columns.SourceType:      publishProfileSourceType,
		columns.SourceNoteUuid:  newPublishProfileUUID(),
		columns.SourceKey:       fmt.Sprintf("youban_publish:profile:%s", newPublishProfileUUID()),
		columns.Title:           in.Title,
		columns.Summary:         profileSummary(in.PlainText),
		columns.PlainText:       in.PlainText,
		columns.Province:        in.Province,
		columns.City:            in.City,
		columns.Age:             extracted.Age,
		columns.IsVirgin:        isVirgin,
		columns.Height:          extracted.Height,
		columns.Weight:          extracted.Weight,
		columns.CupSize:         extracted.Cup,
		columns.Visibility:      consts.ContentVisibilityPublic,
		columns.ReviewStatus:    consts.ContentReviewPending,
		columns.ImportStatus:    "manual",
		columns.SourceCreateBy:  strconv.FormatInt(accountId, 10),
		columns.SourceUpdateBy:  strconv.FormatInt(accountId, 10),
		columns.SourceCreatedAt: now,
		columns.SourceUpdatedAt: now,
		columns.Status:          in.Status,
		columns.CreatedAt:       now,
		columns.UpdatedAt:       now,
	}
	publishedAt := gtime.NewFromStr(in.PublishAt)
	if in.Status == 1 && publishedAt == nil {
		publishedAt = now
	}
	data[columns.PublishedAt] = publishedAt
	data[columns.AdminRemark] = in.CustomerRemark
	var lastErr error
	for i := 0; i < 1000; i++ {
		numberStartedAt := time.Now()
		profileNo, err := s.nextAccountProfileNo(ctx, tx, tenantId, accountId)
		g.Log().Info(ctx, "资料保存阶段完成", g.Map{"trace": in.DiagnosticTraceId, "stage": "generate_profile_no", "elapsedMs": time.Since(numberStartedAt).Milliseconds(), "tenantId": tenantId, "accountId": accountId, "attempt": i + 1, "error": err})
		if err != nil {
			return 0, err
		}
		data[columns.ProfileNo] = profileNo
		insertStartedAt := time.Now()
		id, insertErr := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Data(data).InsertAndGetId()
		g.Log().Info(ctx, "资料保存阶段完成", g.Map{"trace": in.DiagnosticTraceId, "stage": "insert_profile_row", "elapsedMs": time.Since(insertStartedAt).Milliseconds(), "tenantId": tenantId, "accountId": accountId, "attempt": i + 1, "error": insertErr})
		if insertErr == nil {
			return id, nil
		}
		lastErr = insertErr
		if !isProfileNoUniqueConstraintError(insertErr) {
			return 0, gerror.Wrap(insertErr, "创建资料失败")
		}
	}
	return 0, gerror.Wrap(lastErr, "创建资料失败，资料编号重复")
}

func (s *sSysPublish) updateProfileFromInput(ctx context.Context, tx gdb.TX, in *sysin.ProfileSaveInp, profileId int64, accountId int64, publishAt *gtime.Time) error {
	columns := dao.ContentProfile.Columns()
	now := gtime.Now()
	current, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).
		Where(columns.Id, profileId).
		Fields(
			columns.Status,
			columns.PublishedAt,
			columns.Visibility,
			columns.Title,
			columns.PlainText,
			columns.Province,
			columns.City,
			columns.Age,
			columns.IsVirgin,
			columns.Height,
			columns.Weight,
			columns.CupSize,
		).
		One()
	if err != nil {
		return gerror.Wrap(err, "读取资料当前状态失败")
	}
	nextStatus := in.Status
	nextVisibility := in.Visibility
	profilePublishedAt := publishAt
	if in.KeepPublishState {
		nextStatus = current[columns.Status].Int()
		nextVisibility = current[columns.Visibility].String()
		profilePublishedAt = current[columns.PublishedAt].GTime()
	} else if current[columns.Status].Int() == 1 {
		nextStatus = 1
		nextVisibility = consts.ContentVisibilityPublic
	}
	extracted := profileextractor.Refresh(in.PlainText, profileextractor.Fields{
		Age:    current[columns.Age].Int(),
		Virgin: current[columns.IsVirgin].Int(),
		Height: current[columns.Height].Int(),
		Weight: current[columns.Weight].Int(),
		Cup:    current[columns.CupSize].String(),
	})
	if in.IsVirgin != 0 {
		extracted.Virgin = in.IsVirgin
	}
	if cup := profileextractor.NormalizeCup(in.Tag); cup != "" {
		extracted.Cup = cup
	}
	data := g.Map{
		columns.Title:       in.Title,
		columns.Summary:     profileSummary(in.PlainText),
		columns.PlainText:   in.PlainText,
		columns.Province:    in.Province,
		columns.City:        in.City,
		columns.Age:         extracted.Age,
		columns.IsVirgin:    extracted.Virgin,
		columns.Height:      extracted.Height,
		columns.Weight:      extracted.Weight,
		columns.CupSize:     extracted.Cup,
		columns.Visibility:  nextVisibility,
		columns.AdminRemark: in.CustomerRemark,
		columns.Status:      nextStatus,
	}
	if profileContentChanged(current, in) {
		data[columns.SourceUpdateBy] = strconv.FormatInt(accountId, 10)
		data[columns.SourceUpdatedAt] = now
		data[columns.UpdatedAt] = now
	}
	if nextStatus == 1 && profilePublishedAt == nil {
		profilePublishedAt = now
	}
	data[columns.PublishedAt] = profilePublishedAt
	if _, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(columns.Id, profileId).Data(data).Update(); err != nil {
		return gerror.Wrap(err, "更新资料失败")
	}
	return nil
}

func profileContentChanged(current gdb.Record, in *sysin.ProfileSaveInp) bool {
	if current.IsEmpty() || in == nil {
		return false
	}
	return strings.TrimSpace(current["title"].String()) != strings.TrimSpace(in.Title) ||
		strings.TrimSpace(current["plain_text"].String()) != strings.TrimSpace(in.PlainText) ||
		strings.TrimSpace(current["province"].String()) != strings.TrimSpace(in.Province) ||
		strings.TrimSpace(current["city"].String()) != strings.TrimSpace(in.City) ||
		(in.IsVirgin != 0 && current["is_virgin"].Int() != in.IsVirgin) ||
		(profileextractor.NormalizeCup(in.Tag) != "" &&
			strings.TrimSpace(current["cup_size"].String()) != profileextractor.NormalizeCup(in.Tag))
}

func isProfileNoUniqueConstraintError(err error) bool {
	if !isUniqueConstraintError(err) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "profile_no") || strings.Contains(message, "uk_content_profile_no")
}
