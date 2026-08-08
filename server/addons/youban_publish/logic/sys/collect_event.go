package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/hgrds/lock"
)

const (
	collectEventRulesCacheVersionKey = "youban_publish:collect:event_rules:version"
	collectEventRulesCacheKeyPrefix  = "youban_publish:collect:event_rules"
	collectEventRulesCacheTTL        = time.Minute
	collectEventRetentionDays        = 3
)

func (s *sSysPublish) CollectEventList(ctx context.Context, in *sysin.CollectEventListInp) (list []*sysin.CollectEventModel, totalCount int, err error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.CollectEventListInp{}
	}
	mod := pdao.YoubanPublishCollectEvent.DB().Model(pdao.YoubanPublishCollectEvent.Table()+" e").Safe().Ctx(ctx).
		LeftJoin(pdao.YoubanPublishCollectSource.Table()+" s", "s.id=e.source_id").
		Where("e.tenant_id", account.TenantId).
		Where("e.account_id", account.Id).
		WhereGTE("e.created_at", gtime.Now().Add(-time.Duration(collectEventRetentionDays)*24*time.Hour))
	if in.SourceId > 0 {
		mod = mod.Where("e.source_id", in.SourceId)
	}
	if in.Status != "" {
		mod = mod.Where("e.status", strings.TrimSpace(in.Status))
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		mod = mod.WhereLike("e.raw_text", "%"+keyword+"%")
	}
	if totalCount, err = mod.Count(); err != nil {
		return nil, 0, gerror.Wrap(err, "统计采集事件失败")
	}
	fields := "e.*,s.title AS source_title"
	if err = mod.Fields(fields).Page(in.Page, in.PerPage).OrderDesc("e.id").Scan(&list); err != nil {
		return nil, 0, gerror.Wrap(err, "获取采集事件失败")
	}
	if err = s.fillCollectEventRouting(ctx, list, account.TenantId, account.Id); err != nil {
		return nil, 0, err
	}
	if err = s.fillCollectEventMediaCache(ctx, list); err != nil {
		return nil, 0, err
	}
	for _, item := range list {
		item.IgnoreType = collectEventIgnoreType(item.Status, item.ErrorMessage)
	}
	return
}

func (s *sSysPublish) fillCollectEventMediaCache(ctx context.Context, list []*sysin.CollectEventModel) error {
	eventIds := make([]int64, 0, len(list))
	byId := make(map[int64]*sysin.CollectEventModel, len(list))
	for _, item := range list {
		if item != nil && item.Id > 0 {
			eventIds = append(eventIds, item.Id)
			byId[item.Id] = item
		}
	}
	if len(eventIds) == 0 {
		return nil
	}
	rows, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Fields("event_id,cache_status,COUNT(*) AS total").
		WhereIn("event_id", eventIds).
		Group("event_id,cache_status").All()
	if err != nil {
		return gerror.Wrap(err, "统计采集事件媒体状态失败")
	}
	summaries := make(map[int64]collectEventMediaCacheSummary, len(eventIds))
	for _, row := range rows {
		eventId := row["event_id"].Int64()
		summary := summaries[eventId]
		count := row["total"].Int()
		summary.Total += count
		switch row["cache_status"].String() {
		case collectMediaCacheReady:
			summary.Ready += count
		case collectMediaCacheFailed:
			summary.Failed += count
		case collectMediaCacheForwarding, collectMediaCacheDownloading:
			summary.Downloading += count
		default:
			summary.Pending += count
		}
		summaries[eventId] = summary
	}
	for eventId, item := range byId {
		item.MediaCacheStatus, item.MediaCacheMessage = collectEventMediaCacheView(summaries[eventId], item.Status, item.ErrorMessage)
	}
	return nil
}

func collectEventIgnoreType(status, message string) string {
	if status != sysin.CollectEventStatusIgnored {
		return ""
	}
	if strings.Contains(message, "重复") {
		return sysin.CollectEventIgnoreTypeDedupe
	}
	return sysin.CollectEventIgnoreTypeMatch
}

func (s *sSysPublish) fillCollectEventRouting(ctx context.Context, list []*sysin.CollectEventModel, tenantId, accountId int64) error {
	if len(list) == 0 {
		return nil
	}
	eventIds := make([]int64, 0, len(list))
	sourceIds := make([]int64, 0, len(list))
	for _, item := range list {
		if item == nil {
			continue
		}
		eventIds = append(eventIds, item.Id)
		sourceIds = append(sourceIds, item.SourceId)
	}
	if len(eventIds) == 0 {
		return nil
	}

	type route struct {
		ChannelIds []int64
		ReviewId   int64
		Review     string
		Dispatch   string
	}
	routes := make(map[int64]*route, len(eventIds))
	for _, id := range eventIds {
		routes[id] = &route{}
	}
	var dispatches []struct {
		Id       int64  `json:"id"`
		EventId  int64  `json:"eventId"`
		ReviewId int64  `json:"reviewId"`
		Status   string `json:"status"`
	}
	if err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("event_id", eventIds).
		Fields("id,event_id,review_id,status").
		Scan(&dispatches); err != nil {
		return gerror.Wrap(err, "读取采集事件分发信息失败")
	}
	dispatchIds := make([]int64, 0, len(dispatches))
	for _, item := range dispatches {
		dispatchIds = append(dispatchIds, item.Id)
	}
	dispatchChannels, err := collectDispatchChannelMap(ctx, dispatchIds)
	if err != nil {
		return err
	}
	channelIds := make([]int64, 0)
	for _, item := range dispatches {
		r := routes[item.EventId]
		if r == nil {
			continue
		}
		r.ChannelIds = append(r.ChannelIds, dispatchChannels[item.Id]...)
		r.Dispatch = item.Status
		if item.ReviewId > r.ReviewId {
			r.ReviewId = item.ReviewId
		}
	}
	reviewIds := make([]int64, 0)
	for _, r := range routes {
		if r.ReviewId > 0 {
			reviewIds = append(reviewIds, r.ReviewId)
		}
	}
	if len(reviewIds) > 0 {
		var reviews []struct {
			Id     int64  `json:"id"`
			Status string `json:"status"`
		}
		if err := pdao.YoubanPublishCollectReview.Ctx(ctx).
			Where("tenant_id", tenantId).Where("account_id", accountId).
			WhereIn("id", reviewIds).Fields("id,status").Scan(&reviews); err != nil {
			return gerror.Wrap(err, "读取采集事件审核信息失败")
		}
		for _, review := range reviews {
			for _, r := range routes {
				if r.ReviewId == review.Id {
					r.Review = review.Status
				}
			}
		}
	}

	// 事件尚未生成分发记录时，展示采集源绑定规则的候选目标，避免界面误认为没有配置目标。
	var bindings []struct {
		SourceId int64 `json:"sourceId"`
		RuleId   int64 `json:"ruleId"`
	}
	if err := pdao.YoubanPublishCollectSourceRule.Ctx(ctx).
		WhereIn("source_id", sourceIds).Where("status", 1).
		Fields("source_id,rule_id").Scan(&bindings); err != nil {
		return gerror.Wrap(err, "读取采集源目标规则失败")
	}
	ruleIds := make([]int64, 0, len(bindings))
	for _, binding := range bindings {
		ruleIds = append(ruleIds, binding.RuleId)
	}
	var rules []struct {
		Id int64 `json:"id"`
	}
	if len(ruleIds) > 0 {
		if err := pdao.YoubanPublishCollectRule.Ctx(ctx).
			Where("tenant_id", tenantId).Where("account_id", accountId).
			WhereIn("id", ruleIds).Where("status", 1).WhereNull("deleted_at").
			Fields("id").Scan(&rules); err != nil {
			return gerror.Wrap(err, "读取采集规则目标失败")
		}
	}
	ruleTargetIds := make([]int64, 0, len(rules))
	for _, rule := range rules {
		ruleTargetIds = append(ruleTargetIds, rule.Id)
	}
	ruleTargets, err := collectRuleChannelMap(ctx, ruleTargetIds)
	if err != nil {
		return err
	}
	sourceTargets := make(map[int64][]int64)
	for _, binding := range bindings {
		sourceTargets[binding.SourceId] = append(sourceTargets[binding.SourceId], ruleTargets[binding.RuleId]...)
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		r := routes[item.Id]
		if len(r.ChannelIds) == 0 {
			r.ChannelIds = sourceTargets[item.SourceId]
		}
		r.ChannelIds = uniqueIds(r.ChannelIds)
		channelIds = append(channelIds, r.ChannelIds...)
		item.TargetChannelIds = r.ChannelIds
		item.ReviewStatus = r.Review
		item.DispatchStatus = r.Dispatch
	}
	channelIds = uniqueIds(channelIds)
	if len(channelIds) == 0 {
		return nil
	}
	var channels []struct {
		Id           int64  `json:"id"`
		ChannelTitle string `json:"channelTitle"`
	}
	if err := pdao.YoubanPublishChannel.Ctx(ctx).
		Where("tenant_id", tenantId).WhereIn("id", channelIds).WhereNull("deleted_at").
		Fields("id,channel_title").Scan(&channels); err != nil {
		return gerror.Wrap(err, "读取采集目标频道失败")
	}
	names := make(map[int64]string, len(channels))
	for _, channel := range channels {
		names[channel.Id] = strings.TrimSpace(channel.ChannelTitle)
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		for _, id := range item.TargetChannelIds {
			if name := names[id]; name != "" {
				item.TargetChannelNames = append(item.TargetChannelNames, name)
			}
		}
	}
	return nil
}

func (s *sSysPublish) CollectEventClear(ctx context.Context, in *sysin.CollectEventClearInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || in.SourceId <= 0 {
		return gerror.New("采集源ID不能为空")
	}
	_, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("tenant_id", account.TenantId).
		Where("account_id", account.Id).
		Where("source_id", in.SourceId).
		Delete()
	return gerror.Wrap(err, "清空采集事件失败")
}

func (s *sSysPublish) processCollectEvent(ctx context.Context, eventId int64, tenantId int64, accountId int64) error {
	eventLock := lock.NewConfig(15*time.Minute, time.Second).Mutex(fmt.Sprintf("youban_publish:collect:event:%d", eventId))
	if lockErr := eventLock.TryLock(ctx); lockErr != nil {
		if errors.Is(lockErr, lock.ErrLockFailed) {
			g.Log().Debugf(ctx, "采集事件已有实例处理，跳过本次执行 eventId:%d", eventId)
			return nil
		}
		return gerror.Wrap(lockErr, "获取采集事件处理锁失败")
	}
	defer func() { _ = eventLock.Unlock(context.Background()) }()

	event, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("id", eventId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		One()
	if err != nil {
		return gerror.Wrap(err, "读取采集事件失败")
	}
	if event.IsEmpty() {
		g.Log().Warningf(ctx, "采集事件不存在，跳过处理 eventId:%d tenantId:%d accountId:%d", eventId, tenantId, accountId)
		return gerror.New("采集事件不存在")
	}
	g.Log().Infof(ctx, "采集事件开始处理 eventId:%d sourceId:%d sourceMessageId:%d groupedId:%s status:%s media:%d", eventId, event["source_id"].Int64(), event["source_message_id"].Int64(), event["source_grouped_id"].String(), event["status"].String(), event["media_count"].Int())
	source, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("id", event["source_id"].Int64()).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return gerror.Wrap(err, "读取采集源状态失败")
	}
	if source.IsEmpty() {
		return s.ignoreCollectEvent(ctx, eventId, "采集源不存在", "source")
	}
	if err = s.ensureTenantVipFeature(ctx, tenantId, sysin.TenantVipFeatureCollectSource); err != nil {
		return s.ignoreCollectEvent(ctx, eventId, "VIP会员已到期，停止处理采集事件", "vip")
	}
	if source["collect_enabled"].Int() != 1 || source["status"].Int() != 1 {
		_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusPending, "采集源已暂停，等待恢复")
		return s.enqueueCollectProcess(ctx, collectProcessQueuePayload{
			EventId: eventId, TenantId: tenantId, AccountId: accountId, SourceId: event["source_id"].Int64(),
		}, 30*time.Second)
	}
	if collectEventAlreadyMatched(event["status"].String()) {
		needsRepair, repairErr := s.collectEventNeedsProfileRepair(ctx, eventId)
		if repairErr != nil {
			return repairErr
		}
		if !needsRepair {
			return nil
		}
		g.Log().Infof(ctx, "采集事件存在未回写资料，重新进入分发回写 eventId:%d", eventId)
	}
	materialRole := strings.TrimSpace(event["material_role"].String())
	if materialRole == "" || materialRole == collectMaterialRolePending {
		return newCollectProcessRetryError(30*time.Second, "等待资料组窗口完成")
	}
	if materialRole == collectMaterialRoleVerify {
		parentEventId := event["material_parent_event_id"].Int64()
		if parentEventId > 0 {
			parent, parentErr := pdao.YoubanPublishCollectEvent.Ctx(ctx).
				Fields("id,status,error_message").
				Where("id", parentEventId).
				Where("tenant_id", tenantId).
				Where("account_id", accountId).
				One()
			if parentErr != nil {
				return gerror.Wrap(parentErr, "读取验证资料父展示事件失败")
			}
			if parent.IsEmpty() {
				return newCollectProcessRetryError(30*time.Second, "等待父展示资料进入处理链路")
			}
			parentStatus := strings.TrimSpace(parent["status"].String())
			if parentStatus == sysin.CollectEventStatusIgnored || parentStatus == sysin.CollectEventStatusFailed {
				message := strings.TrimSpace(parent["error_message"].String())
				if message == "" {
					message = "父展示资料已忽略"
				}
				return s.ignoreCollectEvent(ctx, eventId, message, "group")
			}
			if !collectDisplayEventPassedEarlyCheck(parentStatus) {
				return newCollectProcessRetryError(5*time.Second, "等待父展示资料完成规则和去重预检")
			}
		}
		if s.collectEventNeedsMediaCache(ctx, event) {
			_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusMediaPending, "验证媒体缓存中")
			return s.enqueueCollectMediaCache(ctx, collectMediaQueuePayloadFromEvent(event), 0)
		}
		_, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", eventId).Data(g.Map{
			"status":                sysin.CollectEventStatusProcessed,
			"material_group_status": "ready",
			"error_message":         "",
			"processed_at":          gtime.Now(),
			"updated_at":            gtime.Now(),
		}).Update()
		if err != nil {
			return gerror.Wrap(err, "完成验证资料组处理失败")
		}
		if parentEventId <= 0 {
			g.Log().Warningf(ctx, "验证资料已完成但没有父展示事件 eventId:%d", eventId)
			return nil
		}
		g.Log().Infof(ctx, "验证资料媒体已完成，回流处理父展示事件 verifyEventId:%d parentEventId:%d", eventId, parentEventId)
		if err = s.processCollectEvent(ctx, parentEventId, tenantId, accountId); err != nil {
			g.Log().Warningf(ctx, "验证资料回流父展示事件失败，将重新排队 verifyEventId:%d parentEventId:%d err:%+v", eventId, parentEventId, err)
			if enqueueErr := s.enqueueCollectProcess(ctx, collectProcessQueuePayload{
				EventId: parentEventId, TenantId: tenantId, AccountId: accountId,
				SourceId: event["source_id"].Int64(),
			}, 30*time.Second); enqueueErr != nil {
				return enqueueErr
			}
		}
		return nil
	}
	rules, err := s.collectEventRules(ctx, event, tenantId, accountId)
	if err != nil {
		return err
	}
	preflight, err := s.preflightCollectMaterialGroup(ctx, event, rules)
	if err != nil {
		return err
	}
	if len(preflight.Rules) == 0 {
		return s.ignoreCollectEvent(ctx, eventId, preflight.Reason, preflight.Stage)
	}
	candidateRules := preflight.Rules
	_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusPrechecked, "")
	verifyReady, verifyErr := s.ensureCollectPairedVerifyReady(ctx, event)
	if verifyErr != nil {
		return verifyErr
	}
	if s.collectEventNeedsMediaCache(ctx, event) {
		g.Log().Infof(ctx, "采集事件进入媒体缓存 eventId:%d media:%d", eventId, event["media_count"].Int())
		if err = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusMediaPending, "媒体缓存中"); err != nil {
			return err
		}
		s.appendCollectEventLogForRecord(ctx, event, "media", "pending", "媒体等待缓存", "")
		return s.enqueueCollectMediaCache(ctx, collectMediaQueuePayloadFromEvent(event), 0)
	}
	if !verifyReady {
		return newCollectProcessRetryError(30*time.Second, "等待验证资料媒体缓存完成")
	}
	dedupeLock := lock.NewConfig(2*time.Minute, 20*time.Millisecond).Mutex(fmt.Sprintf(
		"youban_publish:collect:dedupe-commit:%d:%d",
		event["tenant_id"].Int64(),
		event["account_id"].Int64(),
	))
	if err = dedupeLock.Lock(ctx); err != nil {
		return gerror.Wrap(err, "获取采集资料提交锁失败")
	}
	defer func() { _ = dedupeLock.Unlock(context.Background()) }()
	if event["media_count"].Int() > 0 {
		_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusMediaReady, "")
		s.appendCollectEventLogForRecord(ctx, event, "media", "ready", "媒体已就绪", "")
	}
	content, err := s.ensureCollectContent(ctx, event)
	if err != nil {
		_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusFailed, err.Error())
		return err
	}
	content, err = s.mergeCollectMaterialContent(ctx, event, content)
	if err != nil {
		_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusFailed, err.Error())
		return err
	}
	canonical, err := s.canonicalCollectProfileMedia(ctx, event, content)
	if err != nil {
		return err
	}
	if err = validateCollectMaterialMedia(canonical); err != nil {
		_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusMediaPending, err.Error())
		if enqueueErr := s.enqueueCollectMediaCache(ctx, collectMediaQueuePayloadFromEvent(event), 0); enqueueErr != nil {
			return enqueueErr
		}
		return newCollectProcessRetryError(30*time.Second, err.Error())
	}
	content = canonical
	candidateRules, err = s.filterCollectRulesByFinalDedupeBatch(ctx, event, content, candidateRules)
	if err != nil {
		return err
	}
	if len(candidateRules) == 0 {
		return s.ignoreCollectEvent(ctx, eventId, "图文重复", "dedupe")
	}
	matched := false
	reasons := make([]string, 0, len(candidateRules))
	for _, rule := range candidateRules {
		ruleMatched, skipReason, dispatchErr := s.dispatchCollectEventByRule(ctx, event, content, rule)
		if dispatchErr != nil {
			err = dispatchErr
			_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusFailed, err.Error())
			return err
		}
		if ruleMatched {
			matched = true
		} else if strings.TrimSpace(skipReason) != "" {
			reasons = append(reasons, strings.TrimSpace(skipReason))
		}
	}
	if !matched {
		message := "未命中规则或被屏蔽"
		if len(reasons) > 0 {
			message = strings.Join(uniqueStrings(reasons), "；")
		}
		return s.ignoreCollectEvent(ctx, eventId, message, "final")
	}
	g.Log().Infof(ctx, "采集事件分发完成 eventId:%d matched:%t rules:%d", eventId, matched, len(candidateRules))
	s.appendCollectEventLogForRecord(ctx, event, "dispatch", "dispatched", "采集事件已生成分发任务", "")
	return s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusDispatched, "")
}

func (s *sSysPublish) ignoreCollectEvent(ctx context.Context, eventId int64, message string, stage string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "未命中规则或被屏蔽"
	}
	if stage == "" {
		stage = "precheck"
	}
	s.appendCollectEventLog(ctx, eventId, stage, "ignored", message, "")
	if err := s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusIgnored, message); err != nil {
		return err
	}
	return s.ignorePairedCollectVerifyEvents(ctx, eventId, message)
}

func collectDisplayEventPassedEarlyCheck(status string) bool {
	switch strings.TrimSpace(status) {
	case sysin.CollectEventStatusPrechecked,
		sysin.CollectEventStatusMediaPending,
		sysin.CollectEventStatusMediaReady,
		sysin.CollectEventStatusProcessed,
		sysin.CollectEventStatusDispatched:
		return true
	default:
		return false
	}
}

func (s *sSysPublish) ignorePairedCollectVerifyEvents(ctx context.Context, displayEventId int64, message string) error {
	if displayEventId <= 0 {
		return nil
	}
	eventCols := pdao.YoubanPublishCollectEvent.Columns()
	verifyIds, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields(eventCols.Id).
		Where("material_parent_event_id", displayEventId).
		Where("material_role", collectMaterialRoleVerify).
		WhereNotIn(eventCols.Status, []string{sysin.CollectEventStatusProcessed, sysin.CollectEventStatusDispatched, sysin.CollectEventStatusIgnored}).
		Array()
	if err != nil {
		return gerror.Wrap(err, "读取配对验证资料失败")
	}
	ids := make([]int64, 0, len(verifyIds))
	for _, value := range verifyIds {
		if id := value.Int64(); id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	now := gtime.Now()
	if _, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).
		WhereIn(eventCols.Id, ids).
		Data(g.Map{
			eventCols.Status:       sysin.CollectEventStatusIgnored,
			eventCols.ErrorMessage: message,
			eventCols.ProcessedAt:  now,
			eventCols.UpdatedAt:    now,
		}).Update(); err != nil {
		return gerror.Wrap(err, "忽略配对验证资料失败")
	}
	mediaCols := pdao.YoubanPublishCollectEventMedia.Columns()
	_, _ = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		WhereIn(mediaCols.EventId, ids).
		WhereIn(mediaCols.CacheStatus, []string{collectMediaCachePending, collectMediaCacheDownloading}).
		Data(g.Map{
			mediaCols.CacheStatus:  collectMediaCacheCanceled,
			mediaCols.ErrorMessage: message,
			mediaCols.UpdatedAt:    now,
		}).Update()
	for _, id := range ids {
		s.appendCollectEventLog(ctx, id, "group", "ignored", message, fmt.Sprintf("parentEventId=%d", displayEventId))
	}
	return nil
}

func collectEventAlreadyMatched(status string) bool {
	status = strings.TrimSpace(status)
	return status == sysin.CollectEventStatusProcessed || status == sysin.CollectEventStatusDispatched || status == "matched"
}

func (s *sSysPublish) collectEventNeedsProfileRepair(ctx context.Context, eventId int64) (bool, error) {
	row, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Fields("id").
		Where("event_id", eventId).
		Where("profile_id <= 0").
		One()
	if err != nil {
		return false, gerror.Wrap(err, "检查采集资料回写状态失败")
	}
	return !row.IsEmpty(), nil
}

func (s *sSysPublish) collectEventRules(ctx context.Context, event gdb.Record, tenantId int64, accountId int64) ([]gdb.Record, error) {
	sourceId := event["source_id"].Int64()
	cacheKey := s.collectEventRulesCacheKey(ctx, tenantId, accountId, sourceId)
	if rows, ok := collectEventRulesCacheGet(ctx, cacheKey); ok {
		if err := attachCollectRuleChannels(ctx, rows); err != nil {
			return nil, err
		}
		if err := attachCollectRuleItems(ctx, rows); err != nil {
			return nil, err
		}
		return rows, nil
	}
	var bindRows []struct {
		RuleId int64 `json:"ruleId"`
	}
	if err := pdao.YoubanPublishCollectSourceRule.Ctx(ctx).Where("source_id", sourceId).Where("status", 1).OrderAsc("sort").Scan(&bindRows); err != nil {
		return nil, gerror.Wrap(err, "读取采集源绑定规则失败")
	}
	ruleIds := make([]int64, 0, len(bindRows))
	for _, row := range bindRows {
		ruleIds = append(ruleIds, row.RuleId)
	}
	mod := pdao.YoubanPublishCollectRule.Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Where("status", 1).
		WhereNull("deleted_at")
	if len(ruleIds) > 0 {
		mod = mod.WhereIn("id", ruleIds)
	} else {
		mod = mod.Where("global_enabled", 1)
	}
	rows, err := mod.OrderAsc("sort").OrderAsc("id").All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取采集规则失败")
	}
	if err = attachCollectRuleChannels(ctx, rows); err != nil {
		return nil, err
	}
	if err = attachCollectRuleItems(ctx, rows); err != nil {
		return nil, err
	}
	collectEventRulesCacheSet(ctx, cacheKey, rows)
	return rows, nil
}

func (s *sSysPublish) collectEventRulesCacheKey(ctx context.Context, tenantId int64, accountId int64, sourceId int64) string {
	version := s.collectEventRulesCacheVersion(ctx)
	return fmt.Sprintf("%s:%s:%d:%d:%d", collectEventRulesCacheKeyPrefix, version, tenantId, accountId, sourceId)
}

func (s *sSysPublish) collectEventRulesCacheVersion(ctx context.Context) string {
	cacheVar, err := cache.Instance().Get(ctx, collectEventRulesCacheVersionKey)
	if err == nil && !cacheVar.IsNil() {
		version := strings.TrimSpace(cacheVar.String())
		if version != "" {
			return version
		}
	}
	return "1"
}

func (s *sSysPublish) refreshCollectEventRulesCache(ctx context.Context) {
	_ = cache.Instance().Set(ctx, collectEventRulesCacheVersionKey, fmt.Sprintf("%d", gtime.Now().TimestampNano()), 24*time.Hour)
}

func collectEventRulesCacheGet(ctx context.Context, key string) ([]gdb.Record, bool) {
	cacheVar, err := cache.Instance().Get(ctx, key)
	if err != nil || cacheVar.IsNil() {
		return nil, false
	}
	var list []g.Map
	if err = cacheVar.Scan(&list); err != nil {
		return nil, false
	}
	return collectEventRuleMapsToRecords(list), true
}

func collectEventRulesCacheSet(ctx context.Context, key string, rows gdb.Result) {
	_ = cache.Instance().Set(ctx, key, rows.List(), collectEventRulesCacheTTL)
}

func collectEventRuleMapsToRecords(list []g.Map) []gdb.Record {
	rows := make([]gdb.Record, 0, len(list))
	for _, item := range list {
		record := gdb.Record{}
		for key, value := range item {
			record[key] = gvar.New(value)
		}
		rows = append(rows, record)
	}
	return rows
}

func (s *sSysPublish) dispatchCollectEventByRule(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record) (bool, string, error) {
	decision := buildCollectRuleDecision(event, content, rule)
	if decision.Skipped || !decision.Matched {
		s.appendCollectEventLogForRecord(ctx, event, "rule", "skipped", decision.Reason, fmt.Sprintf("rule=%d", rule["id"].Int64()))
		return false, decision.Reason, nil
	}
	classification, err := s.classifyCollectEvent(ctx, event, content)
	if err != nil {
		return false, "", err
	}
	if classification.Kind == profileMessageKindIgnore {
		const reason = "消息不包含可创建资料的正文或有效媒体"
		s.appendCollectEventLogForRecord(ctx, event, "classifier", "skipped", reason, fmt.Sprintf("rule=%d", rule["id"].Int64()))
		return false, reason, nil
	}
	if classification.Kind == profileMessageKindVerify {
		const reason = "验证资料组不能独立创建资料"
		s.appendCollectEventLogForRecord(ctx, event, "classifier", "skipped", reason, fmt.Sprintf("rule=%d", rule["id"].Int64()))
		return false, reason, nil
	}
	existingDispatch, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("tenant_id", event["tenant_id"].Int64()).
		Where("account_id", event["account_id"].Int64()).
		Where("event_id", event["id"].Int64()).
		Where("rule_id", rule["id"].Int64()).
		OrderDesc("id").
		One()
	if err != nil {
		return false, "", gerror.Wrap(err, "读取采集分发幂等记录失败")
	}
	if !existingDispatch.IsEmpty() {
		if rule["review_enabled"].Int() == 1 && existingDispatch["review_id"].Int64() <= 0 {
			return true, "", s.createCollectReview(ctx, event, content, rule, existingDispatch["id"].Int64(), decision.Text)
		}
		profileId := existingDispatch["profile_id"].Int64()
		if profileId <= 0 {
			profileId, err = s.existingCollectProfileId(ctx, event, rule)
			if err != nil {
				return false, "", err
			}
		}
		if profileId > 0 {
			updatedProfileId, upsertErr := s.commitCollectMaterial(ctx, event, content, rule, decision.Text)
			if upsertErr != nil {
				return false, "", upsertErr
			}
			if existingDispatch["profile_id"].Int64() <= 0 && updatedProfileId > 0 {
				if _, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).
					Where("id", existingDispatch["id"].Int64()).
					Data(g.Map{"profile_id": updatedProfileId, "updated_at": gtime.Now()}).Update(); err != nil {
					return false, "", gerror.Wrap(err, "回填采集分发资料失败")
				}
				g.Log().Infof(ctx, "采集分发记录已回填资料 profileId:%d eventId:%d dispatchId:%d", updatedProfileId, event["id"].Int64(), existingDispatch["id"].Int64())
			}
		}
		return true, "", nil
	}
	now := gtime.Now()
	channelIds := collectRuleTargetChannelIds(rule)
	if len(channelIds) == 0 {
		return false, "", gerror.New("采集规则未配置目标频道")
	}
	var dispatchId int64
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var txErr error
		dispatchId, txErr = tx.Model(pdao.YoubanPublishCollectDispatch.Table()).Ctx(ctx).Data(g.Map{
			"tenant_id": event["tenant_id"].Int64(), "account_id": event["account_id"].Int64(),
			"source_id": event["source_id"].Int64(), "rule_id": rule["id"].Int64(), "event_id": event["id"].Int64(),
			"match_json": decision.MatchJSON, "status": sysin.CollectDispatchStatusPending,
			"created_at": now, "updated_at": now,
		}).InsertAndGetId()
		if txErr != nil {
			return gerror.Wrap(txErr, "创建采集分发记录失败")
		}
		return createCollectDispatchChannelsTx(ctx, tx, event["tenant_id"].Int64(), event["account_id"].Int64(), dispatchId, channelIds)
	})
	if err != nil {
		return false, "", err
	}
	if rule["review_enabled"].Int() == 1 {
		return true, "", s.createCollectReview(ctx, event, content, rule, dispatchId, decision.Text)
	}
	profileId, err := s.commitCollectMaterial(ctx, event, content, rule, decision.Text)
	if err != nil {
		return false, "", err
	}
	if err = s.submitCollectProfileDispatch(ctx, dispatchId, profileId, event); err != nil {
		return false, "", err
	}
	return true, "", nil
}

func (s *sSysPublish) existingCollectProfileId(ctx context.Context, event gdb.Record, rule gdb.Record) (int64, error) {
	sourceKey := collectPublishClientRequestId(event, rule)
	if sourceKey == "" {
		return 0, nil
	}
	columns := dao.ContentProfile.Columns()
	row, err := g.DB().Model(dao.ContentProfile.Table()+" p").Safe().Ctx(ctx).
		Fields("p."+columns.Id).
		Where("p."+columns.SourceKey, sourceKey).
		WhereNull("p."+columns.DeletedAt).
		Where("EXISTS (SELECT 1 FROM hg_youban_publish_profile_state ps WHERE ps.profile_id=p.id AND ps.tenant_id=? AND ps.account_id=? AND ps.deleted_at IS NULL)", event["tenant_id"].Int64(), event["account_id"].Int64()).
		One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取已有采集资料失败")
	}
	if row.IsEmpty() {
		return 0, nil
	}
	return row[columns.Id].Int64(), nil
}

func (s *sSysPublish) classifyCollectEvent(ctx context.Context, event gdb.Record, content *collectContentResult) (profileMessageClassification, error) {
	text := strings.TrimSpace(event["raw_text"].String())
	var items []collectMediaItem
	if content != nil {
		text = strings.TrimSpace(content.RawText)
		items = content.Media
	}
	if len(items) == 0 {
		rows, err := s.collectEventMediaRows(ctx, event["id"].Int64())
		if err != nil {
			return profileMessageClassification{}, err
		}
		items = collectMediaRowsToItems(rows, event["material_role"].String())
	}
	classification := classifyProfileMessage(text, items)
	verifyCount := 0
	displayCount := 0
	for _, item := range items {
		switch strings.ToLower(strings.TrimSpace(item.Purpose)) {
		case collectMaterialRoleVerify:
			verifyCount++
		case collectMaterialRoleDisplay:
			displayCount++
		}
	}
	g.Log().Debugf(ctx, "采集事件分类完成 eventId:%d kind:%s textBytes:%d media:%d displayMedia:%d verifyMedia:%d status:%s", event["id"].Int64(), classification.Kind, len(text), len(items), displayCount, verifyCount, event["status"].String())
	return classification, nil
}

func (s *sSysPublish) collectEventMediaReady(ctx context.Context, eventId int64) (bool, error) {
	if eventId <= 0 {
		return false, nil
	}
	rows, err := s.collectEventMediaRows(ctx, eventId)
	if err != nil {
		return false, err
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		if collectMediaRowNeedsCache(row.SourceFileId, row.SourceMessageRef, row.StoragePath, row.FileUrl, row.BackupChatId, row.BackupMessageId) {
			return false, nil
		}
	}
	return len(rows) > 0, nil
}

func collectMediaJSONHasVideo(mediaJSON string) bool {
	return collectMediaJSONHasPurposeVideo(mediaJSON, "")
}

func collectMediaJSONHasPurposeVideo(mediaJSON string, purpose string) bool {
	var items []collectMediaItem
	if err := json.Unmarshal([]byte(mediaJSON), &items); err != nil {
		return false
	}
	for _, item := range items {
		if !strings.EqualFold(strings.TrimSpace(item.Type), "video") {
			continue
		}
		if purpose != "" && !strings.EqualFold(strings.TrimSpace(item.Purpose), purpose) {
			continue
		}
		if purpose == "" || strings.EqualFold(strings.TrimSpace(item.Purpose), purpose) {
			return true
		}
	}
	return false
}

func (s *sSysPublish) createCollectReview(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record, dispatchId int64, text string) error {
	existingReview, err := pdao.YoubanPublishCollectReview.Ctx(ctx).
		Where("dispatch_id", dispatchId).
		OrderDesc("id").
		One()
	if err != nil {
		return gerror.Wrap(err, "读取采集审核幂等记录失败")
	}
	if !existingReview.IsEmpty() {
		prepared, prepareErr := s.prepareCollectMaterial(ctx, event, content)
		if prepareErr != nil {
			return gerror.Wrap(prepareErr, "刷新采集审核媒体失败")
		}
		if prepared == nil || prepared.Content == nil {
			return newCollectProcessRetryError(30*time.Second, "采集审核资料尚未准备完成")
		}
		_, updateErr := pdao.YoubanPublishCollectReview.Ctx(ctx).
			Where("id", existingReview["id"].Int64()).
			Data(g.Map{
				"media_count": prepared.Content.MediaCount,
				"updated_at":  gtime.Now(),
			}).Update()
		return gerror.Wrap(updateErr, "刷新采集审核媒体数量失败")
	}
	now := gtime.Now()
	prepared, err := s.prepareCollectMaterial(ctx, event, content)
	if err != nil {
		return gerror.Wrap(err, "准备采集审核资料失败")
	}
	if prepared == nil || prepared.Content == nil {
		return newCollectProcessRetryError(30*time.Second, "采集审核资料尚未准备完成")
	}
	mediaCount := prepared.Content.MediaCount
	reviewId, err := pdao.YoubanPublishCollectReview.Ctx(ctx).Data(g.Map{
		"tenant_id":   event["tenant_id"].Int64(),
		"account_id":  event["account_id"].Int64(),
		"source_id":   event["source_id"].Int64(),
		"rule_id":     rule["id"].Int64(),
		"event_id":    event["id"].Int64(),
		"dispatch_id": dispatchId,
		"raw_text":    text,
		"media_count": mediaCount,
		"status":      sysin.CollectReviewStatusPending,
		"created_at":  now,
		"updated_at":  now,
	}).InsertAndGetId()
	if err != nil {
		return gerror.Wrap(err, "创建采集审核失败")
	}
	g.Log().Infof(ctx, "采集审核已创建 reviewId:%d eventId:%d dispatchId:%d ruleId:%d media:%d", reviewId, event["id"].Int64(), dispatchId, rule["id"].Int64(), mediaCount)
	_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where("id", dispatchId).Data(g.Map{
		"review_id":  reviewId,
		"status":     sysin.CollectDispatchStatusReviewing,
		"updated_at": now,
	}).Update()
	return gerror.Wrap(err, "更新采集审核分发失败")
}
