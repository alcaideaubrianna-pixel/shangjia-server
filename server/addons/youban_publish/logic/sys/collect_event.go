package sys

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
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
	var event gdb.Record
	err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("id", eventId).
		Where("tenant_id", tenantId).
		Where("account_id", accountId).
		Scan(&event)
	if err != nil {
		return gerror.Wrap(err, "读取采集事件失败")
	}
	if event.IsEmpty() {
		return gerror.New("采集事件不存在")
	}
	rules, err := s.collectEventRules(ctx, event, tenantId, accountId)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusIgnored, "未命中可用规则")
	}
	matched := false
	for _, rule := range rules {
		ruleMatched, dispatchErr := s.dispatchCollectEventByRule(ctx, event, rule)
		if dispatchErr != nil {
			err = dispatchErr
			_ = s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusFailed, err.Error())
			return err
		}
		if ruleMatched {
			matched = true
		}
	}
	if !matched {
		return s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusIgnored, "未命中规则或被屏蔽")
	}
	return s.markCollectEvent(ctx, eventId, sysin.CollectEventStatusProcessed, "")
}

func (s *sSysPublish) collectEventRules(ctx context.Context, event gdb.Record, tenantId int64, accountId int64) ([]gdb.Record, error) {
	sourceId := event["source_id"].Int64()
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
	return rows, nil
}

func (s *sSysPublish) dispatchCollectEventByRule(ctx context.Context, event gdb.Record, rule gdb.Record) (bool, error) {
	decision, err := s.evaluateCollectRule(ctx, event, rule)
	if err != nil {
		return false, err
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
		return false, gerror.Wrap(err, "创建采集分发记录失败")
	}
	if decision.Skipped || !decision.Matched {
		_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where("id", dispatchId).Data(g.Map{
			"status":        sysin.CollectDispatchStatusSkipped,
			"error_message": decision.Reason,
			"updated_at":    gtime.Now(),
			"finished_at":   gtime.Now(),
		}).Update()
		return false, gerror.Wrap(err, "更新采集跳过记录失败")
	}
	if rule["review_enabled"].Int() == 1 {
		return true, s.createCollectReview(ctx, event, rule, dispatchId, decision.Text)
	}
	taskId, err := s.createCollectPublishTask(ctx, event, rule, decision.Text)
	if err != nil {
		return false, err
	}
	if err = s.ensureTgJobs(ctx, taskId); err != nil {
		return false, err
	}
	_, err = pdao.YoubanPublishCollectDispatch.Ctx(ctx).Where("id", dispatchId).Data(g.Map{
		"task_id":     taskId,
		"status":      sysin.CollectDispatchStatusSent,
		"updated_at":  gtime.Now(),
		"finished_at": gtime.Now(),
	}).Update()
	return true, gerror.Wrap(err, "更新采集分发记录失败")
}

func (s *sSysPublish) createCollectReview(ctx context.Context, event gdb.Record, rule gdb.Record, dispatchId int64, text string) error {
	now := gtime.Now()
	reviewId, err := pdao.YoubanPublishCollectReview.Ctx(ctx).Data(g.Map{
		"tenant_id":              event["tenant_id"].Int64(),
		"account_id":             event["account_id"].Int64(),
		"source_id":              event["source_id"].Int64(),
		"rule_id":                rule["id"].Int64(),
		"event_id":               event["id"].Int64(),
		"dispatch_id":            dispatchId,
		"raw_text":               text,
		"media_count":            event["media_count"].Int(),
		"media_json":             event["media_json"].String(),
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

func (s *sSysPublish) createCollectPublishTask(ctx context.Context, event gdb.Record, rule gdb.Record, text string) (int64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		text = strings.TrimSpace(event["raw_text"].String())
	}
	title := collectTitle(text)
	channelJSON := rule["target_channel_id_json"].String()
	now := gtime.Now()
	taskId, err := pdao.YoubanPublishTask.Ctx(ctx).Data(g.Map{
		"tenant_id":         event["tenant_id"].Int64(),
		"merchant_id":       event["tenant_id"].Int64(),
		"account_id":        event["account_id"].Int64(),
		"client_request_id": "collect:" + event["source_unique_key"].String(),
		"title":             title,
		"plain_text":        text,
		"media_count":       event["media_count"].Int(),
		"channel_id_json":   channelJSON,
		"tg_push_enabled":   1,
		"tg_status":         "pending",
		"status":            sysin.PublishTaskStatusPending,
		"submitted_at":      now,
		"created_at":        now,
		"updated_at":        now,
	}).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建采集上架任务失败")
	}
	return taskId, nil
}

func (s *sSysPublish) markCollectEvent(ctx context.Context, id int64, status string, message string) error {
	_, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", id).Data(g.Map{
		"status":        status,
		"error_message": message,
		"processed_at":  gtime.Now(),
		"updated_at":    gtime.Now(),
	}).Update()
	return err
}

func collectTitle(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "采集资料"
	}
	runes := []rune(text)
	if len(runes) > 48 {
		return string(runes[:48])
	}
	return text
}

func collectHash(value string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}
