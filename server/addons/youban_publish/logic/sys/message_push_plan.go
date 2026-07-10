package sys

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	publishdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	hglock "hotgo/internal/library/hgrds/lock"
)

type messagePushPlanRecord struct {
	Id              int64       `json:"id"`
	TenantId        int64       `json:"tenantId"`
	Name            string      `json:"name"`
	AccountId       int64       `json:"accountId"`
	TemplateIds     string      `json:"templateIds"`
	TargetChatIds   string      `json:"targetChatIds"`
	Times           string      `json:"times"`
	IntervalSeconds int         `json:"intervalSeconds"`
	Status          int         `json:"status"`
	NextRunAt       *gtime.Time `json:"nextRunAt"`
	LastRunAt       *gtime.Time `json:"lastRunAt"`
	LastResult      string      `json:"lastResult"`
	LockedAt        *gtime.Time `json:"lockedAt"`
	CreatedBy       int64       `json:"createdBy"`
	UpdatedBy       int64       `json:"updatedBy"`
	DeletedBy       int64       `json:"deletedBy"`
	CreatedAt       *gtime.Time `json:"createdAt"`
	UpdatedAt       *gtime.Time `json:"updatedAt"`
	DeletedAt       *gtime.Time `json:"deletedAt"`
}

func (s *sSysPublish) AdminMessagePushPlanList(ctx context.Context, in *sysin.MessagePushPlanListInp) (list []*sysin.MessagePushPlanModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.MessagePushPlanListInp{}
	}
	if err = ensureMessagePushTables(ctx); err != nil {
		return nil, 0, err
	}
	if err = in.Filter(ctx); err != nil {
		return nil, 0, err
	}
	mod := publishdao.YoubanPublishMessagePushPlan.Ctx(ctx).
		Where("tenant_id", account.TenantId).
		WhereNull("deleted_at")
	if in.Keyword != "" {
		mod = mod.WhereLike("name", "%"+in.Keyword+"%")
	}
	if in.Status > 0 {
		mod = mod.Where("status", in.Status)
	}
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取消息推送计划总数失败")
	}
	var records []*messagePushPlanRecord
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("id").Scan(&records); err != nil {
		return nil, 0, gerror.Wrap(err, "获取消息推送计划列表失败")
	}
	return messagePushPlanModels(records), totalCount, nil
}

func (s *sSysPublish) AdminMessagePushPlanSave(ctx context.Context, in *sysin.MessagePushPlanSaveInp) (res *sysin.MessagePushPlanSaveModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("消息推送计划不能为空")
	}
	if err = ensureMessagePushTables(ctx); err != nil {
		return nil, err
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if in.Id > 0 {
		if err = s.ensureMessagePushPlansBelongTenant(ctx, []int64{in.Id}, account.TenantId); err != nil {
			return nil, err
		}
	}
	if err = s.ensureMessagePushTgAccountBelongTenant(ctx, in.AccountId, account.TenantId); err != nil {
		return nil, err
	}
	if err = s.ensureMessageTemplatesBelongTenant(ctx, in.TemplateIds, account.TenantId); err != nil {
		return nil, err
	}
	if err = s.ensureMessagePushTargetCaches(ctx, in.AccountId, in.TargetChatIds, account.TenantId); err != nil {
		return nil, err
	}
	now := gtime.Now()
	nextRunAt := nextMessagePushPlanRunAt(in.Times, now)
	data := g.Map{
		"tenant_id":        account.TenantId,
		"name":             in.Name,
		"account_id":       in.AccountId,
		"template_ids":     mustJsonEncode(in.TemplateIds),
		"target_chat_ids":  mustJsonEncode(in.TargetChatIds),
		"times":            mustJsonEncode(in.Times),
		"interval_seconds": in.IntervalSeconds,
		"status":           in.Status,
		"next_run_at":      nextRunAt,
		"locked_at":        nil,
		"last_result":      "",
		"updated_by":       account.Id,
		"updated_at":       now,
	}
	if in.Status != 1 {
		data["next_run_at"] = nil
	}
	if in.Id > 0 {
		_, err = publishdao.YoubanPublishMessagePushPlan.Ctx(ctx).
			Where("id", in.Id).
			Where("tenant_id", account.TenantId).
			WhereNull("deleted_at").
			Data(data).
			Update()
		if err != nil {
			return nil, gerror.Wrap(err, "更新消息推送计划失败")
		}
	} else {
		data["created_by"] = account.Id
		data["created_at"] = now
		in.Id, err = publishdao.YoubanPublishMessagePushPlan.Ctx(ctx).Data(data).InsertAndGetId()
		if err != nil {
			return nil, gerror.Wrap(err, "新增消息推送计划失败")
		}
	}
	return &sysin.MessagePushPlanSaveModel{Id: in.Id}, nil
}

func (s *sSysPublish) AdminMessagePushPlanDelete(ctx context.Context, in *sysin.MessagePushPlanDeleteInp) error {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的消息推送计划")
	}
	if err = ensureMessagePushTables(ctx); err != nil {
		return err
	}
	in.Ids = uniqueIds(in.Ids)
	if err = s.ensureMessagePushPlansBelongTenant(ctx, in.Ids, account.TenantId); err != nil {
		return err
	}
	_, err = publishdao.YoubanPublishMessagePushPlan.Ctx(ctx).
		WhereIn("id", in.Ids).
		Where("tenant_id", account.TenantId).
		Data(g.Map{
			"deleted_by": account.Id,
			"deleted_at": gtime.Now(),
			"locked_at":  nil,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "删除消息推送计划失败")
	}
	return nil
}

func (s *sSysPublish) AdminMessagePushPlanStatus(ctx context.Context, in *sysin.MessagePushPlanStatusInp) error {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("计划状态不能为空")
	}
	if err = ensureMessagePushTables(ctx); err != nil {
		return err
	}
	if err = in.Filter(ctx); err != nil {
		return err
	}
	plan, err := s.messagePushPlanById(ctx, in.Id, account.TenantId)
	if err != nil {
		return err
	}
	data := g.Map{
		"status":     in.Status,
		"updated_by": account.Id,
		"updated_at": gtime.Now(),
		"locked_at":  nil,
	}
	if in.Status == 1 {
		data["next_run_at"] = nextMessagePushPlanRunAt(decodeStringArray(plan.Times), gtime.Now())
	} else {
		data["next_run_at"] = nil
	}
	_, err = publishdao.YoubanPublishMessagePushPlan.Ctx(ctx).
		Where("id", in.Id).
		Where("tenant_id", account.TenantId).
		WhereNull("deleted_at").
		Data(data).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新消息推送计划状态失败")
	}
	return nil
}

func (s *sSysPublish) runMessagePushPlanScheduler(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	time.Sleep(5 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.executeDueMessagePushPlans(ctx, 20); err != nil {
				g.Log().Warningf(ctx, "扫描消息推送计划失败：%+v", err)
			}
		}
	}
}

func (s *sSysPublish) executeDueMessagePushPlans(ctx context.Context, limit int) error {
	if limit <= 0 {
		limit = 20
	}
	if err := ensureMessagePushTables(ctx); err != nil {
		return err
	}
	lock := hglock.NewConfig(10*time.Second, 100*time.Millisecond).Mutex("youban_publish:message_push_plan")
	if err := lock.TryLock(ctx); err != nil {
		if gerror.Is(err, hglock.ErrLockFailed) {
			return nil
		}
		return gerror.Wrap(err, "获取消息推送计划锁失败")
	}
	defer s.releaseTelegramChannelLease(ctx, lock)
	now := gtime.Now()
	var plans []messagePushPlanRecord
	if err := publishdao.YoubanPublishMessagePushPlan.Ctx(ctx).
		Where("status", 1).
		WhereLTE("next_run_at", now).
		Wheref("(locked_at IS NULL OR locked_at < ?)", now.Add(-10*time.Minute)).
		WhereNull("deleted_at").
		OrderAsc("next_run_at").
		Limit(limit).
		Scan(&plans); err != nil {
		return gerror.Wrap(err, "读取到期消息推送计划失败")
	}
	for _, plan := range plans {
		if err := s.executeMessagePushPlan(ctx, plan); err != nil {
			g.Log().Warningf(ctx, "执行消息推送计划失败 plan:%d err:%+v", plan.Id, err)
		}
	}
	return nil
}

func (s *sSysPublish) executeMessagePushPlan(ctx context.Context, plan messagePushPlanRecord) error {
	now := gtime.Now()
	scheduledAt := plan.NextRunAt
	if scheduledAt == nil {
		scheduledAt = now
	}
	result, err := publishdao.YoubanPublishMessagePushPlan.Ctx(ctx).
		Where("id", plan.Id).
		Where("status", 1).
		WhereLTE("next_run_at", now).
		Wheref("(locked_at IS NULL OR locked_at < ?)", now.Add(-10*time.Minute)).
		Data(g.Map{"locked_at": now, "updated_at": now}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "锁定消息推送计划失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	templateIds := decodeInt64Array(plan.TemplateIds)
	targetChatIds := decodeStringArray(plan.TargetChatIds)
	times := decodeStringArray(plan.Times)
	channels, err := s.messagePushCachedTargets(ctx, plan.AccountId, targetChatIds, plan.TenantId)
	if err != nil {
		return s.finishMessagePushPlan(ctx, plan, now, times, err.Error())
	}
	success := 0
	failed := 0
	queued := 0
	delayIndex := 0
	messages := make([]string, 0)
	total := len(templateIds) * len(channels)
	for templateIndex, templateId := range templateIds {
		template, err := s.messageTemplate(ctx, templateId, plan.TenantId)
		if err != nil {
			failed += len(channels)
			messages = append(messages, err.Error())
			continue
		}
		for channelIndex, channel := range channels {
			delay := time.Duration(delayIndex*plan.IntervalSeconds) * time.Second
			operationNo := messagePushPlanOperationNo(plan.Id, scheduledAt, template, channel.TargetChatId)
			result := s.queueMessageTemplateToChannel(ctx, template, channel, plan.TenantId, plan.AccountId, operationNo, delay)
			if result.Status == sysin.MessagePushStatusPending {
				success++
				queued++
			} else {
				failed++
				messages = append(messages, result.Message)
			}
			if shouldWaitMessagePushPlan(templateIndex, channelIndex, len(templateIds), len(channels)) {
				delayIndex++
			}
		}
	}
	lastResult := mustJsonEncode(g.Map{"total": total, "queued": queued, "success": success, "failed": failed, "messages": messages})
	return s.finishMessagePushPlan(ctx, plan, now, times, lastResult)
}

func (s *sSysPublish) finishMessagePushPlan(ctx context.Context, plan messagePushPlanRecord, now *gtime.Time, times []string, lastResult string) error {
	_, err := publishdao.YoubanPublishMessagePushPlan.Ctx(ctx).
		Where("id", plan.Id).
		Data(g.Map{
			"last_run_at": now,
			"last_result": lastResult,
			"next_run_at": nextMessagePushPlanRunAt(times, now),
			"locked_at":   nil,
			"updated_at":  now,
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新消息推送计划执行结果失败")
	}
	return nil
}

func (s *sSysPublish) messagePushPlanById(ctx context.Context, id int64, tenantId int64) (messagePushPlanRecord, error) {
	var plan messagePushPlanRecord
	err := publishdao.YoubanPublishMessagePushPlan.Ctx(ctx).
		Where("id", id).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Scan(&plan)
	if err != nil {
		return plan, gerror.Wrap(err, "读取消息推送计划失败")
	}
	if plan.Id <= 0 {
		return plan, gerror.New("消息推送计划不存在")
	}
	return plan, nil
}

func (s *sSysPublish) ensureMessagePushPlansBelongTenant(ctx context.Context, ids []int64, tenantId int64) error {
	ids = uniqueIds(ids)
	if len(ids) == 0 {
		return gerror.New("请选择消息推送计划")
	}
	count, err := publishdao.YoubanPublishMessagePushPlan.Ctx(ctx).
		WhereIn("id", ids).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查消息推送计划权限失败")
	}
	if count != len(ids) {
		return gerror.New("存在无权操作的消息推送计划")
	}
	return nil
}

func messagePushPlanModels(records []*messagePushPlanRecord) []*sysin.MessagePushPlanModel {
	list := make([]*sysin.MessagePushPlanModel, 0, len(records))
	for _, item := range records {
		if item == nil {
			continue
		}
		list = append(list, &sysin.MessagePushPlanModel{
			Id:              item.Id,
			TenantId:        item.TenantId,
			Name:            item.Name,
			AccountId:       item.AccountId,
			TemplateIds:     decodeInt64Array(item.TemplateIds),
			TargetChatIds:   decodeStringArray(item.TargetChatIds),
			Times:           decodeStringArray(item.Times),
			IntervalSeconds: item.IntervalSeconds,
			Status:          item.Status,
			NextRunAt:       item.NextRunAt,
			LastRunAt:       item.LastRunAt,
			LastResult:      item.LastResult,
			CreatedBy:       item.CreatedBy,
			UpdatedBy:       item.UpdatedBy,
			DeletedBy:       item.DeletedBy,
			CreatedAt:       item.CreatedAt,
			UpdatedAt:       item.UpdatedAt,
			DeletedAt:       item.DeletedAt,
		})
	}
	return list
}

func nextMessagePushPlanRunAt(times []string, now *gtime.Time) *gtime.Time {
	if now == nil {
		now = gtime.Now()
	}
	base := now.Time
	var next time.Time
	for _, value := range times {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		candidate, err := time.ParseInLocation("2006-01-02 15:04:05", base.Format("2006-01-02")+" "+value, time.Local)
		if err != nil {
			continue
		}
		if !candidate.After(base) {
			candidate = candidate.Add(24 * time.Hour)
		}
		if next.IsZero() || candidate.Before(next) {
			next = candidate
		}
	}
	if next.IsZero() {
		next = base.Add(24 * time.Hour)
	}
	return gtime.NewFromTime(next)
}

func shouldWaitMessagePushPlan(templateIndex int, channelIndex int, templateCount int, channelCount int) bool {
	return templateIndex < templateCount-1 || channelIndex < channelCount-1
}

func messagePushPlanOperationNo(planId int64, scheduledAt *gtime.Time, template *sysin.MessageTemplateModel, targetChatId string) string {
	scheduled := int64(0)
	if scheduledAt != nil {
		scheduled = scheduledAt.Timestamp()
	}
	templateId := int64(0)
	if template != nil {
		templateId = template.Id
	}
	targetKey := strings.NewReplacer("-", "", ":", "", "@", "").Replace(normalizeTelegramChannelChatID(targetChatId))
	return "message_push_plan:" + strconv.FormatInt(planId, 10) + ":" + strconv.FormatInt(scheduled, 10) + ":" + strconv.FormatInt(templateId, 10) + ":" + targetKey + ":" + messageTemplateHash(template)
}

func mustJsonEncode(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func decodeInt64Array(value string) []int64 {
	var out []int64
	_ = json.Unmarshal([]byte(value), &out)
	if out == nil {
		return []int64{}
	}
	return out
}

func decodeStringArray(value string) []string {
	var out []string
	_ = json.Unmarshal([]byte(value), &out)
	if out == nil {
		return []string{}
	}
	return out
}
