package sys

import (
	"context"
	"fmt"
	"sort"
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
	var regions profileRegionPreparation
	if draftOnly {
		in.Province = safeProfileRegionValue(in.Province)
		in.City = safeProfileRegionValue(in.City)
	} else {
		regions = prepareProfileRegions(ctx, in.Id, in.PlainText, in.Province, in.City)
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
				if !draftOnly {
					in.Province, in.City = selectProfileRegions(regions, true, "", "", "", in.PlainText)
				}
				profileId, err = s.createProfileFromInput(ctx, tx, in, tenantId, accountId)
				logStage("create_profile_row_and_number", stepStartedAt, err)
				if err != nil {
					return err
				}
			} else {
				if err = s.updateProfileFromInput(ctx, tx, in, profileId, tenantId, accountId, publishAt, regions); err != nil {
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

// profileRegionPreparation retains the request values until the existing row is
// available. All dictionary lookups finish before entering the save transaction.
type profileRegionPreparation struct {
	rawProvince, rawCity   string
	province, city         string
	bodyProvince, bodyCity string
	cityProvince           string
	explicitFailed         bool
}

func prepareProfileRegions(ctx context.Context, profileID int64, text, province, city string) profileRegionPreparation {
	started := time.Now()
	prepared := profileRegionPreparation{rawProvince: province, rawCity: city}
	lookupCtx, cancel := context.WithTimeout(ctx, profileRegionLookupTimeout)
	defer cancel()
	// Plain lists must reach the dictionary intact. RegionLabels parses one
	// labelled value and intentionally stops at commas; it is only needed for
	// legacy fields carrying embedded labels such as "北京 城市：北京".
	if strings.ContainsAny(province, ":：=") || strings.ContainsAny(city, ":：=") {
		providedProvince, providedCity := profileextractor.RegionLabels(fmt.Sprintf("省份:%s\n城市:%s", province, city))
		if providedProvince != "" {
			province = providedProvince
		}
		if providedCity != "" {
			city = providedCity
		}
	}
	prepared.province, prepared.city = safeProfileRegionValue(province), safeProfileRegionValue(city)
	normalizeStarted := time.Now()
	var lookupErrors []string
	if len([]rune(strings.TrimSpace(prepared.rawProvince))) > profileRegionMaxRunes || len([]rune(strings.TrimSpace(prepared.rawCity))) > profileRegionMaxRunes {
		prepared.explicitFailed = true
		lookupErrors = append(lookupErrors, "地区输入超过长度限制")
	}
	explicitResult := "empty"
	if !prepared.explicitFailed && (prepared.province != "" || prepared.city != "") {
		// Canonical code lists need no name lookup. Parent validation below uses
		// the same cached legacy index as body recognition.
		normalizedProvince, normalizedCity := prepared.province, prepared.city
		var err error
		if !profileRegionCodeList(prepared.province) || !profileRegionCodeList(prepared.city) {
			normalizedProvince, normalizedCity, _, err = location.NormalizeRegionCodes(lookupCtx, prepared.province, prepared.city)
		}
		if err != nil {
			prepared.explicitFailed = true
			lookupErrors = append(lookupErrors, err.Error())
			explicitResult = "lookup_error"
		} else {
			prepared.province, prepared.city = safeProfileRegionValue(normalizedProvince), safeProfileRegionValue(normalizedCity)
			explicitResult = "normalized"
		}
	}
	// Resolve changed city names independently of an echoed old province.
	// Save-time selection below decides which field the user actually changed.
	if !prepared.explicitFailed && city != "" && !profileRegionCodeList(city) {
		parent, resolvedCity, matched, err := location.ResolveCityRegion(lookupCtx, city)
		if err != nil {
			prepared.explicitFailed = true
			explicitResult = "lookup_error"
			lookupErrors = append(lookupErrors, err.Error())
		} else if matched {
			prepared.cityProvince = safeProfileRegionValue(parent)
			prepared.city = safeProfileRegionValue(resolvedCity)
		}
	}
	normalizeElapsed := time.Since(normalizeStarted)
	dictionaryStarted := time.Now()
	if !prepared.explicitFailed && ((prepared.province != "" && profileRegionCodeList(prepared.province)) || (prepared.city != "" && profileRegionCodeList(prepared.city))) {
		index, err := getLegacyCMSRegionIndex(lookupCtx)
		if err == nil {
			if prepared.province != "" && profileRegionCodeList(prepared.province) {
				for _, code := range strings.Split(profileRegionComparisonKey(prepared.province), ",") {
					id, _ := strconv.ParseInt(code, 10, 64)
					row := index.optionsById[id]
					if row == nil || row.Level != 1 {
						err = gerror.Newf("省份或国家编码%s未找到有效地区", code)
						break
					}
				}
				prepared.province = profileRegionComparisonKey(prepared.province)
			}
			parents := make([]string, 0)
			var cityCodes []string
			if err == nil && prepared.city != "" && profileRegionCodeList(prepared.city) {
				cityCodes = strings.Split(profileRegionComparisonKey(prepared.city), ",")
			}
			for _, code := range cityCodes {
				id, _ := strconv.ParseInt(code, 10, 64)
				row := index.optionsById[id]
				if row == nil || row.Level != 2 || row.Pid <= 0 || index.optionsById[row.Pid] == nil || index.optionsById[row.Pid].Level != 1 {
					err = gerror.Newf("城市编码%s未找到有效父省", code)
					break
				}
				parents = append(parents, strconv.FormatInt(row.Pid, 10))
			}
			if err == nil {
				if len(cityCodes) > 0 {
					prepared.cityProvince = profileRegionComparisonKey(strings.Join(parents, ","))
					prepared.city = profileRegionComparisonKey(prepared.city)
				}
			}
		}
		if err != nil {
			prepared.explicitFailed = true
			explicitResult = "lookup_error"
			lookupErrors = append(lookupErrors, err.Error())
		}
	}
	dictionaryElapsed := time.Since(dictionaryStarted)
	if prepared.explicitFailed && explicitResult == "empty" {
		explicitResult = "invalid_input"
	}
	result := "no_location_fields"
	extractStarted := time.Now()
	fields := materialImportRegionFieldsFromText(text)
	extractElapsed := time.Since(extractStarted)
	matchStarted := time.Now()
	if len(fields.provinceTexts) > 0 || len(fields.cityTexts) > 0 {
		bodyProvince, bodyCity, err := materialImportRegionCodesForFields(lookupCtx, fields)
		if err != nil {
			lookupErrors = append(lookupErrors, err.Error())
			result = "lookup_error"
		} else {
			prepared.bodyProvince, prepared.bodyCity = safeProfileRegionValue(bodyProvince), safeProfileRegionValue(bodyCity)
			result = "no_match"
			if prepared.bodyProvince != "" || prepared.bodyCity != "" {
				result = "matched"
			}
		}
	}
	fieldsLog := g.Map{
		"profileId": profileID, "entry": "saveProfile", "phase": "region_prepare",
		"elapsedMs": time.Since(started).Milliseconds(), "result": result, "explicitResult": explicitResult,
		"normalizeMs":  float64(normalizeElapsed.Microseconds()) / 1000,
		"dictionaryMs": float64(dictionaryElapsed.Microseconds()) / 1000,
		"extractMs":    float64(extractElapsed.Microseconds()) / 1000,
		"matchMs":      float64(time.Since(matchStarted).Microseconds()) / 1000,
	}
	if len(lookupErrors) > 0 {
		fieldsLog["error"] = strings.Join(lookupErrors, "; ")
	}
	if result == "lookup_error" || prepared.explicitFailed {
		g.Log().Warning(ctx, "资料地区预处理已降级", fieldsLog)
	} else {
		g.Log().Info(ctx, "资料地区预处理完成", fieldsLog)
	}
	return prepared
}

// profileRegionCodeList only identifies numeric input. Membership and hierarchy
// are checked against the shared dictionary, never inferred from code length.
func profileRegionCodeList(value string) bool {
	key := profileRegionComparisonKey(value)
	if key == "" {
		return true
	}
	for _, code := range strings.Split(key, ",") {
		if id, err := strconv.ParseInt(code, 10, 64); err != nil || id <= 0 {
			return false
		}
		for _, digit := range code {
			if digit < '0' || digit > '9' {
				return false
			}
		}
	}
	return true
}

// profileRegionComparisonKey treats code lists as sets, including JSON-style
// list strings returned by older clients. It performs no dictionary lookups.
func profileRegionComparisonKey(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '，' || r == '[' || r == ']' || r == '"' || r == '\'' || r == ' ' || r == '\n' || r == '\t'
	})
	seen := make(map[string]bool, len(parts))
	unique := make([]string, 0, len(parts))
	for _, part := range parts {
		if !seen[part] {
			seen[part] = true
			unique = append(unique, part)
		}
	}
	sort.Strings(unique)
	return strings.Join(unique, ",")
}

// selectProfileRegions chooses a whole pair: a partial new province must never
// inherit an unrelated old city. Empty/failed recognition preserves the old pair.
func selectProfileRegions(prepared profileRegionPreparation, create bool, oldProvince, oldCity, oldText, text string) (string, string) {
	if create {
		if prepared.cityProvince != "" {
			return prepared.cityProvince, prepared.city
		}
		if prepared.province != "" || prepared.city != "" {
			return prepared.province, prepared.city
		}
		return prepared.bodyProvince, prepared.bodyCity
	}
	if prepared.explicitFailed {
		return oldProvince, oldCity
	}
	same := func(raw, normalized, old string) bool {
		key := profileRegionComparisonKey(old)
		return profileRegionComparisonKey(raw) == key || profileRegionComparisonKey(normalized) == key
	}
	// Missing fields carry no manual selection intent. Compare only supplied
	// fields, so omitting a city cannot clear it during an ordinary content edit.
	provinceProvided := strings.TrimSpace(prepared.rawProvince) != ""
	cityProvided := strings.TrimSpace(prepared.rawCity) != ""
	manualProvince := provinceProvided && !same(prepared.rawProvince, prepared.province, oldProvince)
	manualCity := cityProvided && !same(prepared.rawCity, prepared.city, oldCity)
	if cityProvided && prepared.city == "" && prepared.cityProvince != "" &&
		profileRegionComparisonKey(prepared.cityProvince) != profileRegionComparisonKey(oldProvince) {
		// A country-only value in the city field legitimately normalizes to an
		// empty city. That is still a changed selection when the country differs.
		manualCity = true
	}
	if manualProvince || manualCity {
		if prepared.province == "" && prepared.city == "" && prepared.cityProvince == "" {
			return oldProvince, oldCity
		}
		// A changed province with the previous city echoed by a client is a
		// province-only selection; retaining that city would create a mixed pair.
		if manualProvince && !manualCity {
			return prepared.province, ""
		}
		if manualCity && prepared.cityProvince != "" {
			return prepared.cityProvince, prepared.city
		}
		return prepared.province, prepared.city
	}
	if (text != oldText || strings.TrimSpace(oldProvince) == "" || strings.TrimSpace(oldCity) == "") && (prepared.bodyProvince != "" || prepared.bodyCity != "") {
		return prepared.bodyProvince, prepared.bodyCity
	}
	return oldProvince, oldCity
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
	operationNo = strings.TrimSpace(operationNo)
	if operationNo == "" {
		operationNo = fmt.Sprintf("delete:%d:%d", tenantId, time.Now().UnixNano())
	}
	var cleanupPersisted bool
	var snapshotsByID map[int64]gdb.Record
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
		var txErr error
		cleanupPersisted, snapshotsByID, txErr = s.persistProfilesTelegramCleanupBeforeDeleteTx(ctx, tx, ids, tenantId, operationNo)
		if txErr != nil {
			return txErr
		}
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
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(operationNo)), botProfileDeleteOperationPrefix) && !cleanupPersisted {
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
		WhereIn("status", []string{"pending", "sending", "confirming", "failed_retry", "unknown", "reconciling"}).
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

// Persist cleanup with the deletion; the existing cleanup intent recovery loop
// dispatches committed rows without a post-commit enqueue dependency.
func (s *sSysPublish) persistProfilesTelegramCleanupBeforeDeleteTx(ctx context.Context, tx gdb.TX, ids []int64, tenantId int64, operationNo string) (bool, map[int64]gdb.Record, error) {
	ctx = gdb.WithTX(ctx, tx)
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
		if _, _, err := ensureTelegramCleanupIntent(ctx, job.Id, telegramOutboxCleanupPrefix+"manual", operationNo, tgQueuePayload{
			CleanupOperationNo: operationNo,
			CleanupJobIds:      jobIds,
			Trace:              capturePublishTrace(ctx),
		}); err != nil {
			return len(jobIds) > 0, snapshotByID, gerror.Wrap(err, "保存资料TG清理意图失败")
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "delete", "pending", "已保存资料TG历史消息清理意图，事务提交后由后台分发")
	}
	return len(jobIds) > 0, snapshotByID, nil
}

func (s *sSysPublish) updateProfileStatus(ctx context.Context, in *sysin.ProfileStatusInp, tenantId int64, accountId int64) (res *sysin.ProfileStatusModel, err error) {
	if in == nil || (len(in.Ids) == 0 && len(in.Uuids) == 0) {
		return nil, gerror.New("请选择要处理的资料")
	}
	if in.CancelCycle {
		targetIds, resolveErr := s.allowedProfileTargetIds(ctx, in.Ids, in.Uuids, tenantId, accountId)
		if resolveErr != nil {
			return nil, resolveErr
		}
		ids, resolveErr := s.allowedProfileIds(ctx, targetIds, tenantId, accountId)
		if resolveErr != nil {
			return nil, resolveErr
		}
		return s.cancelProfileCycle(ctx, ids, tenantId, accountId)
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
	operationNo := fmt.Sprintf("unpublish:%d:%d", tenantId, time.Now().UnixNano())
	if err = s.setProfilesOffline(ctx, ids, tenantId, operationNo); err != nil {
		return nil, err
	}
	if err = s.enqueueProfileDownRunWithOperationNo(ctx, tenantId, ids, 0, operationNo); err != nil {
		_ = s.clearProfilesUnpublishing(ctx, ids, tenantId, operationNo)
		return nil, gerror.Wrap(err, "加入资料下架队列失败")
	}
	// The cleanup queue can finish before the state projection is written on
	// very small/local deployments. Re-check once after enqueueing so that a
	// completed down operation cannot leave a stale loading state behind.
	_ = s.finishProfilesUnpublishing(ctx, ids, tenantId, operationNo)
	service.SysContent().ClearHomeProfileCardsCache(ctx)
	return &sysin.ProfileStatusModel{Message: "资料已下架，已有TG消息将在后台清理"}, nil
}

// setProfilesOffline applies the shared local state transition used by both
// regular batch operations and collect-source shutdowns. Telegram cleanup is
// intentionally handled by each caller's queue workflow.
func (s *sSysPublish) setProfilesOffline(ctx context.Context, ids []int64, tenantId int64, operationNos ...string) error {
	ids = uniqueIds(ids)
	for _, id := range ids {
		if err := s.withProfileLifecycleLock(ctx, tenantId, id, func() error {
			_, lockErr := s.syncProfilePublishState(ctx, id, 2, consts.ContentVisibilityPrivate, nil)
			return lockErr
		}); err != nil {
			return gerror.Wrap(err, "更新资料状态失败")
		}
	}
	if len(operationNos) > 0 && strings.TrimSpace(operationNos[0]) != "" {
		if err := s.markProfilesUnpublishing(ctx, ids, tenantId, operationNos[0]); err != nil {
			return err
		}
	}
	columns := dao.ContentProfile.Columns()
	for _, id := range ids {
		state, err := dao.ContentProfile.Ctx(ctx).Fields(columns.Status, columns.Visibility).
			Where(columns.Id, id).One()
		if err != nil {
			return gerror.Wrap(err, "确认资料下架状态失败")
		}
		if state.IsEmpty() {
			return gerror.Newf("资料 %d 不存在或已删除，无法确认下架状态", id)
		}
		if state[columns.Status].Int() != 2 || state[columns.Visibility].String() != consts.ContentVisibilityPrivate {
			return gerror.Newf("资料 %d 下架状态未生效，请稍后重试", id)
		}
	}
	if err := s.supersedeProfilePendingTelegramJobs(ctx, ids, tenantId); err != nil {
		return err
	}
	activeJobs, err := s.telegramJobsWithActiveMessages(ctx, tenantId, ids, "")
	if err != nil {
		return err
	}
	manualDown := len(operationNos) > 0 && isManualTelegramCleanupUnpublish(tgQueuePayload{CleanupOperationNo: operationNos[0]})
	var cleanupPayloads map[int64]tgQueuePayload
	if manualDown {
		channelIDs := make([]int64, 0, len(activeJobs))
		for _, job := range activeJobs {
			channelIDs = append(channelIDs, job.ChannelId)
		}
		policies, policyErr := s.channelHistoryMessagePolicies(ctx, tenantId, channelIDs)
		if policyErr != nil {
			return policyErr
		}
		cleanupPayloads, err = s.prepareProfileDownCleanupPayloads(ctx, activeJobs, policies, operationNos[0])
		if err != nil {
			return err
		}
	}
	for _, job := range activeJobs {
		if job.Id <= 0 {
			continue
		}
		cleanupPayload := tgQueuePayload{}
		if manualDown {
			var ok bool
			cleanupPayload, ok = cleanupPayloads[job.Id]
			if !ok {
				continue
			}
		} else if len(operationNos) > 0 {
			cleanupPayload.CleanupOperationNo = strings.TrimSpace(operationNos[0])
		}
		if err := s.enqueueTelegramCleanupJobWithContext(ctx, job.Id, 0, cleanupPayload); err != nil {
			return gerror.Wrap(err, "加入资料已发送消息清理队列失败")
		}
		s.appendTelegramJobLog(ctx, job.telegramJobRecord(), "down", "queued", "资料下架后补扫已发送TG消息并加入清理队列")
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
		columns.SourceType:     publishProfileSourceType,
		columns.SourceNoteUuid: newPublishProfileUUID(),
		columns.SourceKey:      fmt.Sprintf("youban_publish:profile:%s", newPublishProfileUUID()),
		columns.Title:          in.Title,
		columns.Summary:        profileSummary(in.PlainText),
		columns.PlainText:      in.PlainText,
		columns.Province:       in.Province,
		columns.City:           in.City,
		columns.Age:            extracted.Age,
		columns.IsVirgin:       isVirgin,
		columns.Height:         extracted.Height,
		columns.Weight:         extracted.Weight,
		columns.CupSize:        extracted.Cup,
		// Newly created profiles are always offline. Lifecycle transitions are
		// owned by Publish/Unpublish/Republish, never by the save payload.
		columns.Visibility:      consts.ContentVisibilityPrivate,
		columns.ReviewStatus:    consts.ContentReviewPending,
		columns.ImportStatus:    "manual",
		columns.SourceCreateBy:  strconv.FormatInt(accountId, 10),
		columns.SourceUpdateBy:  strconv.FormatInt(accountId, 10),
		columns.SourceCreatedAt: now,
		columns.SourceUpdatedAt: now,
		columns.Status:          2,
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

type pendingProfileEditorKey struct{}

func (s *sSysPublish) updateProfileFromInput(ctx context.Context, tx gdb.TX, in *sysin.ProfileSaveInp, profileId, tenantId, accountId int64, publishAt *gtime.Time, regions profileRegionPreparation) error {
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
	if in.EditPendingReview && current[columns.Status].Int() != sysin.ProfileStatusReviewPending {
		return gerror.New("资料审核状态已变化，请刷新后重试")
	}
	var pendingReviewID int64
	if current[columns.Status].Int() == sysin.ProfileStatusReviewPending {
		adminID, _ := ctx.Value(pendingProfileEditorKey{}).(int64)
		if adminID <= 0 {
			return gerror.New("资料正在审核，请由审批管理员编辑")
		}
		review, err := tx.Model(profileReviewTable).Ctx(ctx).Where("profile_id", profileId).
			Where("tenant_id", tenantId).Where("account_id", accountId).Where("owner_account_id", adminID).
			Where("status", consts.ContentReviewPending).Fields("id").OrderDesc("id").LockUpdate().One()
		if err != nil {
			return err
		}
		if review.IsEmpty() {
			return gerror.New("审核状态已变化或当前管理员无权编辑")
		}
		pendingReviewID = review["id"].Int64()
	}
	in.Province, in.City = selectProfileRegions(regions, false, current[columns.Province].String(), current[columns.City].String(), current[columns.PlainText].String(), in.PlainText)
	// Editing content must not change lifecycle state. Publish/Unpublish/
	// Republish are the only operations allowed to transition it.
	nextStatus := current[columns.Status].Int()
	nextVisibility := current[columns.Visibility].String()
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
	if pendingReviewID > 0 && in.Title != current[columns.Title].String() {
		if _, err := tx.Model(profileReviewTable).Ctx(ctx).Where("id", pendingReviewID).Data("title", in.Title).Update(); err != nil {
			return gerror.Wrap(err, "更新待审核资料标题失败")
		}
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
