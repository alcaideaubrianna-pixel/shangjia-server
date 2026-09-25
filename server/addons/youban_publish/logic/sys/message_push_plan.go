package sys

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botService "hotgo/addons/youban_bot/service"
	publishdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
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
	IntervalDays    int         `json:"intervalDays"`
	IntervalSeconds int         `json:"intervalSeconds"`
	Status          int         `json:"status"`
	PushMode        string      `json:"pushMode"`
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
	account, err := s.ensureGroupPushCapability(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in == nil {
		in = &sysin.MessagePushPlanListInp{}
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
	models, err := s.messagePushPlanModels(ctx, records)
	if err != nil {
		return nil, 0, err
	}
	groups := make(map[int64][]string)
	for _, item := range models {
		if item == nil {
			continue
		}
		groups[item.AccountId] = append(groups[item.AccountId], item.TargetChatIds...)
	}
	labels, err := s.resolveTargetChatLabels(ctx, account.TenantId, groups)
	if err != nil {
		return nil, 0, gerror.Wrap(err, "读取消息推送目标名称失败")
	}
	for _, item := range models {
		if item != nil {
			item.TargetChatLabels = labels[item.AccountId]
		}
	}
	return models, totalCount, nil
}

func (s *sSysPublish) AdminMessagePushPlanSave(ctx context.Context, in *sysin.MessagePushPlanSaveInp) (res *sysin.MessagePushPlanSaveModel, err error) {
	account, err := s.ensureGroupPushCapability(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("消息推送计划不能为空")
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	if in.PushMode == "" {
		in.PushMode = sysin.MessageTemplatePushModeBot
	}
	if in.PushMode != sysin.MessageTemplatePushModeBot && in.PushMode != sysin.MessageTemplatePushModeAccount {
		return nil, gerror.New("消息推送方式不合法")
	}
	if in.Id > 0 {
		if err = s.ensureMessagePushPlansBelongTenant(ctx, []int64{in.Id}, account.TenantId); err != nil {
			return nil, err
		}
	}
	if err = s.ensureMessagePushTgAccountBelongTenant(ctx, in.AccountId, account.TenantId); err != nil {
		return nil, err
	}
	if in.TemplateIds, err = s.filterDeletedMessageTemplates(ctx, in.TemplateIds, account.TenantId); err != nil {
		return nil, err
	}
	if err = s.ensureMessagePushTargetCaches(ctx, in.AccountId, in.TargetChatIds, account.TenantId); err != nil {
		return nil, err
	}
	now := gtime.Now()
	nextRunAt := firstMessagePushPlanRunAt(in.Times, now)
	intervalDays := in.IntervalDays
	vip, vipErr := s.tenantVipStatus(ctx, account.TenantId)
	if vipErr != nil {
		return nil, vipErr
	}
	if !tenantVipStatusActive(vip) {
		intervalDays = s.freeMessagePushPlanIntervalDays(ctx)
	}
	if in.Id > 0 {
		storedPlan, loadErr := s.messagePushPlanById(ctx, in.Id, account.TenantId)
		if loadErr != nil {
			return nil, loadErr
		}
		if !tenantVipStatusActive(vip) {
			// Keep the member's custom value for renewal; free/expired plans use ENV at runtime.
			intervalDays = storedPlan.IntervalDays
		}
	}
	data := g.Map{
		"tenant_id":        account.TenantId,
		"name":             in.Name,
		"account_id":       in.AccountId,
		"template_ids":     mustJsonEncode(in.TemplateIds),
		"target_chat_ids":  mustJsonEncode(in.TargetChatIds),
		"times":            mustJsonEncode(in.Times),
		"interval_days":    intervalDays,
		"interval_seconds": in.IntervalSeconds,
		"status":           in.Status,
		"push_mode":        in.PushMode,
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
	account, err := s.ensureGroupPushCapability(ctx)
	if err != nil {
		return err
	}
	if in == nil || len(in.Ids) == 0 {
		return gerror.New("请选择要删除的消息推送计划")
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
	account, err := s.ensureGroupPushCapability(ctx)
	if err != nil {
		return err
	}
	if in == nil {
		return gerror.New("计划状态不能为空")
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
		data["next_run_at"] = firstMessagePushPlanRunAt(decodeStringArray(plan.Times), gtime.Now())
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
	if err := s.repairMessagePushPlanSchedules(ctx); err != nil {
		g.Log().Warningf(ctx, "修复消息推送计划时间失败：%+v", err)
	}
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

func (s *sSysPublish) repairMessagePushPlanSchedules(ctx context.Context) error {
	lock := hglock.NewConfig(30*time.Second, 100*time.Millisecond).Mutex("youban_publish:message_push_plan")
	if err := lock.TryLock(ctx); err != nil {
		if gerror.Is(err, hglock.ErrLockFailed) {
			return nil
		}
		return gerror.Wrap(err, "获取消息推送计划修复锁失败")
	}
	defer s.releaseTelegramChannelLease(ctx, lock)

	var plans []messagePushPlanRecord
	if err := publishdao.YoubanPublishMessagePushPlan.Ctx(ctx).
		Where("status", 1).
		WhereNull("deleted_at").
		OrderAsc("id").
		Scan(&plans); err != nil {
		return gerror.Wrap(err, "读取待修复消息推送计划失败")
	}
	now := gtime.Now()
	for _, plan := range plans {
		nextRunAt := messagePushPlanNextRunAtFromRecord(decodeStringArray(plan.Times), s.effectiveMessagePushPlanIntervalDays(ctx, plan), plan.LastRunAt, now)
		if messagePushPlanSameWallClock(plan.NextRunAt, nextRunAt) {
			continue
		}
		if _, err := publishdao.YoubanPublishMessagePushPlan.Ctx(ctx).
			Where("id", plan.Id).
			Where("status", 1).
			WhereNull("deleted_at").
			Data(g.Map{
				"next_run_at": nextRunAt,
				"locked_at":   nil,
				"updated_at":  now,
			}).
			Update(); err != nil {
			return gerror.Wrapf(err, "修复消息推送计划时间失败 plan:%d", plan.Id)
		}
		g.Log().Infof(ctx, "消息推送计划时间已修复 plan:%d nextRunAt:%s", plan.Id, nextRunAt.Format("Y-m-d H:i:s"))
	}
	return nil
}

func (s *sSysPublish) executeDueMessagePushPlans(ctx context.Context, limit int) error {
	if err := s.pauseInactiveFreeMessagePushPlans(ctx); err != nil {
		g.Log().Warningf(ctx, "暂停长期未活跃免费循环计划失败：%+v", err)
	}
	if limit <= 0 {
		limit = 20
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

// pauseInactiveFreeMessagePushPlans releases scheduler resources held by tenants
// that no longer use the backend. Membership and thresholds are evaluated at run time.
func (s *sSysPublish) pauseInactiveFreeMessagePushPlans(ctx context.Context) error {
	inactiveDays := g.Cfg().MustGet(ctx, "youbanPublish.messagePush.freeInactiveDays", 15).Int()
	if inactiveDays <= 0 {
		inactiveDays = 15
	}
	cutoff := gtime.Now().Add(-time.Duration(inactiveDays) * 24 * time.Hour)
	var plans []*messagePushPlanRecord
	if err := publishdao.YoubanPublishMessagePushPlan.Ctx(ctx).Where("status", 1).WhereNull("deleted_at").Scan(&plans); err != nil {
		return gerror.Wrap(err, "读取循环计划失败")
	}
	accountCols := publishdao.YoubanPublishAccount.Columns()
	planCols := publishdao.YoubanPublishMessagePushPlan.Columns()
	for _, plan := range plans {
		vip, err := s.tenantVipStatus(ctx, plan.TenantId)
		if err != nil || tenantVipStatusActive(vip) {
			continue
		}
		lastActive := new(gtime.Time)
		if err = publishdao.YoubanPublishAccount.Ctx(ctx).Fields(accountCols.LastActiveAt).
			Where(accountCols.TenantId, plan.TenantId).
			Where(accountCols.AccountType, sysin.PublishAccountTypeAdmin).
			Where(accountCols.Status, 1).OrderDesc(accountCols.LastActiveAt).Limit(1).Scan(&lastActive); err != nil {
			return err
		}
		if lastActive.IsZero() || lastActive.Before(cutoff) {
			_, err = publishdao.YoubanPublishMessagePushPlan.Ctx(ctx).Where(planCols.Id, plan.Id).Where(planCols.Status, 1).Data(g.Map{
				planCols.Status:     2,
				planCols.NextRunAt:  nil,
				planCols.LastResult: "为避免长期未使用的循环推送计划持续占用服务器资源，保障整体服务稳定运行，该计划已暂时暂停。重新进入后台后可手动恢复。",
			}).Update()
			if err != nil {
				return gerror.Wrap(err, "暂停循环计划失败")
			}
			notifyId, notifyErr := s.tenantVipNotifyAccountId(ctx, plan.TenantId, 0)
			if notifyErr == nil && notifyId > 0 {
				if notifyErr = botService.SysBot().NotifyAccount(ctx, &botsysin.NotifyAccountInp{
					BotStrategy: "official", FallbackBoundBot: true, IgnoreFeatureSwitch: true,
					RequireDelivery: true, App: consts.AppApi, AccountId: notifyId,
					Text: "为避免长期未使用的循环推送计划持续占用服务器资源，保障整体服务稳定运行，该计划已暂时暂停。重新进入后台后，可前往群聊推送页面手动恢复。",
				}); notifyErr != nil {
					g.Log().Warningf(ctx, "发送循环计划暂停通知失败 tenantId:%d planId:%d err:%+v", plan.TenantId, plan.Id, notifyErr)
				}
			}
		}
	}
	return nil
}

func (s *sSysPublish) executeMessagePushPlan(ctx context.Context, plan messagePushPlanRecord) error {
	now := gtime.Now()
	scheduledAt := messagePushPlanWallClock(plan.NextRunAt)
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
		return s.finishMessagePushPlan(ctx, plan, scheduledAt, now, times, err.Error())
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
		targets := make([]*messageTemplatePushTarget, 0, len(channels))
		for channelIndex, channel := range channels {
			delay := time.Duration(delayIndex*plan.IntervalSeconds) * time.Second
			operationNo := messagePushPlanOperationNo(plan.Id, scheduledAt, template, channel.TargetChatId)
			targets = append(targets, &messageTemplatePushTarget{Channel: channel, AccountId: plan.AccountId, OperationNo: operationNo, Delay: delay, Priority: tgJobPriorityBulk, QueueName: tgQueueNameBulk, PushMode: plan.PushMode})
			if shouldWaitMessagePushPlan(templateIndex, channelIndex, len(templateIds), len(channels)) {
				delayIndex++
			}
		}
		batch := s.queueMessageTemplateTargets(ctx, template, targets, plan.TenantId, plan.AccountId)
		success += batch.Success
		failed += batch.Failed
		for _, result := range batch.Results {
			if result == nil {
				continue
			}
			if result.Status == sysin.MessagePushStatusPending {
				queued++
			}
			if result.Status == sysin.MessagePushStatusFailed && strings.TrimSpace(result.Message) != "" {
				messages = append(messages, result.Message)
			}
		}
	}
	lastResult := mustJsonEncode(g.Map{"total": total, "queued": queued, "success": success, "failed": failed, "messages": messages})
	return s.finishMessagePushPlan(ctx, plan, scheduledAt, now, times, lastResult)
}

func (s *sSysPublish) finishMessagePushPlan(ctx context.Context, plan messagePushPlanRecord, scheduledAt, now *gtime.Time, times []string, lastResult string) error {
	_, err := publishdao.YoubanPublishMessagePushPlan.Ctx(ctx).
		Where("id", plan.Id).
		Data(g.Map{
			"last_run_at": now,
			"last_result": lastResult,
			"next_run_at": nextMessagePushPlanRunAt(times, s.effectiveMessagePushPlanIntervalDays(ctx, plan), scheduledAt, now),
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

func (s *sSysPublish) messagePushPlanModels(ctx context.Context, records []*messagePushPlanRecord) ([]*sysin.MessagePushPlanModel, error) {
	list := make([]*sysin.MessagePushPlanModel, 0, len(records))
	for _, item := range records {
		if item == nil {
			continue
		}
		intervalDays := normalizeMessagePushPlanIntervalDays(item.IntervalDays)
		vip, err := s.tenantVipStatus(ctx, item.TenantId)
		if err != nil {
			return nil, gerror.Wrap(err, "读取消息推送计划会员状态失败")
		}
		if !tenantVipStatusActive(vip) {
			intervalDays = s.freeMessagePushPlanIntervalDays(ctx)
		}
		list = append(list, &sysin.MessagePushPlanModel{
			Id:              item.Id,
			TenantId:        item.TenantId,
			Name:            item.Name,
			AccountId:       item.AccountId,
			TemplateIds:     decodeInt64Array(item.TemplateIds),
			TargetChatIds:   decodeStringArray(item.TargetChatIds),
			Times:           decodeStringArray(item.Times),
			IntervalDays:    intervalDays,
			IntervalSeconds: item.IntervalSeconds,
			Status:          item.Status,
			NextRunAt:       messagePushPlanWallClock(item.NextRunAt),
			LastRunAt:       messagePushPlanWallClock(item.LastRunAt),
			LastResult:      item.LastResult,
			CreatedBy:       item.CreatedBy,
			UpdatedBy:       item.UpdatedBy,
			DeletedBy:       item.DeletedBy,
			CreatedAt:       item.CreatedAt,
			UpdatedAt:       item.UpdatedAt,
			DeletedAt:       item.DeletedAt,
		})
	}
	return list, nil
}

func firstMessagePushPlanRunAt(times []string, now *gtime.Time) *gtime.Time {
	if now == nil {
		now = gtime.Now()
	}
	base := now.Time.In(time.Local)
	parsedTimes := parseMessagePushPlanTimes(times)
	if len(parsedTimes) == 0 {
		return gtime.NewFromTime(base.AddDate(0, 0, 1))
	}
	day := messagePushPlanDay(base)
	if next, ok := messagePushPlanTimeAfter(day, parsedTimes, base); ok {
		return gtime.NewFromTime(next)
	}
	return gtime.NewFromTime(messagePushPlanTime(day.AddDate(0, 0, 1), parsedTimes[0]))
}

func nextMessagePushPlanRunAt(times []string, intervalDays int, scheduledAt, now *gtime.Time) *gtime.Time {
	if now == nil {
		now = gtime.Now()
	}
	if scheduledAt == nil {
		scheduledAt = now
	}
	intervalDays = normalizeMessagePushPlanIntervalDays(intervalDays)
	base := now.Time.In(time.Local)
	scheduled := messagePushPlanWallClock(scheduledAt).Time
	parsedTimes := parseMessagePushPlanTimes(times)
	if len(parsedTimes) == 0 {
		return gtime.NewFromTime(base.AddDate(0, 0, intervalDays))
	}
	scheduledDay := messagePushPlanDay(scheduled)
	if scheduledDay.Equal(messagePushPlanDay(base)) {
		threshold := base
		if scheduled.After(threshold) {
			threshold = scheduled
		}
		if next, ok := messagePushPlanTimeAfter(scheduledDay, parsedTimes, threshold); ok {
			return gtime.NewFromTime(next)
		}
	}
	activeDay := scheduledDay.AddDate(0, 0, intervalDays)
	currentDay := messagePushPlanDay(base)
	if activeDay.Before(currentDay) {
		daysBehind := calendarDayDiff(activeDay, currentDay)
		steps := daysBehind / intervalDays
		activeDay = activeDay.AddDate(0, 0, steps*intervalDays)
		if activeDay.Before(currentDay) {
			activeDay = activeDay.AddDate(0, 0, intervalDays)
		}
	}
	if activeDay.Equal(currentDay) {
		if next, ok := messagePushPlanTimeAfter(activeDay, parsedTimes, base); ok {
			return gtime.NewFromTime(next)
		}
		activeDay = activeDay.AddDate(0, 0, intervalDays)
	}
	return gtime.NewFromTime(messagePushPlanTime(activeDay, parsedTimes[0]))
}

func messagePushPlanNextRunAtFromRecord(times []string, intervalDays int, lastRunAt, now *gtime.Time) *gtime.Time {
	if lastRunAt == nil {
		return firstMessagePushPlanRunAt(times, now)
	}
	return nextMessagePushPlanRunAt(times, intervalDays, messagePushPlanWallClock(lastRunAt), now)
}

func messagePushPlanWallClock(value *gtime.Time) *gtime.Time {
	if value == nil {
		return nil
	}
	item := value.Time
	return gtime.NewFromTime(time.Date(
		item.Year(), item.Month(), item.Day(),
		item.Hour(), item.Minute(), item.Second(), item.Nanosecond(),
		time.Local,
	))
}

func messagePushPlanSameWallClock(left, right *gtime.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	leftTime := messagePushPlanWallClock(left).Time
	rightTime := messagePushPlanWallClock(right).Time
	return leftTime.Equal(rightTime)
}

func parseMessagePushPlanTimes(values []string) []time.Time {
	parsed := make([]time.Time, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if _, ok := seen[value]; ok || value == "" {
			continue
		}
		item, err := time.ParseInLocation("15:04:05", value, time.Local)
		if err != nil {
			continue
		}
		seen[value] = struct{}{}
		parsed = append(parsed, item)
	}
	sort.Slice(parsed, func(i, j int) bool {
		return parsed[i].Format("15:04:05") < parsed[j].Format("15:04:05")
	})
	return parsed
}

func messagePushPlanDay(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.Local)
}

func messagePushPlanTime(day time.Time, value time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), value.Hour(), value.Minute(), value.Second(), 0, time.Local)
}

func messagePushPlanTimeAfter(day time.Time, times []time.Time, threshold time.Time) (time.Time, bool) {
	for _, value := range times {
		candidate := messagePushPlanTime(day, value)
		if candidate.After(threshold) {
			return candidate, true
		}
	}
	return time.Time{}, false
}

func calendarDayDiff(from, to time.Time) int {
	fromUTC := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	toUTC := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	return int(toUTC.Sub(fromUTC) / (24 * time.Hour))
}

func normalizeMessagePushPlanIntervalDays(intervalDays int) int {
	if intervalDays <= 0 {
		return 1
	}
	return intervalDays
}

func (s *sSysPublish) effectiveMessagePushPlanIntervalDays(ctx context.Context, plan messagePushPlanRecord) int {
	vip, err := s.tenantVipStatus(ctx, plan.TenantId)
	if err == nil && tenantVipStatusActive(vip) {
		return normalizeMessagePushPlanIntervalDays(plan.IntervalDays)
	}
	return s.freeMessagePushPlanIntervalDays(ctx)
}

func (s *sSysPublish) freeMessagePushPlanIntervalDays(ctx context.Context) int {
	days := g.Cfg().MustGet(ctx, "youbanPublish.messagePush.freeIntervalDays", 9).Int()
	if days <= 0 {
		days = 9
	}
	return days
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
