package sys

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"

	collectorservice "hotgo/addons/telegram_collector/service"
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
	"replay_bot_source_updates":   {},
}

type aiOpsRepairPlanPayload struct {
	Action            string  `json:"action"`
	SourceID          int64   `json:"sourceId"`
	EventIDs          []int64 `json:"eventIds"`
	VerifyEventIDs    []int64 `json:"verifyEventIds"`
	CollectorEventIDs []int64 `json:"collectorEventIds,omitempty"`
	ProfileIDs        []int64 `json:"profileIds"`
	TenantID          int64   `json:"tenantId"`
	AccountID         int64   `json:"accountId"`
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
		if action == "replay_bot_source_updates" {
			limit = 1000
		}
	}
	if limit > 1000 {
		return nil, gerror.New("单次修复最多1000条事件")
	}
	var (
		err                                                     error
		eventIDs, verifyEventIDs, profileIDs, collectorEventIDs []int64
		replayPreview                                           []*sysin.AIOpsSourceReplayPreviewItem
		tenantID, accountID                                     int64
	)
	if action == "replay_bot_source_updates" {
		collectorEventIDs, replayPreview, tenantID, accountID, err = s.previewAIOpsBotSourceReplay(ctx, in.SourceId, in.CollectorEventIds, in.ChatId, in.StartMessageId, in.EndMessageId, limit)
	} else {
		eventIDs, verifyEventIDs, profileIDs, err = s.previewAIOpsRepairTargets(ctx, action, in, limit)
		tenantID, accountID = in.TenantId, in.AccountId
	}
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(aiOpsRepairPlanPayload{
		Action: action, SourceID: in.SourceId, EventIDs: eventIDs, VerifyEventIDs: verifyEventIDs, ProfileIDs: profileIDs,
		CollectorEventIDs: collectorEventIDs, TenantID: tenantID, AccountID: accountID,
	})
	if err != nil {
		return nil, gerror.Wrap(err, "生成修复计划失败")
	}
	candidateCount := len(eventIDs) + len(verifyEventIDs) + len(collectorEventIDs)
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
		CollectorEventIds: collectorEventIDs, ReplayPreview: replayPreview,
		ConfirmToken: token, ExpiresAt: expiresAt.Format(time.RFC3339),
	}, nil
}

// previewAIOpsBotSourceReplay 统一解析明确ID、频道或消息区间，生成有界重放清单。
func (s *sSysPublish) previewAIOpsBotSourceReplay(ctx context.Context, sourceID int64, requested []int64, chatID string, startMessageID, endMessageID int64, limit int) ([]int64, []*sysin.AIOpsSourceReplayPreviewItem, int64, int64, error) {
	ids := uniqueIds(requested)
	if sourceID <= 0 || len(ids) > limit {
		return nil, nil, 0, 0, gerror.Newf("目标source无效或事件数量超过%d条", limit)
	}
	var target struct {
		ID             int64  `json:"id"`
		TenantID       int64  `json:"tenantId"`
		AccountID      int64  `json:"accountId"`
		BotID          int64  `json:"botId"`
		SourceType     string `json:"sourceType"`
		CollectEnabled int    `json:"collectEnabled"`
		Status         int    `json:"status"`
	}
	sourceColumns := pdao.YoubanPublishCollectSource.Columns()
	if err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Fields(sourceColumns.Id, sourceColumns.TenantId, sourceColumns.AccountId, sourceColumns.BotId, sourceColumns.SourceType, sourceColumns.CollectEnabled, sourceColumns.Status).
		Where(sourceColumns.Id, sourceID).WhereNull(sourceColumns.DeletedAt).Scan(&target); err != nil {
		return nil, nil, 0, 0, gerror.Wrap(err, "读取目标采集source失败")
	}
	if target.ID <= 0 || target.TenantID <= 0 || target.AccountID <= 0 || target.BotID <= 0 || target.SourceType != sysin.CollectSourceTypeBot || target.CollectEnabled != 1 || target.Status != 1 {
		return nil, nil, 0, 0, gerror.New("目标采集source不存在、未启用或不是Bot source")
	}
	// 未指定事件ID时，按频道和消息区间从统一 Collector 事件表生成重放范围。
	if len(ids) == 0 {
		if strings.TrimSpace(chatID) == "" {
			return nil, nil, 0, 0, gerror.New("请指定Collector事件ID，或指定频道Chat ID")
		}
		if (startMessageID > 0 && endMessageID <= 0) || (startMessageID <= 0 && endMessageID > 0) || endMessageID < startMessageID && endMessageID > 0 {
			return nil, nil, 0, 0, gerror.New("消息区间必须同时提供有效的起止消息ID")
		}
		if startMessageID > 0 && endMessageID-startMessageID+1 > int64(limit)*10 {
			return nil, nil, 0, 0, gerror.New("消息区间过大，请缩小范围后重试")
		}
		var rows []struct{ ID int64 }
		model := g.DB().Model("hg_tg_collector_event").Ctx(ctx).
			Fields("id").Where("tenant_id", target.TenantID).Where("source_id", sourceID).
			Where("source_type", sysin.CollectSourceTypeBot).Where("chat_id", strings.TrimSpace(chatID))
		if startMessageID > 0 {
			model = model.WhereBetween("message_id", startMessageID, endMessageID)
		}
		model = model.OrderDesc("message_id").Limit(limit)

		if err := model.Scan(&rows); err != nil {
			return nil, nil, 0, 0, gerror.Wrap(err, "按频道消息区间读取重放事件失败")
		}
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
	}
	if !s.collectGlobalEnabled(ctx) {
		return nil, nil, 0, 0, gerror.New("采集总开关已关闭，当前不能重放")
	}
	if err := s.ensureTenantVipFeature(ctx, target.TenantID, sysin.TenantVipFeatureCollectSource); err != nil {
		return nil, nil, 0, 0, gerror.Wrap(err, "目标租户当前不能处理采集重放")
	}
	candidates, err := collectorservice.Collector().BotUpdateReplayCandidates(ctx, target.TenantID, ids)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	originIDs := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil || candidate.ID <= 0 || candidate.SourceID <= 0 || candidate.BotKey == "" || candidate.ChatID == "" || candidate.MessageID <= 0 {
			return nil, nil, 0, 0, gerror.New("原始Collector事件不是可重放的Bot消息")
		}
		originIDs = append(originIDs, candidate.SourceID)
	}
	originIDs = uniqueIds(originIDs)
	var origins []struct {
		ID    int64 `json:"id"`
		BotID int64 `json:"botId"`
	}
	if err = pdao.YoubanPublishCollectSource.Ctx(ctx).
		Fields(sourceColumns.Id, sourceColumns.BotId).Where(sourceColumns.TenantId, target.TenantID).
		WhereIn(sourceColumns.Id, originIDs).Where(sourceColumns.SourceType, sysin.CollectSourceTypeBot).
		WhereNull(sourceColumns.DeletedAt).Scan(&origins); err != nil {
		return nil, nil, 0, 0, gerror.Wrap(err, "校验原始事件Bot来源失败")
	}
	originBotIDs := make(map[int64]int64, len(origins))
	for _, origin := range origins {
		originBotIDs[origin.ID] = origin.BotID
	}
	for _, candidate := range candidates {
		if originBotIDs[candidate.SourceID] != target.BotID {
			return nil, nil, 0, 0, gerror.Newf("原始事件%d与目标source不属于同一个Bot", candidate.ID)
		}
	}

	preview := make([]*sysin.AIOpsSourceReplayPreviewItem, 0, len(candidates))
	type resolvedReplayRule struct {
		sourceID int64
		ruleID   int64
		name     string
	}
	rulesByChat := make(map[string]resolvedReplayRule)
	for _, candidate := range candidates {
		resolved, exists := rulesByChat[candidate.ChatID]
		if !exists {
			if err = s.validateAIOpsReplayChannelApproval(ctx, target.TenantID, target.AccountID, target.BotID, candidate.ChatID); err != nil {
				return nil, nil, 0, 0, err
			}
			event := gdb.Record{"id": gvar.New(int64(0)), "source_id": gvar.New(sourceID), "source_chat_id": gvar.New(candidate.ChatID)}
			rules, ruleErr := s.collectEventRules(ctx, event, target.TenantID, target.AccountID)
			if ruleErr != nil {
				return nil, nil, 0, 0, gerror.Wrapf(ruleErr, "预演原始事件%d的source规则失败", candidate.ID)
			}
			if len(rules) != 1 {
				return nil, nil, 0, 0, gerror.Newf("目标source%d无法解析到唯一专属规则", sourceID)
			}
			rule := rules[0]
			resolved = resolvedReplayRule{sourceID: collectRuleSourceID(event, rule), ruleID: rule["id"].Int64(), name: rule["name"].String()}
			rulesByChat[candidate.ChatID] = resolved
		}
		preview = append(preview, &sysin.AIOpsSourceReplayPreviewItem{
			CollectorEventId: candidate.ID, OriginSourceId: candidate.SourceID, UpdateId: candidate.UpdateID,
			ChatId: candidate.ChatID, MessageId: candidate.MessageID,
			RuleSourceId: resolved.sourceID, RuleId: resolved.ruleID, RuleName: resolved.name,
		})
	}
	return ids, preview, target.TenantID, target.AccountID, nil
}

func (s *sSysPublish) validateAIOpsReplayChannelApproval(ctx context.Context, tenantID, accountID, botID int64, chatID string) error {
	var channels []botCollectChannelCandidate
	channelColumns := pdao.YoubanPublishCollectChannel.Columns()
	if err := pdao.YoubanPublishCollectChannel.Ctx(ctx).
		Fields(channelColumns.Id, channelColumns.TelegramChatId, channelColumns.Title).Where(channelColumns.TenantId, tenantID).
		WhereIn(channelColumns.TelegramChatId, tgChannelCacheLookupIds(chatID)).
		WhereNull(channelColumns.DeletedAt).Scan(&channels); !collectOptionalRecordScanSucceeded(err) {
		return gerror.Wrap(err, "预演读取Telegram频道失败")
	}
	channelIDs := make([]int64, 0, len(channels))
	for _, channel := range channels {
		if channel.Id > 0 {
			channelIDs = append(channelIDs, channel.Id)
		}
	}
	if len(channelIDs) == 0 {
		return gerror.Newf("频道%s尚无可核验的账号审批关系，拒绝重放", chatID)
	}
	var relations []botCollectChannelRelation
	relationColumns := pdao.YoubanPublishCollectAccountChannel.Columns()
	if err := pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).
		Fields(relationColumns.Id, relationColumns.ChannelId, relationColumns.CollectEnabled, relationColumns.Status, relationColumns.ApprovalStatus).
		Where(relationColumns.TenantId, tenantID).Where(relationColumns.AccountId, accountID).
		WhereIn(relationColumns.ChannelId, channelIDs).WhereNull(relationColumns.DeletedAt).Scan(&relations); !collectOptionalRecordScanSucceeded(err) {
		return gerror.Wrap(err, "预演读取Telegram频道审批状态失败")
	}
	channel := selectBotCollectChannelForAccount(channels, relations, chatID)
	relation := selectBotCollectChannelRelation(relations, channel.Id)
	if channel.Id <= 0 || relation.Id <= 0 || strings.ToLower(strings.TrimSpace(relation.ApprovalStatus)) != "approved" || !botCollectChannelRelationEnabled(relation.CollectEnabled, relation.Status, relation.ApprovalStatus) {
		return gerror.Newf("频道%s在目标账号下没有已批准的采集关系，拒绝重放", chatID)
	}
	var botRelation struct {
		Status string `json:"status"`
	}
	botChannelColumns := pdao.YoubanPublishCollectBotChannel.Columns()
	if err := pdao.YoubanPublishCollectBotChannel.Ctx(ctx).
		Fields(botChannelColumns.Status).Where(botChannelColumns.TenantId, tenantID).Where(botChannelColumns.BotId, botID).
		Where(botChannelColumns.ChannelId, channel.Id).Scan(&botRelation); err != nil {
		return gerror.Wrap(err, "预演读取Bot频道关系失败")
	}
	if botRelation.Status == sysin.CollectBotChannelRemoved {
		return gerror.Newf("Bot已退出频道%s，拒绝重放", chatID)
	}
	return nil
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
	case "replay_bot_source_updates":
		var tenantID, accountID int64
		_, _, tenantID, accountID, err = s.previewAIOpsBotSourceReplay(ctx, payload.SourceID, payload.CollectorEventIDs, "", 0, 0, 1000)
		if err == nil && (tenantID != payload.TenantID || accountID != payload.AccountID) {
			err = gerror.New("目标source归属在计划预演后发生变化，请重新预演")
		}
		if err == nil {
			var replay *collectorservice.BotUpdateReplayResult
			replay, err = collectorservice.Collector().ReplayBotUpdatesForSource(ctx, tenantID, payload.SourceID, payload.CollectorEventIDs)
			if err == nil && replay != nil {
				result.Applied = replay.Queued
				result.Failed = replay.Failed
				result.Message = fmt.Sprintf("已入队%d条，已存在或正在处理%d条", replay.Queued, replay.Skipped)
			}
		}
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

func (s *sSysPublish) AIOpsApplySourceReplayPlan(ctx context.Context, in *sysin.AIOpsRepairApplyInp) (*sysin.AIOpsRepairApplyModel, error) {
	if in == nil {
		return nil, gerror.New("修复计划和确认令牌不能为空")
	}
	if err := s.validateAIOpsPlanAction(ctx, in.PlanId, "replay_bot_source_updates"); err != nil {
		return nil, err
	}
	return s.AIOpsApplyRepairPlan(ctx, in)
}

func (s *sSysPublish) AIOpsSourceReplayPlanStatus(ctx context.Context, in *sysin.AIOpsRepairPlanStatusInp) (*sysin.AIOpsRepairPlanStatusModel, error) {
	if in == nil {
		return nil, gerror.New("修复计划ID不能为空")
	}
	if err := s.validateAIOpsPlanAction(ctx, in.PlanId, "replay_bot_source_updates"); err != nil {
		return nil, err
	}
	return s.AIOpsRepairPlanStatus(ctx, in)
}

func (s *sSysPublish) validateAIOpsPlanAction(ctx context.Context, planID, action string) error {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return gerror.New("修复计划ID不能为空")
	}
	row, err := g.DB().Model(aiOpsRepairPlanTable).Ctx(ctx).Fields("action").Where("plan_id", planID).One()
	if err != nil {
		return gerror.Wrap(err, "读取修复计划失败")
	}
	if row.IsEmpty() {
		return gerror.New("修复计划不存在")
	}
	if row["action"].String() != action {
		return gerror.New("该计划不是source重放计划")
	}
	return nil
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
