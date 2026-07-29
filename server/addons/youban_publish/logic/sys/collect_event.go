package sys

import (
	"context"
	"encoding/json"
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
	"hotgo/internal/library/cache"
)

const (
	collectEventRulesCacheVersionKey = "youban_publish:collect:event_rules:version"
	collectEventRulesCacheKeyPrefix  = "youban_publish:collect:event_rules"
	collectEventRulesCacheTTL        = time.Minute
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
		Where("e.account_id", account.Id)
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
	for _, item := range list {
		item.MediaCacheStatus, item.MediaCacheMessage = collectEventMediaCacheView(item.MediaJson, item.MediaCount, item.Status, item.ErrorMessage)
	}
	return
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
		EventId             int64  `json:"eventId"`
		TargetChannelIdJson string `json:"targetChannelIdJson"`
		ReviewId            int64  `json:"reviewId"`
		Status              string `json:"status"`
	}
	if err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		WhereIn("event_id", eventIds).
		Fields("event_id,target_channel_id_json,review_id,status").
		Scan(&dispatches); err != nil {
		return gerror.Wrap(err, "读取采集事件分发信息失败")
	}
	channelIds := make([]int64, 0)
	for _, item := range dispatches {
		r := routes[item.EventId]
		if r == nil {
			continue
		}
		r.ChannelIds = append(r.ChannelIds, decodeInt64JSON(item.TargetChannelIdJson)...)
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
		Id                  int64  `json:"id"`
		TargetChannelIdJson string `json:"targetChannelIdJson"`
	}
	if len(ruleIds) > 0 {
		if err := pdao.YoubanPublishCollectRule.Ctx(ctx).
			Where("tenant_id", tenantId).Where("account_id", accountId).
			WhereIn("id", ruleIds).Where("status", 1).WhereNull("deleted_at").
			Fields("id,target_channel_id_json").Scan(&rules); err != nil {
			return gerror.Wrap(err, "读取采集规则目标失败")
		}
	}
	ruleTargets := make(map[int64][]int64, len(rules))
	for _, rule := range rules {
		ruleTargets[rule.Id] = decodeInt64JSON(rule.TargetChannelIdJson)
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
	if source["collect_enabled"].Int() != 1 || source["status"].Int() != 1 {
		_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusPending, "采集源已暂停，等待恢复")
		return s.enqueueCollectProcess(ctx, collectProcessQueuePayload{
			EventId: eventId, TenantId: tenantId, AccountId: accountId, SourceId: event["source_id"].Int64(),
		}, 30*time.Second)
	}
	if collectEventAlreadyMatched(event["status"].String()) {
		return nil
	}
	if waiting, err := s.waitCollectGroupedEventReady(ctx, event); err != nil {
		if _, ok := err.(*collectProcessRetryError); ok {
			return err
		}
		_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusFailed, err.Error())
		return err
	} else if waiting {
		return nil
	}
	rules, err := s.collectEventRules(ctx, event, tenantId, accountId)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		g.Log().Infof(ctx, "采集事件无可用规则 eventId:%d sourceId:%d", eventId, event["source_id"].Int64())
		return s.ignoreCollectEvent(ctx, eventId, "未命中可用规则", "precheck")
	}
	candidateRules := rules
	var precheckReasons []string
	candidateRules, precheckReasons = s.precheckCollectEventRules(event, rules)
	if len(candidateRules) == 0 {
		message := "未命中规则或被屏蔽"
		if len(precheckReasons) > 0 {
			message = strings.Join(uniqueStrings(precheckReasons), "；")
		}
		return s.ignoreCollectEvent(ctx, eventId, message, "precheck")
	}
	if waiting, err := s.waitCollectEventSourceOrder(ctx, event); err != nil {
		if _, ok := err.(*collectProcessRetryError); ok {
			return err
		}
		_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusFailed, err.Error())
		return err
	} else if waiting {
		return nil
	}
	_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusPrechecked, "")
	if s.collectEventNeedsMediaCache(ctx, event) {
		g.Log().Infof(ctx, "采集事件进入媒体缓存 eventId:%d media:%d", eventId, event["media_count"].Int())
		if err = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusMediaPending, "媒体缓存中"); err != nil {
			return err
		}
		s.appendCollectEventLogForRecord(ctx, event, "media", "pending", "媒体等待缓存", "")
		return s.enqueueCollectMediaCache(ctx, collectMediaQueuePayload{
			EventId:     eventId,
			TenantId:    tenantId,
			AccountId:   accountId,
			SourceId:    event["source_id"].Int64(),
			TgAccountId: event["tg_account_id"].Int64(),
		}, 0)
	}
	if event["media_count"].Int() > 0 {
		_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusMediaReady, "")
		s.appendCollectEventLogForRecord(ctx, event, "media", "ready", "媒体已就绪", "")
	}
	content, err := s.ensureCollectContent(ctx, event)
	if err != nil {
		_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusFailed, err.Error())
		return err
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
	return s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusIgnored, message)
}

func collectEventAlreadyMatched(status string) bool {
	status = strings.TrimSpace(status)
	return status == sysin.CollectEventStatusProcessed || status == sysin.CollectEventStatusDispatched || status == "matched"
}

func (s *sSysPublish) collectEventRules(ctx context.Context, event gdb.Record, tenantId int64, accountId int64) ([]gdb.Record, error) {
	sourceId := event["source_id"].Int64()
	cacheKey := s.collectEventRulesCacheKey(ctx, tenantId, accountId, sourceId)
	if rows, ok := collectEventRulesCacheGet(ctx, cacheKey); ok {
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
	decision, err := s.evaluateCollectRule(ctx, event, content, rule)
	if err != nil {
		return false, "", err
	}
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
	if rule["review_enabled"].Int() == 1 && classification.Kind == profileMessageKindVerify {
		if merged, err := s.mergeCollectEventIntoPendingReview(ctx, event, content, rule, decision.Text); err != nil {
			return false, "", err
		} else if merged {
			return true, "", nil
		}
	}
	if attached, err := s.attachCollectVerifyEventToPreviousTask(ctx, event, content, rule); err != nil {
		return false, "", err
	} else if attached {
		return true, "", nil
	}
	if classification.Kind == profileMessageKindVerify {
		if !s.collectVerifyEventTimedOut(ctx, event) {
			const message = "验证视频暂未匹配到前序资料，等待前序资料完成处理"
			if _, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", event["id"].Int64()).Data(g.Map{
				"status":        sysin.CollectEventStatusWaitingOrder,
				"error_message": message,
				"updated_at":    gtime.Now(),
			}).Update(); err != nil {
				return false, "", gerror.Wrap(err, "标记验证视频等待前序资料失败")
			}
			s.appendCollectEventLogForRecord(ctx, event, "verify", "waiting", message, "auto_retry=true")
			return false, message, newCollectProcessRetryError(collectOrderRetryDelay, message)
		}
		const reason = "验证视频未匹配到连续的前序资料，已忽略"
		s.appendCollectEventLogForRecord(ctx, event, "classifier", "skipped", reason, fmt.Sprintf("rule=%d", rule["id"].Int64()))
		return false, reason, nil
	}
	now := gtime.Now()
	dispatchId, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).Data(g.Map{
		"tenant_id":              event["tenant_id"].Int64(),
		"account_id":             event["account_id"].Int64(),
		"source_id":              event["source_id"].Int64(),
		"rule_id":                rule["id"].Int64(),
		"event_id":               event["id"].Int64(),
		"target_channel_id_json": rule["target_channel_id_json"].String(),
		"bot_id_json":            rule["bot_id_json"].String(),
		"match_json":             decision.MatchJSON,
		"status":                 sysin.CollectDispatchStatusPending,
		"created_at":             now,
		"updated_at":             now,
	}).InsertAndGetId()
	if err != nil {
		return false, "", gerror.Wrap(err, "创建采集分发记录失败")
	}
	if rule["review_enabled"].Int() == 1 {
		return true, "", s.createCollectReview(ctx, event, content, rule, dispatchId, decision.Text)
	}
	profileId, err := s.upsertCollectProfile(ctx, event, content, rule, decision.Text)
	if err != nil {
		return false, "", err
	}
	if err = s.submitCollectProfileDispatch(ctx, dispatchId, profileId, event, rule); err != nil {
		return false, "", err
	}
	return true, "", nil
}

func (s *sSysPublish) classifyCollectEvent(ctx context.Context, event gdb.Record, content *collectContentResult) (profileMessageClassification, error) {
	text := strings.TrimSpace(event["raw_text"].String())
	mediaJSON := strings.TrimSpace(event["media_json"].String())
	if content != nil {
		text = strings.TrimSpace(content.RawText)
		mediaJSON = strings.TrimSpace(content.MediaJSON)
	}
	var items []collectMediaItem
	if mediaJSON != "" {
		if err := json.Unmarshal([]byte(mediaJSON), &items); err != nil {
			return profileMessageClassification{}, gerror.Wrap(err, "解析采集媒体失败")
		}
	}
	if len(items) == 0 && event["media_count"].Int() > 0 {
		rows, err := s.collectEventMediaRows(ctx, event["id"].Int64())
		if err != nil {
			return profileMessageClassification{}, err
		}
		items = collectMediaRowsToItems(rows)
	}
	return classifyProfileMessage(text, items), nil
}

func (s *sSysPublish) attachCollectVerifyEventToPreviousTask(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record) (bool, error) {
	rows, err := s.collectEventMediaRows(ctx, event["id"].Int64())
	if err != nil {
		return false, err
	}
	items := collectMediaRowsToItems(rows)
	text := strings.TrimSpace(event["raw_text"].String())
	if text == "" && content != nil {
		text = strings.TrimSpace(content.RawText)
	}
	classification := classifyProfileMessage(text, items)
	if classification.Kind != profileMessageKindVerify {
		return false, nil
	}
	previous, err := s.previousCollectProfileForVerify(ctx, event, rule)
	if err != nil || previous.IsEmpty() {
		return false, err
	}
	continuous, err := s.collectVerifyEventIsContinuous(ctx, event, previous)
	if err != nil || !continuous {
		return false, err
	}
	profileId := previous["profile_id"].Int64()
	if err = s.insertCollectOwnedMediaRows(ctx, event, collectPublishMediaOwner{ProfileId: profileId}, "verify", items); err != nil {
		return false, err
	}
	now := gtime.Now()
	if _, err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("profile_id", profileId).WhereNull("deleted_at").Data(g.Map{"updated_at": now}).Update(); err != nil {
		return false, gerror.Wrap(err, "绑定采集验证视频到资料失败")
	}
	if _, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("profile_id", profileId).Where("collect_event_id", previous["event_id"].Int64()).WhereIn("status", []string{"pending", "failed_retry"}).Data(g.Map{
		"status":          "pending",
		"dispatch_status": tgDispatchStatusIdle,
		"next_retry_at":   nil,
		"error_message":   "",
		"updated_at":      now,
	}).Update(); err != nil {
		return false, gerror.Wrap(err, "重置绑定验证视频TG任务失败")
	}
	s.appendCollectEventLogForRecord(ctx, event, "dispatch", "attached", "验证视频已绑定到前一个采集资料", fmt.Sprintf("profile=%d", profileId))
	return true, nil
}

func (s *sSysPublish) collectVerifyEventIsContinuous(ctx context.Context, event gdb.Record, previous gdb.Record) (bool, error) {
	if event.IsEmpty() || previous.IsEmpty() {
		return false, nil
	}
	lastSourceMessageId, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Where("event_id", previous["event_id"].Int64()).
		Fields("MAX(source_message_id)").
		Value()
	if err != nil {
		return false, gerror.Wrap(err, "读取采集资料最后消息失败")
	}
	lastMessageId := lastSourceMessageId.Int64()
	currentMessageId := event["source_message_id"].Int64()
	return lastMessageId > 0 && currentMessageId == lastMessageId+1, nil
}

func (s *sSysPublish) previousCollectProfileForVerify(ctx context.Context, event gdb.Record, rule gdb.Record) (gdb.Record, error) {
	row, err := g.DB().Model(pdao.YoubanPublishCollectDispatch.Table()+" d").Safe().Ctx(ctx).
		InnerJoin(pdaoCollectEventTable()+" e", "e.id=d.event_id").
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=d.profile_id AND ps.deleted_at IS NULL").
		Fields("d.profile_id,d.event_id,e.source_message_id").
		Where("d.tenant_id", event["tenant_id"].Int64()).
		Where("d.account_id", event["account_id"].Int64()).
		Where("d.source_id", event["source_id"].Int64()).
		Where("e.source_chat_id", strings.TrimSpace(event["source_chat_id"].String())).
		Where("e.source_message_id > 0").
		Where("e.source_message_id < ?", event["source_message_id"].Int64()).
		Where("ps.channel_id_json", rule["target_channel_id_json"].String()).
		Where("e.media_count > 0").
		Where("COALESCE(e.raw_text, '') <> ''").
		Where("NOT EXISTS (SELECT 1 FROM "+pdaoCollectEventTable()+" mid WHERE mid.tenant_id=e.tenant_id AND mid.account_id=e.account_id AND mid.source_id=e.source_id AND mid.source_chat_id=e.source_chat_id AND mid.source_message_id > e.source_message_id AND mid.source_message_id < ? AND mid.media_count > 0 AND COALESCE(mid.raw_text, '') <> '')", event["source_message_id"].Int64()).
		OrderDesc("e.source_message_id").
		OrderDesc("d.id").
		Limit(1).
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "查找验证视频所属采集资料失败")
	}
	return row, nil
}

func (s *sSysPublish) mergeCollectEventIntoPendingReview(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record, text string) (bool, error) {
	if content == nil || !collectMediaJSONHasVideo(content.MediaJSON) {
		return false, nil
	}
	currentMessageId := event["source_message_id"].Int64()
	if currentMessageId <= 0 || strings.TrimSpace(event["source_chat_id"].String()) == "" {
		return false, nil
	}
	review, err := pdao.YoubanPublishCollectReview.DB().Model(pdao.YoubanPublishCollectReview.Table()+" r").Safe().Ctx(ctx).
		LeftJoin(pdao.YoubanPublishCollectEvent.Table()+" e", "e.id=r.event_id").
		Fields("r.id,r.dispatch_id,r.raw_text,r.media_json,r.media_count,e.id AS source_event_id,e.source_message_id").
		Where("r.tenant_id", event["tenant_id"].Int64()).
		Where("r.account_id", event["account_id"].Int64()).
		Where("r.source_id", event["source_id"].Int64()).
		Where("r.rule_id", rule["id"].Int64()).
		Where("r.status", sysin.CollectReviewStatusPending).
		Where("e.source_chat_id", strings.TrimSpace(event["source_chat_id"].String())).
		Where("e.source_message_id > 0").
		Where("e.source_message_id < ?", currentMessageId).
		Where("e.source_message_id >= ?", currentMessageId-20).
		OrderDesc("e.source_message_id").
		Limit(1).
		One()
	if err != nil {
		return false, gerror.Wrap(err, "读取待合并采集审核失败")
	}
	if review.IsEmpty() || collectMediaJSONHasVideo(review["media_json"].String()) {
		return false, nil
	}
	verifyMediaJSON := collectMediaJSONWithPurpose(content.MediaJSON, "verify")
	mediaJSON, mediaCount := mergeCollectMediaJSON(review["media_json"].String(), verifyMediaJSON)
	mergedText := mergeCollectReviewText(review["raw_text"].String(), text)
	now := gtime.Now()
	if _, err = pdao.YoubanPublishCollectReview.Ctx(ctx).Where("id", review["id"].Int64()).Data(g.Map{
		"raw_text":    mergedText,
		"media_count": mediaCount,
		"media_json":  mediaJSON,
		"updated_at":  now,
	}).Update(); err != nil {
		return false, gerror.Wrap(err, "合并采集审核失败")
	}
	s.appendCollectEventLogForRecord(ctx, event, "review", "merged", "已合并到上一组采集审核", fmt.Sprintf("review=%d", review["id"].Int64()))
	return true, nil
}

func collectMediaJSONHasVideo(mediaJSON string) bool {
	var items []collectMediaItem
	if err := json.Unmarshal([]byte(mediaJSON), &items); err != nil {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Type), "video") {
			return true
		}
	}
	return false
}

func mergeCollectReviewText(existing string, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if existing == "" {
		return next
	}
	if next == "" || existing == next {
		return existing
	}
	return existing + "\n" + next
}

func (s *sSysPublish) createCollectReview(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record, dispatchId int64, text string) error {
	now := gtime.Now()
	mediaCount := event["media_count"].Int()
	mediaJSON := collectMediaJSONWithPurpose(event["media_json"].String(), "display")
	if content != nil {
		mediaCount = content.MediaCount
		mediaJSON = collectMediaJSONWithPurpose(content.MediaJSON, "display")
	}
	reviewId, err := pdao.YoubanPublishCollectReview.Ctx(ctx).Data(g.Map{
		"tenant_id":              event["tenant_id"].Int64(),
		"account_id":             event["account_id"].Int64(),
		"source_id":              event["source_id"].Int64(),
		"rule_id":                rule["id"].Int64(),
		"event_id":               event["id"].Int64(),
		"dispatch_id":            dispatchId,
		"raw_text":               text,
		"media_count":            mediaCount,
		"media_json":             mediaJSON,
		"target_channel_id_json": rule["target_channel_id_json"].String(),
		"bot_id_json":            rule["bot_id_json"].String(),
		"status":                 sysin.CollectReviewStatusPending,
		"created_at":             now,
		"updated_at":             now,
	}).InsertAndGetId()
	if err != nil {
		return gerror.Wrap(err, "创建采集审核失败")
	}
	g.Log().Infof(ctx, "采集审核已创建 reviewId:%d eventId:%d dispatchId:%d ruleId:%d media:%d", reviewId, event["id"].Int64(), dispatchId, rule["id"].Int64(), mediaCount)
	if err = s.backfillCollectVerifyEventIntoReview(ctx, event, reviewId); err != nil {
		return err
	}
	_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where("id", dispatchId).Data(g.Map{
		"review_id":  reviewId,
		"status":     sysin.CollectDispatchStatusReviewing,
		"updated_at": now,
	}).Update()
	return gerror.Wrap(err, "更新采集审核分发失败")
}

func (s *sSysPublish) backfillCollectVerifyEventIntoReview(ctx context.Context, event gdb.Record, reviewId int64) error {
	if event.IsEmpty() || reviewId <= 0 || strings.TrimSpace(event["source_chat_id"].String()) == "" {
		return nil
	}
	lastMessageValue, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Where("event_id", event["id"].Int64()).
		Fields("MAX(source_message_id)").
		Value()
	if err != nil {
		return gerror.Wrap(err, "读取审核资料最后消息失败")
	}
	lastMessageId := lastMessageValue.Int64()
	if lastMessageId <= 0 {
		lastMessageId = event["source_message_id"].Int64()
	}
	rows, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("tenant_id", event["tenant_id"].Int64()).
		Where("account_id", event["account_id"].Int64()).
		Where("source_id", event["source_id"].Int64()).
		Where("source_chat_id", strings.TrimSpace(event["source_chat_id"].String())).
		WhereGT("source_message_id", lastMessageId).
		WhereLTE("source_message_id", lastMessageId+20).
		WhereGT("media_count", 0).
		Where("COALESCE(raw_text, '') = ''").
		WhereIn("status", []string{
			sysin.CollectEventStatusIgnored,
			sysin.CollectEventStatusPending,
			sysin.CollectEventStatusWaitingOrder,
			sysin.CollectEventStatusFailed,
		}).
		OrderAsc("source_message_id").
		Limit(10).
		All()
	if err != nil {
		return gerror.Wrap(err, "读取待补回验证视频失败")
	}
	for _, candidate := range rows {
		classification, classifyErr := s.classifyCollectEvent(ctx, candidate, nil)
		if classifyErr != nil {
			return classifyErr
		}
		if classification.Kind != profileMessageKindVerify || candidate["source_message_id"].Int64() != lastMessageId+1 {
			continue
		}
		verifyMediaJSON := collectMediaJSONWithPurpose(candidate["media_json"].String(), "verify")
		mediaJSON, mediaCount := mergeCollectMediaJSON(event["media_json"].String(), verifyMediaJSON)
		if _, err = pdao.YoubanPublishCollectReview.Ctx(ctx).Where("id", reviewId).Data(g.Map{
			"media_json":  mediaJSON,
			"media_count": mediaCount,
			"updated_at":  gtime.Now(),
		}).Update(); err != nil {
			return gerror.Wrap(err, "补回验证视频到审核失败")
		}
		if _, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", candidate["id"].Int64()).Data(g.Map{
			"status":        sysin.CollectEventStatusProcessed,
			"error_message": "",
			"processed_at":  gtime.Now(),
			"updated_at":    gtime.Now(),
		}).Update(); err != nil {
			return gerror.Wrap(err, "更新补回验证视频事件失败")
		}
		s.appendCollectEventLogForRecord(ctx, candidate, "verify", "attached", "验证视频已补回到审核资料", fmt.Sprintf("review=%d", reviewId))
		break
	}
	return nil
}
