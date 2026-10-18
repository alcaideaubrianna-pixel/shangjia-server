package sys

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const (
	aiOpsRepairPlanTable = "hg_youban_publish_ai_ops_repair_plan"
	aiOpsRepairPlanTTL   = 15 * time.Minute
)

var aiOpsRepairActions = map[string]struct{}{
	"recover_verifications":       {},
	"requeue_events":              {},
	"queue_verification":          {},
	"repair_missing_verification": {},
	"offline_profiles":            {},
	"republish_profiles":          {},
	"delete_profiles":             {},
	"repair_profile_media":        {},
}

type aiOpsRepairPlanPayload struct {
	Action         string  `json:"action"`
	SourceID       int64   `json:"sourceId"`
	EventIDs       []int64 `json:"eventIds"`
	VerifyEventIDs []int64 `json:"verifyEventIds"`
	ProfileIDs     []int64 `json:"profileIds"`
	TenantID       int64   `json:"tenantId"`
	AccountID      int64   `json:"accountId"`
}

func (s *sSysPublish) AIOpsCreateRepairPlan(ctx context.Context, in *sysin.AIOpsRepairPlanInp) (*sysin.AIOpsRepairPlanModel, error) {
	if in == nil {
		return nil, gerror.New("修复计划参数不能为空")
	}
	action := strings.TrimSpace(in.Action)
	if _, ok := aiOpsRepairActions[action]; !ok {
		return nil, gerror.New("修复动作不受支持")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return nil, gerror.New("单次修复最多1000条事件")
	}
	eventIDs, verifyEventIDs, profileIDs, err := s.previewAIOpsRepairTargets(ctx, action, in, limit)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(aiOpsRepairPlanPayload{
		Action: action, SourceID: in.SourceId, EventIDs: eventIDs, VerifyEventIDs: verifyEventIDs, ProfileIDs: profileIDs,
		TenantID: in.TenantId, AccountID: in.AccountId,
	})
	if err != nil {
		return nil, gerror.Wrap(err, "生成修复计划失败")
	}
	candidateCount := len(eventIDs) + len(verifyEventIDs)
	if candidateCount == 0 {
		candidateCount = len(profileIDs)
	}
	planID := guid.S()
	expiresAt := gtime.Now().Add(aiOpsRepairPlanTTL)
	token, err := aiOpsRepairConfirmToken(planID, string(payload), expiresAt.Time)
	if err != nil {
		return nil, err
	}
	_, err = g.DB().Model(aiOpsRepairPlanTable).Ctx(ctx).Insert(g.Map{
		"plan_id": planID, "action": action, "payload_json": string(payload), "status": "pending",
		"candidate_count": candidateCount, "confirm_token_hash": collectHash(token), "expires_at": expiresAt,
		"created_at": gtime.Now(), "updated_at": gtime.Now(),
	})
	if err != nil {
		return nil, gerror.Wrap(err, "保存修复计划失败")
	}
	g.Log().Info(ctx, "AI运维修复计划已生成", g.Map{"planId": planID, "action": action, "candidates": candidateCount})
	return &sysin.AIOpsRepairPlanModel{
		PlanId: planID, Action: action, Candidates: candidateCount, EventIds: eventIDs, VerifyEventIds: verifyEventIDs, ProfileIds: profileIDs,
		ConfirmToken: token, ExpiresAt: expiresAt.Format(time.RFC3339),
	}, nil
}

func (s *sSysPublish) previewAIOpsRepairTargets(ctx context.Context, action string, in *sysin.AIOpsRepairPlanInp, limit int) ([]int64, []int64, []int64, error) {
	switch action {
	case "offline_profiles", "republish_profiles", "delete_profiles", "repair_profile_media":
		ids, err := s.resolveAIOpsRepairProfileIDs(ctx, in.ProfileIds, in.ProfileNos, limit)
		if err != nil {
			return nil, nil, nil, err
		}
		if action == "delete_profiles" {
			ids, err = s.AIOpsDeleteImportedProfiles(ctx, in.TenantId, in.AccountId, ids, true)
			if err != nil {
				return nil, nil, nil, err
			}
		}
		if action == "repair_profile_media" {
			if len(ids) > 20 {
				return nil, nil, nil, gerror.New("单次只能修复1到20条资料媒体")
			}
			result, repairErr := s.AIOpsRebuildProfileMedia(ctx, ids, true)
			if repairErr != nil {
				return nil, nil, nil, repairErr
			}
			ids = uniqueIds(result.ProfileIDs)
		}
		if action == "republish_profiles" {
			if len(ids) > 20 {
				return nil, nil, nil, gerror.New("单次只能重新上架1到20条资料")
			}
			for _, id := range ids {
				if _, err = s.profilePublishSource(ctx, id, 0, 0, false); err != nil {
					return nil, nil, nil, err
				}
				complete, completeErr := collectProfileMediaComplete(ctx, id)
				if completeErr != nil {
					return nil, nil, nil, completeErr
				}
				if !complete {
					return nil, nil, nil, gerror.Newf("资料%d媒体尚未恢复完整，禁止重新上架", id)
				}
			}
		}
		return nil, nil, ids, nil
	case "repair_missing_verification":
		eventIDs, verifyEventIDs, profileIDs, err := s.previewAIOpsMissingVerificationEvents(ctx, in, limit)
		return eventIDs, verifyEventIDs, profileIDs, err
	default:
		ids, err := s.previewAIOpsRepairEvents(ctx, action, in.SourceId, in.EventIds, limit)
		return ids, nil, nil, err
	}
}

func (s *sSysPublish) resolveAIOpsRepairProfileIDs(ctx context.Context, requested []int64, profileNos []string, limit int) ([]int64, error) {
	ids := uniqueIds(requested)
	nos := uniqueStrings(profileNos)
	if len(nos) > 0 {
		rows, err := g.DB().Model("hg_content_profile").Ctx(ctx).Fields("id,profile_no").
			WhereIn("profile_no", nos).WhereNull("deleted_at").All()
		if err != nil {
			return nil, gerror.Wrap(err, "按编号读取资料失败")
		}
		if len(rows) != len(nos) {
			return nil, gerror.Newf("资料编号校验失败：请求%d条，仅找到%d条", len(nos), len(rows))
		}
		for _, row := range rows {
			ids = append(ids, row["id"].Int64())
		}
		ids = uniqueIds(ids)
	}
	if len(ids) == 0 || len(ids) > limit {
		return nil, gerror.Newf("资料数量必须为1到%d条", limit)
	}
	return ids, nil
}

func (s *sSysPublish) previewAIOpsMissingVerificationEvents(ctx context.Context, in *sysin.AIOpsRepairPlanInp, limit int) ([]int64, []int64, []int64, error) {
	startAt, err := time.Parse(time.RFC3339, strings.TrimSpace(in.StartAt))
	if err != nil {
		return nil, nil, nil, gerror.New("缺验证修复必须提供RFC3339格式的开始时间")
	}
	endAt, err := time.Parse(time.RFC3339, strings.TrimSpace(in.EndAt))
	if err != nil || !endAt.After(startAt) || endAt.Sub(startAt) > 48*time.Hour {
		return nil, nil, nil, gerror.New("缺验证修复结束时间不合法，单次时间范围不能超过48小时")
	}
	model := g.DB().Model(pdao.YoubanPublishCollectEvent.Table()+" e").Ctx(ctx).
		Fields("e.id,d.profile_id").
		InnerJoin("hg_youban_publish_collect_dispatch d", "d.event_id=e.id").
		Where("e.material_role", collectMaterialRoleDisplay).
		Where("COALESCE(e.received_at,e.created_at) >= ?", startAt).
		Where("COALESCE(e.received_at,e.created_at) < ?", endAt).
		Where("EXISTS (SELECT 1 FROM "+pdao.YoubanPublishCollectEvent.Table()+" v WHERE v.material_parent_event_id=e.id AND v.material_role=?)", collectMaterialRoleVerify).
		Where("EXISTS (SELECT 1 FROM "+publishTgJobTable+" j WHERE j.profile_id=d.profile_id AND j.collect_event_id=e.id AND j.status=? AND NOT EXISTS (SELECT 1 FROM "+publishTgMessageTable+" m WHERE m.job_id=j.id AND m.purpose=? AND m.status='sent' AND m.deleted_at IS NULL))", telegramJobStatusSent, collectMaterialRoleVerify).
		Group("e.id,d.profile_id")
	if in.SourceId > 0 {
		model = model.Where("e.source_id", in.SourceId)
	}
	rows, err := model.OrderAsc("e.id").Limit(limit + 1).All()
	if err != nil {
		return nil, nil, nil, gerror.Wrap(err, "读取缺验证视频资料失败")
	}
	if len(rows) > limit {
		return nil, nil, nil, gerror.Newf("缺验证视频候选超过安全上限%d，请缩小时间或采集源范围", limit)
	}
	eventIDs := make([]int64, 0, len(rows))
	profileIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		eventIDs = append(eventIDs, row["id"].Int64())
		profileIDs = append(profileIDs, row["profile_id"].Int64())
	}
	verifyModel := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("status", sysin.CollectEventStatusIgnored).
		Where("material_role", collectMaterialRoleVerify).
		Where("error_message", collectMaterialVerifyUnmatchedMessage).
		Where("COALESCE(received_at,created_at) >= ?", startAt).
		Where("COALESCE(received_at,created_at) < ?", endAt)
	if in.SourceId > 0 {
		verifyModel = verifyModel.Where("source_id", in.SourceId)
	}
	verifyRows, err := verifyModel.OrderAsc("id").Limit(limit + 1).All()
	if err != nil {
		return nil, nil, nil, gerror.Wrap(err, "读取未绑定验证事件失败")
	}
	verifyEventIDs := make([]int64, 0)
	for _, verify := range verifyRows {
		display, findErr := s.findStoredCollectDisplayEvent(ctx, verify)
		if findErr != nil {
			return nil, nil, nil, findErr
		}
		if display.IsEmpty() {
			continue
		}
		canonical, canonicalErr := s.isCanonicalCollectMaterialPair(ctx, display, verify)
		if canonicalErr != nil {
			return nil, nil, nil, canonicalErr
		}
		if !canonical {
			continue
		}
		dispatch, dispatchErr := pdao.YoubanPublishCollectDispatch.Ctx(ctx).Fields("profile_id").Where("event_id", display["id"].Int64()).One()
		if dispatchErr != nil {
			return nil, nil, nil, gerror.Wrap(dispatchErr, "读取未绑定验证资料分发记录失败")
		}
		if dispatch["profile_id"].Int64() <= 0 {
			continue
		}
		verifyEventIDs = append(verifyEventIDs, verify["id"].Int64())
		profileIDs = append(profileIDs, dispatch["profile_id"].Int64())
		if len(uniqueIds(eventIDs))+len(uniqueIds(verifyEventIDs)) > limit {
			return nil, nil, nil, gerror.Newf("缺验证视频候选超过安全上限%d，请缩小时间或采集源范围", limit)
		}
	}
	return uniqueIds(eventIDs), uniqueIds(verifyEventIDs), uniqueIds(profileIDs), nil
}

func (s *sSysPublish) previewAIOpsRepairEvents(ctx context.Context, action string, sourceID int64, requested []int64, limit int) ([]int64, error) {
	ids := uniqueIds(requested)
	if len(ids) > limit {
		return nil, gerror.Newf("请求事件超过安全上限%d", limit)
	}
	model := pdao.YoubanPublishCollectEvent.Ctx(ctx).Fields("id")
	if len(ids) > 0 {
		model = model.WhereIn("id", ids)
	} else {
		if sourceID <= 0 {
			return nil, gerror.New("未指定事件时必须提供采集源ID")
		}
		model = model.Where("source_id", sourceID)
	}
	switch action {
	case "recover_verifications":
		model = model.Where("status", sysin.CollectEventStatusIgnored).
			Where("material_role", collectMaterialRoleVerify).
			Where("error_message", collectMaterialVerifyUnmatchedMessage)
	case "requeue_events", "queue_verification":
		model = model.Where("material_role", collectMaterialRoleDisplay)
	}
	rows, err := model.OrderAsc("id").Limit(limit + 1).All()
	if err != nil {
		return nil, gerror.Wrap(err, "预演修复事件失败")
	}
	if len(rows) > limit {
		return nil, gerror.Newf("候选事件超过安全上限%d，请缩小范围", limit)
	}
	result := make([]int64, 0, len(rows))
	for _, row := range rows {
		result = append(result, row["id"].Int64())
	}
	return result, nil
}

func (s *sSysPublish) AIOpsApplyRepairPlan(ctx context.Context, in *sysin.AIOpsRepairApplyInp) (*sysin.AIOpsRepairApplyModel, error) {
	if in == nil || strings.TrimSpace(in.PlanId) == "" || strings.TrimSpace(in.ConfirmToken) == "" {
		return nil, gerror.New("修复计划和确认令牌不能为空")
	}
	row, err := g.DB().Model(aiOpsRepairPlanTable).Ctx(ctx).Where("plan_id", in.PlanId).One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取修复计划失败")
	}
	if row.IsEmpty() || row["status"].String() != "pending" {
		return nil, gerror.New("修复计划不存在或已执行")
	}
	if expires := row["expires_at"].GTime(); expires == nil || expires.Before(gtime.Now()) {
		return nil, gerror.New("修复计划已过期，请重新预演")
	}
	if !hmac.Equal([]byte(row["confirm_token_hash"].String()), []byte(collectHash(in.ConfirmToken))) {
		return nil, gerror.New("修复确认令牌无效")
	}
	var payload aiOpsRepairPlanPayload
	if err = json.Unmarshal([]byte(row["payload_json"].String()), &payload); err != nil {
		return nil, gerror.Wrap(err, "解析修复计划失败")
	}
	claim, err := g.DB().Model(aiOpsRepairPlanTable).Ctx(ctx).Where("plan_id", in.PlanId).Where("status", "pending").
		Data(g.Map{"status": "running", "started_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
	if err != nil {
		return nil, gerror.Wrap(err, "领取修复计划失败")
	}
	affected, _ := claim.RowsAffected()
	if affected == 0 {
		return nil, gerror.New("修复计划已被其他执行器领取")
	}
	result := &sysin.AIOpsRepairApplyModel{PlanId: in.PlanId, Status: "completed", Failed: []int64{}}
	switch payload.Action {
	case "recover_verifications":
		result.Applied, result.Failed, err = s.applyIgnoredVerificationEvents(ctx, payload.EventIDs)
	case "requeue_events":
		result.Applied, result.Failed, err = RequeueCollectEventsBestEffort(ctx, payload.EventIDs)
	case "queue_verification":
		result.Applied, err = s.AIOpsQueueCollectVerification(ctx, payload.EventIDs, false)
	case "repair_missing_verification":
		var recovered int
		var recoveryFailed []int64
		recovered, recoveryFailed, err = s.applyIgnoredVerificationEvents(ctx, payload.VerifyEventIDs)
		result.Applied, result.Failed = recovered, append(result.Failed, recoveryFailed...)
		if err == nil {
			var replayed int
			var replayFailed []int64
			replayed, replayFailed, err = RequeueCollectEventsBestEffort(ctx, payload.EventIDs)
			result.Applied += replayed
			result.Failed = append(result.Failed, replayFailed...)
		}
	case "offline_profiles":
		result.Applied, result.Failed = s.applyAIOpsProfilePlan(ctx, payload.Action, payload.ProfileIDs, payload.TenantID, payload.AccountID)
	case "republish_profiles":
		result.Applied, result.Failed = s.applyAIOpsProfilePlan(ctx, payload.Action, payload.ProfileIDs, payload.TenantID, payload.AccountID)
	case "delete_profiles":
		result.Applied, result.Failed = s.applyAIOpsProfilePlan(ctx, payload.Action, payload.ProfileIDs, payload.TenantID, payload.AccountID)
	case "repair_profile_media":
		result.Applied, result.Failed = s.applyAIOpsProfilePlan(ctx, payload.Action, payload.ProfileIDs, payload.TenantID, payload.AccountID)
	}
	if err != nil {
		result.Status, result.Message = "failed", err.Error()
	}
	_, updateErr := g.DB().Model(aiOpsRepairPlanTable).Ctx(ctx).Where("plan_id", in.PlanId).Data(g.Map{
		"status": result.Status, "applied_count": result.Applied, "failed_ids_json": mustJSONString(result.Failed),
		"error_message": result.Message, "finished_at": gtime.Now(), "updated_at": gtime.Now(),
	}).Update()
	if updateErr != nil {
		return nil, gerror.Wrap(updateErr, "记录修复结果失败")
	}
	g.Log().Info(ctx, "AI运维修复计划执行完成", g.Map{"planId": in.PlanId, "result": result})
	return result, err
}

func (s *sSysPublish) applyAIOpsProfilePlan(ctx context.Context, action string, profileIDs []int64, tenantID, accountID int64) (int, []int64) {
	applied := 0
	failed := make([]int64, 0)
	for _, profileID := range uniqueIds(profileIDs) {
		var err error
		switch action {
		case "offline_profiles":
			_, err = s.AIOpsOfflineProfiles(ctx, []int64{profileID})
		case "republish_profiles":
			_, err = s.AIOpsRepublishProfiles(ctx, &sysin.ProfileStatusInp{Ids: []int64{profileID}, Status: 1})
		case "delete_profiles":
			_, err = s.AIOpsDeleteImportedProfiles(ctx, tenantID, accountID, []int64{profileID}, false)
		case "repair_profile_media":
			_, err = s.AIOpsRebuildProfileMedia(ctx, []int64{profileID}, false)
		}
		if err != nil {
			failed = append(failed, profileID)
			g.Log().Warningf(ctx, "AI运维资料操作失败 action:%s profileId:%d err:%+v", action, profileID, err)
			continue
		}
		applied++
	}
	return applied, failed
}

func (s *sSysPublish) AIOpsRepairPlanStatus(ctx context.Context, in *sysin.AIOpsRepairPlanStatusInp) (*sysin.AIOpsRepairPlanStatusModel, error) {
	if in == nil || strings.TrimSpace(in.PlanId) == "" {
		return nil, gerror.New("修复计划ID不能为空")
	}
	row, err := g.DB().Model(aiOpsRepairPlanTable).Ctx(ctx).Where("plan_id", strings.TrimSpace(in.PlanId)).One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取修复计划状态失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("修复计划不存在")
	}
	failed := make([]int64, 0)
	if raw := strings.TrimSpace(row["failed_ids_json"].String()); raw != "" {
		_ = json.Unmarshal([]byte(raw), &failed)
	}
	formatTime := func(field string) string {
		if value := row[field].GTime(); value != nil && !value.IsZero() {
			return value.Format(time.RFC3339)
		}
		return ""
	}
	return &sysin.AIOpsRepairPlanStatusModel{
		PlanId: row["plan_id"].String(), Action: row["action"].String(), Status: row["status"].String(),
		Candidates: row["candidate_count"].Int(), Applied: row["applied_count"].Int(), Failed: failed,
		Message: row["error_message"].String(), ExpiresAt: formatTime("expires_at"),
		StartedAt: formatTime("started_at"), FinishedAt: formatTime("finished_at"),
	}, nil
}

func (s *sSysPublish) applyIgnoredVerificationEvents(ctx context.Context, eventIDs []int64) (int, []int64, error) {
	processed, failed := 0, make([]int64, 0)
	for _, eventID := range eventIDs {
		verify, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(eventID).One()
		if err != nil || verify.IsEmpty() {
			failed = append(failed, eventID)
			continue
		}
		display, err := s.findStoredCollectDisplayEvent(ctx, verify)
		if err != nil || display.IsEmpty() {
			failed = append(failed, eventID)
			continue
		}
		paired, err := s.bindCollectMaterialPair(ctx, display, verify)
		if err != nil || !paired {
			failed = append(failed, eventID)
			continue
		}
		if err = s.scheduleCollectDisplaySettlement(ctx, collectProcessQueuePayload{
			TenantId: verify["tenant_id"].Int64(), AccountId: verify["account_id"].Int64(), SourceId: verify["source_id"].Int64(),
			SourceChatId: verify["source_chat_id"].String(),
		}, display, s.collectDisplayRetryDelay(ctx, display)); err != nil {
			failed = append(failed, eventID)
			continue
		}
		processed++
	}
	return processed, failed, nil
}

func aiOpsRepairConfirmToken(planID, payload string, expiresAt time.Time) (string, error) {
	secret := strings.TrimSpace(os.Getenv("YOUBAN_AI_OPS_TOKEN"))
	if secret == "" {
		return "", gerror.New("AI运维凭证未配置")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(planID + "\n" + payload + "\n" + expiresAt.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func mustJSONString(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
