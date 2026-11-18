package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
)

// collectWakeEventIdentity binds a plan to the persisted business event, not a
// Collector event or profile. Mutable processing state deliberately stays out.
// Keep these fields in progress queries too; no additional event lookup is needed.
func collectWakeEventIdentity(event gdb.Record) string {
	for _, field := range []string{"id", "tenant_id", "account_id", "source_id", "source_chat_id", "source_message_id", "source_unique_key", "created_at"} {
		if _, ok := event[field]; !ok {
			return ""
		}
	}
	body, _ := json.Marshal([]interface{}{
		event["id"].Int64(), event["tenant_id"].Int64(), event["account_id"].Int64(),
		event["source_id"].Int64(), event["source_chat_id"].String(), event["source_message_id"].Int64(),
		event["source_unique_key"].String(), event["created_at"].String(),
	})
	return collectHash(string(body))
}

func (s *sSysPublish) previewAIOpsCollectWake(ctx context.Context, in *sysin.AIOpsRepairPlanInp) ([]int64, []*sysin.AIOpsCollectWakePreviewItem, int64, int64, error) {
	ids, preview, _, _, err := s.loadAIOpsCollectWake(ctx, in)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	return ids, preview, in.TenantId, in.AccountId, nil
}

// loadAIOpsCollectWake reads all facts once, validates the entire scope before
// considering any event eligible, and caches authorization only within this batch.
func (s *sSysPublish) loadAIOpsCollectWake(ctx context.Context, in *sysin.AIOpsRepairPlanInp) ([]int64, []*sysin.AIOpsCollectWakePreviewItem, map[int64]gdb.Record, gdb.Record, error) {
	if in == nil || in.TenantId <= 0 || in.AccountId <= 0 || in.SourceId <= 0 || len(in.EventIds) == 0 {
		return nil, nil, nil, nil, gerror.New("请明确指定租户、来源所属账户、采集源和业务事件ID")
	}
	if len(in.CollectorEventIds) > 0 || len(in.ProfileIds) > 0 || len(in.ProfileNos) > 0 {
		return nil, nil, nil, nil, gerror.New("业务事件唤醒不能混用原始事件ID或资料ID")
	}
	if in.PrioritizeHistory && (in.RuleId <= 0 || len(in.EventIds) > 50) {
		return nil, nil, nil, nil, gerror.New("历史提优先级必须指定规则，最多50个业务事件")
	}
	if in.Limit < 0 || in.Limit > 200 {
		return nil, nil, nil, nil, gerror.New("业务事件唤醒上限必须在0到200之间")
	}
	limit := in.Limit
	if limit == 0 {
		limit = 200
	}
	ids := make([]int64, 0, min(len(in.EventIds), limit))
	seen := make(map[int64]bool)
	for _, id := range in.EventIds {
		if id <= 0 {
			return nil, nil, nil, nil, gerror.New("业务事件ID必须为正数")
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
			if len(ids) > limit {
				return nil, nil, nil, nil, gerror.Newf("本批业务事件数量超过指定上限%d条", limit)
			}
		}
	}
	sc := pdao.YoubanPublishCollectSource.Columns()
	source, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Fields(sc.Id, sc.TenantId, sc.AccountId, sc.SourceType, sc.BotId, sc.TgAccountId, sc.CollectEnabled, sc.Status, sc.ChannelBlacklisted).
		Where(sc.Id, in.SourceId).Where(sc.TenantId, in.TenantId).Where(sc.AccountId, in.AccountId).One()
	if err != nil {
		return nil, nil, nil, nil, gerror.Wrap(err, "读取唤醒采集源失败")
	}
	if source.IsEmpty() {
		return nil, nil, nil, nil, gerror.New("采集源不存在或不属于指定租户和账户")
	}
	ec := pdao.YoubanPublishCollectEvent.Columns()
	events, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields(ec.Id, ec.TenantId, ec.AccountId, ec.SourceId, ec.SourceType, ec.SourceChatId, ec.SourceMessageId,
			ec.SourceUniqueKey, ec.CreatedAt, ec.Status, ec.ProcessedAt, ec.ReceivedAt).
		WhereIn(ec.Id, ids).Limit(200).All()
	if err != nil {
		return nil, nil, nil, nil, gerror.Wrap(err, "批量读取待唤醒业务事件失败")
	}
	facts := make(map[int64]gdb.Record, len(events))
	for _, event := range events {
		if _, err = collectEventForBranch(event, source); err != nil ||
			(event[ec.AccountId].Int64() == 0 && source[sc.SourceType].String() != sysin.CollectSourceTypeBot) {
			return nil, nil, nil, nil, gerror.New("业务事件与指定租户、来源或账户归属不一致，整批拒绝唤醒")
		}
		facts[event[ec.Id].Int64()] = event
	}
	if len(facts) != len(ids) {
		return nil, nil, nil, nil, gerror.New("部分业务事件不存在，整批拒绝唤醒")
	}
	config, err := s.CollectConfig(ctx)
	if err != nil {
		return nil, nil, nil, nil, gerror.Wrap(err, "读取采集总开关失败")
	}
	vip, err := s.tenantVipStatus(ctx, in.TenantId)
	if err != nil {
		return nil, nil, nil, nil, gerror.Wrap(err, "读取采集VIP权限失败")
	}
	enabled := source[sc.CollectEnabled].Int() == 1 && source[sc.Status].Int() == 1 && source[sc.ChannelBlacklisted].Int() == 0 &&
		(config != nil && config.Enabled == 1) && tenantVipStatusActive(vip) && containsString(vip.Features, sysin.TenantVipFeatureCollectSource)
	allowedByChat := make(map[string]bool)
	preview := make([]*sysin.AIOpsCollectWakePreviewItem, 0, len(ids))
	for _, id := range ids {
		event := facts[id]
		item := &sysin.AIOpsCollectWakePreviewItem{
			EventId: id, TenantId: in.TenantId, AccountId: event[ec.AccountId].Int64(), OwnerAccountId: in.AccountId,
			SourceId: in.SourceId, SourceChatId: event[ec.SourceChatId].String(), SourceMessageId: event[ec.SourceMessageId].Int64(),
			Status: event[ec.Status].String(), Identity: collectWakeEventIdentity(event), Decision: sysin.CollectWakeBlocked,
		}
		preview = append(preview, item)
		switch item.Status {
		case sysin.CollectEventStatusProcessed, sysin.CollectEventStatusDispatched, "matched", sysin.CollectEventStatusIgnored:
			item.Decision = sysin.CollectWakeTerminal
			continue
		}
		if !event[ec.ProcessedAt].IsNil() {
			item.Decision = sysin.CollectWakeTerminal
			continue
		}
		switch item.Status {
		case sysin.CollectEventStatusPending, sysin.CollectEventStatusGroupCollect, sysin.CollectEventStatusWaitingOrder,
			sysin.CollectEventStatusPrechecked, sysin.CollectEventStatusMediaPending, sysin.CollectEventStatusMediaReady, sysin.CollectEventStatusFailed:
		default:
			continue
		}
		if !enabled || item.Identity == "" || item.SourceMessageId <= 0 {
			continue
		}
		chat := canonicalCollectSourceChatID(item.SourceChatId)
		if chat == "" {
			continue
		}
		allowed, checked := allowedByChat[chat]
		if !checked {
			access, accessErr := resolveCollectEventChannelAccess(ctx, event, in.AccountId)
			if accessErr != nil {
				return nil, nil, nil, nil, accessErr
			}
			allowed = access.Allowed
			if allowed && source[sc.BotId].Int64() > 0 {
				relation, relationErr := resolveCollectEventBotChannel(ctx, in.TenantId, source[sc.BotId].Int64(), access)
				if relationErr != nil {
					return nil, nil, nil, nil, relationErr
				}
				allowed = relation.Id <= 0 || relation.Status != sysin.CollectBotChannelRemoved
			}
			_, blocked, targetErr := s.checkCollectEventPublishTarget(ctx, event)
			if targetErr != nil {
				return nil, nil, nil, nil, targetErr
			}
			allowed = allowed && !blocked
			if allowed && in.PrioritizeHistory {
				rules, ruleErr := s.collectEventRules(ctx, event, in.TenantId, in.AccountId)
				if ruleErr != nil {
					return nil, nil, nil, nil, ruleErr
				}
				if len(rules) != 1 || rules[0]["id"].Int64() != in.RuleId || collectRuleSourceID(event, rules[0]) != in.SourceId {
					return nil, nil, nil, nil, gerror.New("频道未唯一绑定指定采集规则")
				}
			}
			allowedByChat[chat] = allowed
		}
		if allowed {
			item.Decision = sysin.CollectWakeEligible
		}
	}
	return ids, preview, facts, source, nil
}

func (s *sSysPublish) applyAIOpsCollectWake(ctx context.Context, payload aiOpsRepairPlanPayload, result *sysin.AIOpsRepairApplyModel) error {
	if result == nil {
		return gerror.New("缺少业务事件唤醒执行结果")
	}
	_, preview, facts, source, err := s.loadAIOpsCollectWake(ctx, &sysin.AIOpsRepairPlanInp{
		TenantId: payload.TenantID, AccountId: payload.AccountID, SourceId: payload.SourceID, EventIds: payload.EventIDs, RuleId: payload.RuleID, PrioritizeHistory: payload.PrioritizeHistory,
	})
	if err != nil {
		return err
	}
	original := make(map[int64]*sysin.AIOpsCollectWakePreviewItem, len(payload.CollectWakeEvents))
	for _, item := range payload.CollectWakeEvents {
		if item == nil || item.EventId <= 0 || original[item.EventId] != nil || !item.Decision.Valid() ||
			item.TenantId != payload.TenantID || item.OwnerAccountId != payload.AccountID || item.SourceId != payload.SourceID ||
			item.AccountId < 0 || (item.AccountId > 0 && item.AccountId != payload.AccountID) {
			return gerror.New("唤醒方案的事件签名事实无效")
		}
		original[item.EventId] = item
	}
	if len(original) != len(payload.EventIDs) {
		return gerror.New("唤醒方案的事件签名数量与事件ID数量不一致")
	}
	plannedIDs := make(map[int64]bool, len(payload.EventIDs))
	for _, id := range payload.EventIDs {
		if original[id] == nil || plannedIDs[id] {
			return gerror.New("唤醒方案的事件ID与签名事实不一致")
		}
		plannedIDs[id] = true
	}
	if payload.PrioritizeHistory {
		// Each priority insertion goes to the front. Reverse the batch so it
		// keeps the same relative order as the approved event list.
		slices.Reverse(preview)
	}
	accepted, skipped, failed := 0, 0, 0
	for _, item := range preview {
		before := original[item.EventId]
		if before == nil || before.Identity == "" || before.Identity != item.Identity ||
			before.AccountId != item.AccountId || before.SourceChatId != item.SourceChatId || before.SourceMessageId != item.SourceMessageId {
			result.Failed = append(result.Failed, item.EventId)
			failed++
			continue
		}
		// Never expand the effect approved in preview, even when permissions or
		// status have changed. Completion is never reset by this entry point.
		if before.Decision != sysin.CollectWakeEligible || item.Decision != sysin.CollectWakeEligible {
			skipped++
			continue
		}
		wake := collectProcessQueuePayload{History: true, PrioritizeHistory: payload.PrioritizeHistory, EventId: item.EventId, TenantId: item.TenantId,
			AccountId: item.AccountId, SourceId: item.SourceId, SourceChatId: item.SourceChatId}
		wake = captureCollectProcessObservation(collectBusinessEventContext(ctx, facts[item.EventId], source, 0), wake, false, time.Now())
		if err = s.enqueueCollectProcess(ctx, wake, 0); err != nil {
			result.Failed = append(result.Failed, item.EventId)
			failed++
			continue
		}
		accepted++
	}
	result.Applied = accepted
	result.Message = fmt.Sprintf("业务事件唤醒：已入队%d条，跳过%d条，失败%d条（含身份变化或签名缺失）；入队不代表资料处理完成", accepted, skipped, failed)
	return nil
}
