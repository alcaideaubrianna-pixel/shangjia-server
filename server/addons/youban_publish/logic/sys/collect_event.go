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
	"hotgo/internal/consts"
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
	for _, item := range list {
		item.MediaCacheStatus, item.MediaCacheMessage = collectEventMediaCacheView(item.MediaJson, item.MediaCount, item.Status, item.ErrorMessage)
	}
	return
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

func (s *sSysPublish) CollectEventProcess(ctx context.Context, in *sysin.CollectEventProcessInp) error {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || in.Id <= 0 {
		return gerror.New("事件ID不能为空")
	}
	return s.processCollectEvent(ctx, in.Id, account.TenantId, account.Id)
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
		return gerror.New("采集事件不存在")
	}
	if collectEventAlreadyMatched(event["status"].String()) {
		return nil
	}
	if waiting, err := s.waitCollectGroupedEventReady(ctx, event); err != nil {
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
		_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusFailed, err.Error())
		return err
	} else if waiting {
		return nil
	}
	_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusPrechecked, "")
	if s.collectEventNeedsMediaCache(ctx, event) {
		if err = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusMediaPending, "媒体缓存中"); err != nil {
			return err
		}
		s.appendCollectEventLogForRecord(ctx, event, "media", "pending", "媒体等待缓存", "")
		return s.enqueueCollectMediaCache(ctx, collectMediaQueuePayload{
			EventId:     eventId,
			TenantId:    tenantId,
			AccountId:   accountId,
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
	if rule["review_enabled"].Int() == 1 {
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
	taskId, err := s.createCollectPublishTask(ctx, event, content, rule, decision.Text)
	if err != nil {
		return false, "", err
	}
	if err = s.ensureCollectTgJobs(ctx, taskId); err != nil {
		return false, "", err
	}
	if err = s.markCollectDispatchQueued(ctx, dispatchId, taskId); err != nil {
		return false, "", err
	}
	return true, "", nil
}

func (s *sSysPublish) attachCollectVerifyEventToPreviousTask(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record) (bool, error) {
	items, ok, err := s.collectVerifyOnlyEventItems(ctx, event, content)
	if err != nil || !ok {
		return false, err
	}
	previous, err := s.previousCollectPublishTaskForVerify(ctx, event, rule)
	if err != nil || previous.IsEmpty() {
		return false, err
	}
	taskId := previous["id"].Int64()
	if err = s.insertCollectPublishMediaRows(ctx, event, taskId, "verify", items); err != nil {
		return false, err
	}
	now := gtime.Now()
	if _, err = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", taskId).Data(g.Map{
		"media_count": gdb.Raw("media_count + " + fmt.Sprintf("%d", len(items))),
		"tg_status":   "pending",
		"status":      sysin.PublishTaskStatusPending,
		"updated_at":  now,
	}).Update(); err != nil {
		return false, gerror.Wrap(err, "绑定采集验证视频到资料失败")
	}
	if _, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("task_id", taskId).WhereIn("status", []string{"pending", "failed_retry"}).Data(g.Map{
		"status":          "pending",
		"dispatch_status": tgDispatchStatusIdle,
		"next_retry_at":   nil,
		"error_message":   "",
		"updated_at":      now,
	}).Update(); err != nil {
		return false, gerror.Wrap(err, "重置绑定验证视频TG任务失败")
	}
	s.appendCollectEventLogForRecord(ctx, event, "dispatch", "attached", "验证视频已绑定到前一个采集资料", fmt.Sprintf("task=%d", taskId))
	return true, nil
}

func (s *sSysPublish) collectVerifyOnlyEventItems(ctx context.Context, event gdb.Record, content *collectContentResult) ([]collectMediaItem, bool, error) {
	if strings.TrimSpace(event["raw_text"].String()) != "" {
		return nil, false, nil
	}
	if content != nil && strings.TrimSpace(content.RawText) != "" {
		return nil, false, nil
	}
	if event["media_count"].Int() <= 0 {
		return nil, false, nil
	}
	rows, err := s.collectEventMediaRows(ctx, event["id"].Int64())
	if err != nil {
		return nil, false, err
	}
	items := collectMediaRowsToItems(rows)
	if len(items) == 0 {
		return nil, false, nil
	}
	for _, item := range items {
		if strings.TrimSpace(item.Type) != "video" {
			return nil, false, nil
		}
	}
	return items, true, nil
}

func (s *sSysPublish) previousCollectPublishTaskForVerify(ctx context.Context, event gdb.Record, rule gdb.Record) (gdb.Record, error) {
	row, err := g.DB().Model(publishTaskTable+" t").Safe().Ctx(ctx).
		LeftJoin(pdaoCollectEventTable()+" e", "e.id=t.collect_event_id").
		Fields("t.id,t.status,t.tg_status,t.channel_id_json").
		Where("t.tenant_id", event["tenant_id"].Int64()).
		Where("t.account_id", event["account_id"].Int64()).
		Where("t.collect_source_id", event["source_id"].Int64()).
		Where("t.collect_source_chat_id", strings.TrimSpace(event["source_chat_id"].String())).
		Where("t.collect_source_message_id > 0").
		Where("t.collect_source_message_id < ?", event["source_message_id"].Int64()).
		Where("t.channel_id_json", rule["target_channel_id_json"].String()).
		Where("t.deleted_at IS NULL").
		Where("e.media_count > 0").
		Where("e.source_grouped_id <> ''").
		Where("COALESCE(e.raw_text, '') <> ''").
		Where("NOT EXISTS (SELECT 1 FROM "+pdaoCollectEventTable()+" mid WHERE mid.tenant_id=t.tenant_id AND mid.account_id=t.account_id AND mid.source_id=t.collect_source_id AND mid.source_chat_id=t.collect_source_chat_id AND mid.source_message_id > t.collect_source_message_id AND mid.source_message_id < ? AND mid.media_count > 0 AND COALESCE(mid.raw_text, '') <> '')", event["source_message_id"].Int64()).
		OrderDesc("t.collect_source_message_id").
		OrderDesc("t.id").
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
	mediaJSON, mediaCount := mergeCollectMediaJSON(review["media_json"].String(), content.MediaJSON)
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
	mediaJSON := event["media_json"].String()
	if content != nil {
		mediaCount = content.MediaCount
		mediaJSON = content.MediaJSON
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
	_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where("id", dispatchId).Data(g.Map{
		"review_id":  reviewId,
		"status":     sysin.CollectDispatchStatusReviewing,
		"updated_at": now,
	}).Update()
	return gerror.Wrap(err, "更新采集审核分发失败")
}

func (s *sSysPublish) createCollectPublishTask(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record, text string) (int64, error) {
	text = strings.TrimSpace(text)
	mediaCount := event["media_count"].Int()
	if content != nil {
		mediaCount = content.MediaCount
	}
	if err := ensureCollectTelegramOrderColumns(ctx); err != nil {
		return 0, err
	}
	title := collectTitle(text)
	channelJSON := rule["target_channel_id_json"].String()
	now := gtime.Now()
	clientRequestId := collectPublishClientRequestId(event, rule)
	if strings.EqualFold(g.DB().GetConfig().Type, consts.DBPgsql) {
		return s.upsertCollectPublishTaskPgsql(ctx, event, content, clientRequestId, title, text, mediaCount, channelJSON, now)
	}
	existing, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Fields("id,tg_operation_no,status,deleted_at").
		Where("tenant_id", event["tenant_id"].Int64()).
		Where("client_request_id", clientRequestId).
		One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取采集上架任务失败")
	}
	if !existing.IsEmpty() {
		taskId := existing["id"].Int64()
		operationNo := newTelegramOperationNo(telegramPublishBizCollect, taskId)
		_, err = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", taskId).Data(g.Map{
			"profile_id":                0,
			"title":                     title,
			"plain_text":                text,
			"media_count":               mediaCount,
			"channel_id_json":           channelJSON,
			"collect_event_id":          event["id"].Int64(),
			"collect_source_id":         event["source_id"].Int64(),
			"collect_source_chat_id":    strings.TrimSpace(event["source_chat_id"].String()),
			"collect_source_message_id": event["source_message_id"].Int64(),
			"tg_push_enabled":           1,
			"tg_status":                 "pending",
			"tg_operation_no":           operationNo,
			"status":                    sysin.PublishTaskStatusPending,
			"submitted_at":              now,
			"updated_at":                now,
		}).Update()
		if err != nil {
			return 0, gerror.Wrap(err, "更新采集上架任务失败")
		}
		if err = s.createCollectPublishMedia(ctx, event, content, taskId); err != nil {
			return 0, err
		}
		return taskId, nil
	}
	taskId, err := pdao.YoubanPublishTask.Ctx(ctx).Data(g.Map{
		"tenant_id":                 event["tenant_id"].Int64(),
		"merchant_id":               event["tenant_id"].Int64(),
		"account_id":                event["account_id"].Int64(),
		"client_request_id":         clientRequestId,
		"title":                     title,
		"plain_text":                text,
		"media_count":               mediaCount,
		"channel_id_json":           channelJSON,
		"collect_event_id":          event["id"].Int64(),
		"collect_source_id":         event["source_id"].Int64(),
		"collect_source_chat_id":    strings.TrimSpace(event["source_chat_id"].String()),
		"collect_source_message_id": event["source_message_id"].Int64(),
		"tg_push_enabled":           1,
		"tg_status":                 "pending",
		"status":                    sysin.PublishTaskStatusPending,
		"submitted_at":              now,
		"created_at":                now,
		"updated_at":                now,
	}).InsertAndGetId()
	if err != nil {
		if isCollectPublishTaskUniqueConflict(err) {
			existing, readErr := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
				Fields("id,tg_operation_no,status,deleted_at").
				Where("tenant_id", event["tenant_id"].Int64()).
				Where("client_request_id", clientRequestId).
				One()
			if readErr != nil {
				return 0, gerror.Wrap(readErr, "读取冲突采集上架任务失败")
			}
			if !existing.IsEmpty() {
				return s.updateCollectPublishTask(ctx, event, content, existing["id"].Int64(), title, text, mediaCount, channelJSON, now)
			}
		}
		return 0, gerror.Wrap(err, "创建采集上架任务失败")
	}
	if err = s.createCollectPublishMedia(ctx, event, content, taskId); err != nil {
		return 0, err
	}
	return taskId, nil
}

func (s *sSysPublish) upsertCollectPublishTaskPgsql(ctx context.Context, event gdb.Record, content *collectContentResult, clientRequestId string, title string, text string, mediaCount int, channelJSON string, now *gtime.Time) (int64, error) {
	sql := fmt.Sprintf(`
INSERT INTO "%s"(
	"tenant_id","merchant_id","account_id","client_request_id","title","plain_text","media_count","channel_id_json",
	"collect_event_id","collect_source_id","collect_source_chat_id","collect_source_message_id",
	"tg_push_enabled","tg_status","status","submitted_at","created_at","updated_at"
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,1,?,?,?, ?,?)
ON CONFLICT ("tenant_id","client_request_id") WHERE "client_request_id" <> ''
DO UPDATE SET
	"profile_id"=0,
	"title"=EXCLUDED."title",
	"plain_text"=EXCLUDED."plain_text",
	"media_count"=EXCLUDED."media_count",
	"channel_id_json"=EXCLUDED."channel_id_json",
	"collect_event_id"=EXCLUDED."collect_event_id",
	"collect_source_id"=EXCLUDED."collect_source_id",
	"collect_source_chat_id"=EXCLUDED."collect_source_chat_id",
	"collect_source_message_id"=EXCLUDED."collect_source_message_id",
	"tg_push_enabled"=1,
	"tg_status"=EXCLUDED."tg_status",
	"status"=EXCLUDED."status",
	"deleted_at"=NULL,
	"submitted_at"=EXCLUDED."submitted_at",
	"updated_at"=EXCLUDED."updated_at"
RETURNING "id"`, publishTaskTable)
	value, err := g.DB().GetValue(ctx, sql,
		event["tenant_id"].Int64(),
		event["tenant_id"].Int64(),
		event["account_id"].Int64(),
		clientRequestId,
		title,
		text,
		mediaCount,
		channelJSON,
		event["id"].Int64(),
		event["source_id"].Int64(),
		strings.TrimSpace(event["source_chat_id"].String()),
		event["source_message_id"].Int64(),
		"pending",
		sysin.PublishTaskStatusPending,
		now,
		now,
		now,
	)
	if err != nil {
		return 0, gerror.Wrap(err, "保存采集上架任务失败")
	}
	taskId := value.Int64()
	if taskId <= 0 {
		return 0, gerror.New("保存采集上架任务失败")
	}
	operationNo := newTelegramOperationNo(telegramPublishBizCollect, taskId)
	if _, err = g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", taskId).Data(g.Map{
		"tg_operation_no": operationNo,
		"profile_id":      0,
		"updated_at":      now,
	}).Update(); err != nil {
		return 0, gerror.Wrap(err, "更新采集上架任务操作号失败")
	}
	if err = s.createCollectPublishMedia(ctx, event, content, taskId); err != nil {
		return 0, err
	}
	return taskId, nil
}

func isCollectPublishTaskUniqueConflict(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "uk_ybp_task_tenant_client_request") ||
		strings.Contains(message, "duplicate key value")
}

func (s *sSysPublish) updateCollectPublishTask(ctx context.Context, event gdb.Record, content *collectContentResult, taskId int64, title string, text string, mediaCount int, channelJSON string, now *gtime.Time) (int64, error) {
	if taskId <= 0 {
		return 0, gerror.New("采集上架任务不存在")
	}
	operationNo := newTelegramOperationNo(telegramPublishBizCollect, taskId)
	_, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).Where("id", taskId).Data(g.Map{
		"profile_id":                0,
		"title":                     title,
		"plain_text":                text,
		"media_count":               mediaCount,
		"channel_id_json":           channelJSON,
		"collect_event_id":          event["id"].Int64(),
		"collect_source_id":         event["source_id"].Int64(),
		"collect_source_chat_id":    strings.TrimSpace(event["source_chat_id"].String()),
		"collect_source_message_id": event["source_message_id"].Int64(),
		"tg_push_enabled":           1,
		"tg_status":                 "pending",
		"tg_operation_no":           operationNo,
		"status":                    sysin.PublishTaskStatusPending,
		"deleted_at":                nil,
		"submitted_at":              now,
		"updated_at":                now,
	}).Update()
	if err != nil {
		return 0, gerror.Wrap(err, "更新冲突采集上架任务失败")
	}
	if err = s.createCollectPublishMedia(ctx, event, content, taskId); err != nil {
		return 0, err
	}
	return taskId, nil
}
