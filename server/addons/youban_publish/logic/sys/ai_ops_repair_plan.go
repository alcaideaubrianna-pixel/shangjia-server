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
	"recover_verifications": {},
	"requeue_events":        {},
	"queue_verification":    {},
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
	eventIDs, err := s.previewAIOpsRepairEvents(ctx, action, in.SourceId, in.EventIds, limit)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(struct {
		Action   string  `json:"action"`
		SourceID int64   `json:"sourceId"`
		EventIDs []int64 `json:"eventIds"`
	}{action, in.SourceId, eventIDs})
	if err != nil {
		return nil, gerror.Wrap(err, "生成修复计划失败")
	}
	planID := guid.S()
	expiresAt := gtime.Now().Add(aiOpsRepairPlanTTL)
	token, err := aiOpsRepairConfirmToken(planID, string(payload), expiresAt.Time)
	if err != nil {
		return nil, err
	}
	_, err = g.DB().Model(aiOpsRepairPlanTable).Ctx(ctx).Insert(g.Map{
		"plan_id": planID, "action": action, "payload_json": string(payload), "status": "pending",
		"candidate_count": len(eventIDs), "confirm_token_hash": collectHash(token), "expires_at": expiresAt,
		"created_at": gtime.Now(), "updated_at": gtime.Now(),
	})
	if err != nil {
		return nil, gerror.Wrap(err, "保存修复计划失败")
	}
	g.Log().Info(ctx, "AI运维修复计划已生成", g.Map{"planId": planID, "action": action, "candidates": len(eventIDs)})
	return &sysin.AIOpsRepairPlanModel{
		PlanId: planID, Action: action, Candidates: len(eventIDs), EventIds: eventIDs,
		ConfirmToken: token, ExpiresAt: expiresAt.Format(time.RFC3339),
	}, nil
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
	var payload struct {
		Action   string  `json:"action"`
		SourceID int64   `json:"sourceId"`
		EventIDs []int64 `json:"eventIds"`
	}
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
