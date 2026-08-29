package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botService "hotgo/addons/youban_bot/service"
	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/entity"
	"hotgo/addons/youban_publish/model/input/sysin"
	publishService "hotgo/addons/youban_publish/service"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
	"hotgo/internal/model"
	baseentity "hotgo/internal/model/entity"
	"hotgo/internal/service"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const tenantVipEventTable = "hg_youban_publish_tenant_vip_event"

const (
	tenantVipEventBindGift        = "bind_gift"
	tenantVipEventInviteBindGift  = "invite_bind_gift"
	tenantVipEventInviteFirstPay  = "invite_first_pay_gift"
	tenantVipEventPay             = "pay"
	tenantVipEventAdminAdjust     = "admin_adjust"
	tenantVipEventExpiringOneDay  = "expiring_1d"
	tenantVipEventExpiringSixHour = "expiring_6h"
	tenantVipEventExpired         = "expired"
)

const (
	tenantVipReminderOneDay  = 24 * time.Hour
	tenantVipReminderSixHour = 6 * time.Hour
)

func init() {
	botService.RegisterAccountBoundHook(func(ctx context.Context, event *botService.AccountBoundEvent) error {
		if event == nil || event.App != consts.AppApi {
			return nil
		}
		return publishService.SysPublish().HandleVipAccountBound(ctx, event.AccountId)
	})
}

type tenantVipChangeInp struct {
	EventKey           string
	EventType          string
	ActivityCode       string
	ActivityGeneration int
	TenantId           int64
	AccountId          int64
	TriggerTenantId    int64
	TriggerAccountId   int64
	ReferenceType      string
	ReferenceId        string
	Level              int
	Days               int
	Remark             string
}

type tenantVipChangeResult struct {
	Applied   bool
	ExpiredAt *gtime.Time
	EventId   int64
}

type tenantVipEventSummary struct {
	EventType string `json:"event_type"`
	Count     int    `json:"count"`
	Days      int    `json:"days"`
}

func (s *sSysPublish) tenantVipActivities(ctx context.Context, account *sysin.AccountModel) ([]*sysin.TenantVipActivityModel, *model.YoubanPublishVipActivityConfig, error) {
	cfg, err := service.SysConfig().GetYoubanPublishVipActivity(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !tenantVipActivityAccountEligible(account) {
		return []*sysin.TenantVipActivityModel{}, cfg, nil
	}
	var summaries []*tenantVipEventSummary
	err = g.DB().Model(tenantVipEventTable).Safe().Ctx(ctx).
		Fields("event_type,COUNT(*) AS count,COALESCE(SUM(change_days),0) AS days").
		Where("tenant_id", account.TenantId).
		WhereIn("event_type", []string{tenantVipEventBindGift, tenantVipEventInviteBindGift, tenantVipEventInviteFirstPay}).
		Group("event_type").
		Scan(&summaries)
	if err != nil {
		return nil, nil, gerror.Wrap(err, "读取会员活动记录失败")
	}
	stats := make(map[string]*tenantVipEventSummary, len(summaries))
	for _, item := range summaries {
		stats[item.EventType] = item
	}

	activities := make([]*sysin.TenantVipActivityModel, 0, 3)
	if cfg.BindGiftEnabled && cfg.BindGiftDays > 0 {
		bound, bindErr := s.tenantVipAccountTgBound(ctx, account.Id)
		if bindErr != nil {
			return nil, nil, bindErr
		}
		item := tenantVipActivityModel(tenantVipEventBindGift, "绑定 Telegram", "首次绑定 Telegram 后自动赠送会员，无需手动领取。", "bind_tg", cfg.BindGiftDays, stats[tenantVipEventBindGift])
		applyTenantVipBindActivityStatus(item, bound)
		activities = append(activities, item)
	}
	if cfg.InviteBindGiftEnabled && cfg.InviteBindGiftDays > 0 {
		activities = append(activities, tenantVipActivityModel(tenantVipEventInviteBindGift, "邀请好友绑定 TG", "好友通过邀请码注册并完成首次绑定后，奖励自动到账，可邀请多人叠加。", "invite", cfg.InviteBindGiftDays, stats[tenantVipEventInviteBindGift]))
	}
	if cfg.InviteFirstPayEnabled && cfg.InviteFirstPayDays > 0 {
		activities = append(activities, tenantVipActivityModel(tenantVipEventInviteFirstPay, "邀请好友首次开通", "好友首次真实付费购买月卡后奖励一次，同一好友重复续费不重复奖励。", "invite", cfg.InviteFirstPayDays, stats[tenantVipEventInviteFirstPay]))
	}
	return activities, cfg, nil
}

func applyTenantVipBindActivityStatus(item *sysin.TenantVipActivityModel, bound bool) {
	if item == nil {
		return
	}
	item.Completed = bound
	switch {
	case bound && item.RewardCount > 0:
		item.StatusText = "已绑定，奖励已到账"
	case bound:
		item.StatusText = "已绑定，奖励处理中"
	case item.RewardCount > 0:
		item.StatusText = "奖励已领取，当前未绑定"
	default:
		item.StatusText = "绑定后自动到账"
	}
}

func tenantVipActivityModel(code, title, description, action string, rewardDays int, summary *tenantVipEventSummary) *sysin.TenantVipActivityModel {
	model := &sysin.TenantVipActivityModel{
		Code:        code,
		Title:       title,
		Description: description,
		Action:      action,
		RewardDays:  rewardDays,
		StatusText:  "完成后自动到账",
	}
	if summary != nil {
		model.RewardCount = summary.Count
		model.RewardDaysTotal = summary.Days
	}
	return model
}

func (s *sSysPublish) tenantVipAccountTgBound(ctx context.Context, accountId int64) (bool, error) {
	count, err := g.DB().Model("hg_youban_bot_account_bind").Safe().Ctx(ctx).
		Where("app", consts.AppApi).
		Where("account_id", accountId).
		Where("status", consts.StatusEnabled).
		WhereNull("deleted_at").
		Count()
	return count > 0, gerror.Wrap(err, "读取Telegram绑定状态失败")
}

func (s *sSysPublish) handleTenantVipAccountBound(ctx context.Context, accountId int64) error {
	return s.handleTenantVipAccountBoundAt(ctx, accountId, gtime.Now())
}

func (s *sSysPublish) handleTenantVipAccountBoundAt(ctx context.Context, accountId int64, boundAt *gtime.Time) error {
	account, err := s.tenantVipAccountById(ctx, accountId)
	if err != nil {
		return err
	}
	if !tenantVipActivityAccountEligible(account) {
		return nil
	}
	boundAt, err = s.tenantVipAccountFirstBoundAt(ctx, account.Id, boundAt)
	if err != nil {
		return err
	}
	cfg, err := service.SysConfig().GetYoubanPublishVipActivity(ctx)
	if err != nil {
		return err
	}
	if err = s.applyTenantVipBindGift(ctx, account, boundAt, cfg); err != nil {
		return err
	}
	return s.applyTenantVipInviteBindGift(ctx, account, boundAt, cfg)
}

func (s *sSysPublish) tenantVipAccountFirstBoundAt(ctx context.Context, accountId int64, fallback *gtime.Time) (*gtime.Time, error) {
	if accountId <= 0 {
		return fallback, nil
	}
	value, err := g.DB().Model("hg_youban_bot_account_bind").Safe().Ctx(ctx).
		Fields("COALESCE(MIN(created_at),MIN(updated_at))").
		Where("app", consts.AppApi).
		Where("account_id", accountId).
		WhereNull("deleted_at").
		Value()
	if err != nil {
		return nil, gerror.Wrap(err, "读取Telegram首次绑定时间失败")
	}
	if value.IsNil() {
		return fallback, nil
	}
	firstBoundAt := value.GTime()
	if firstBoundAt == nil {
		return fallback, nil
	}
	return firstBoundAt, nil
}

func (s *sSysPublish) tenantVipAccountById(ctx context.Context, accountId int64) (*sysin.AccountModel, error) {
	if accountId <= 0 {
		return nil, nil
	}
	accountColumns := pdao.YoubanPublishAccount.Columns()
	var account *sysin.AccountModel
	if err := pdao.YoubanPublishAccount.Ctx(ctx).
		Where(accountColumns.Id, accountId).
		WhereNull(accountColumns.DeletedAt).
		Scan(&account); err != nil {
		return nil, gerror.Wrap(err, "读取绑定账号归属失败")
	}
	return account, nil
}

func (s *sSysPublish) applyTenantVipBindGift(ctx context.Context, account *sysin.AccountModel, boundAt *gtime.Time, cfg *model.YoubanPublishVipActivityConfig) error {
	if !tenantVipActivityAccountEligible(account) || cfg == nil || !cfg.BindGiftEnabled || cfg.BindGiftDays <= 0 || !tenantVipActivityTriggerEligible(boundAt, cfg.BindGiftEnabledAt) {
		return nil
	}
	eventKey, generation, err := s.tenantVipActivityEventIdentity(ctx, tenantVipEventBindGift, account.TenantId, fmt.Sprintf("%s:%d", tenantVipEventBindGift, account.TenantId))
	if err != nil {
		return err
	}
	_, err = s.applyTenantVipExtension(ctx, &tenantVipChangeInp{
		EventKey:           eventKey,
		EventType:          tenantVipEventBindGift,
		ActivityCode:       tenantVipEventBindGift,
		ActivityGeneration: generation,
		TenantId:           account.TenantId,
		AccountId:          account.Id,
		ReferenceType:      "tg_bind",
		ReferenceId:        fmt.Sprintf("%d", account.Id),
		Level:              1,
		Days:               cfg.BindGiftDays,
		Remark:             "首次绑定Telegram赠送会员",
	})
	return err
}

func (s *sSysPublish) applyTenantVipInviteBindGift(ctx context.Context, account *sysin.AccountModel, boundAt *gtime.Time, cfg *model.YoubanPublishVipActivityConfig) error {
	if !tenantVipActivityAccountEligible(account) || cfg == nil {
		return nil
	}
	if !cfg.InviteBindGiftEnabled || cfg.InviteBindGiftDays <= 0 || !tenantVipActivityTriggerEligible(boundAt, cfg.InviteBindGiftEnabledAt) {
		return nil
	}
	invite, err := s.inviteByUsedTenant(ctx, account.TenantId)
	if err != nil || invite == nil || invite.InviterTenantId <= 0 || invite.InviterTenantId == account.TenantId {
		return err
	}
	eventKey, generation, err := s.tenantVipActivityEventIdentity(ctx, tenantVipEventInviteBindGift, invite.InviterTenantId, fmt.Sprintf("%s:%d:%d", tenantVipEventInviteBindGift, invite.InviterTenantId, account.TenantId))
	if err != nil {
		return err
	}
	_, err = s.applyTenantVipExtension(ctx, &tenantVipChangeInp{
		EventKey:           eventKey,
		EventType:          tenantVipEventInviteBindGift,
		ActivityCode:       tenantVipEventInviteBindGift,
		ActivityGeneration: generation,
		TenantId:           invite.InviterTenantId,
		AccountId:          invite.InviterAccountId,
		TriggerTenantId:    account.TenantId,
		TriggerAccountId:   account.Id,
		ReferenceType:      "invite",
		ReferenceId:        fmt.Sprintf("%d", invite.Id),
		Level:              1,
		Days:               cfg.InviteBindGiftDays,
		Remark:             "邀请好友绑定Telegram奖励",
	})
	return err
}

func tenantVipActivityAccountEligible(account *sysin.AccountModel) bool {
	return account != nil && account.TenantId > 0 && account.AccountType == sysin.PublishAccountTypeAdmin
}

func (s *sSysPublish) HandleVipAccountBound(ctx context.Context, accountId int64) error {
	return s.handleTenantVipAccountBound(ctx, accountId)
}

func tenantVipActivityTriggerEligible(triggerAt *gtime.Time, enabledAtText string) bool {
	enabledAt := gtime.NewFromStr(strings.TrimSpace(enabledAtText))
	if enabledAt == nil || triggerAt == nil {
		return true
	}
	return !triggerAt.Before(enabledAt)
}

func (s *sSysPublish) inviteByUsedTenant(ctx context.Context, tenantId int64) (*webInviteRow, error) {
	var row *webInviteRow
	err := g.DB().Model(botInviteUsageTable).Safe().Ctx(ctx).
		Where("used_tenant_id", tenantId).
		WhereNull("deleted_at").
		OrderDesc("used_at").
		Scan(&row)
	return row, gerror.Wrap(err, "读取邀请关系失败")
}

func (s *sSysPublish) applyTenantVipExtension(ctx context.Context, in *tenantVipChangeInp) (*tenantVipChangeResult, error) {
	if in == nil || in.TenantId <= 0 || in.Level <= 0 || in.Days <= 0 {
		return nil, gerror.New("会员变更参数不完整")
	}
	in.EventKey = strings.TrimSpace(in.EventKey)
	if in.EventKey == "" {
		in.EventKey = fmt.Sprintf("%s:%d:%d", in.EventType, in.TenantId, gtime.Now().TimestampNano())
	}
	result := &tenantVipChangeResult{}
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		now := gtime.Now()
		eventId, err := tx.Model(tenantVipEventTable).Safe().Ctx(ctx).Data(g.Map{
			"event_key":           in.EventKey,
			"event_type":          in.EventType,
			"activity_code":       in.ActivityCode,
			"activity_generation": in.ActivityGeneration,
			"tenant_id":           in.TenantId,
			"account_id":          in.AccountId,
			"trigger_tenant_id":   in.TriggerTenantId,
			"trigger_account_id":  in.TriggerAccountId,
			"reference_type":      in.ReferenceType,
			"reference_id":        in.ReferenceId,
			"change_days":         in.Days,
			"notify_status":       "pending",
			"remark":              in.Remark,
			"created_at":          now,
			"updated_at":          now,
		}).InsertAndGetId()
		if err != nil {
			if tenantVipDuplicateError(err) {
				return err
			}
			return gerror.Wrap(err, "创建会员变更事件失败")
		}
		result.EventId = eventId

		cols := pdao.YoubanPublishTenantVip.Columns()
		var beforeEntity *entity.YoubanPublishTenantVip
		if err = tx.Model(pdao.YoubanPublishTenantVip.Table()).Safe().Ctx(ctx).
			Where(cols.TenantId, in.TenantId).
			WhereNull(cols.DeletedAt).
			LockUpdate().
			Scan(&beforeEntity); err != nil {
			return gerror.Wrap(err, "锁定租户会员失败")
		}
		before := tenantVipStatusFromEntity(in.TenantId, beforeEntity)
		var currentExpiredAt *gtime.Time
		if beforeEntity != nil {
			currentExpiredAt = beforeEntity.ExpiredAt
		}
		expiredAt := tenantVipExtensionExpiredAt(now, currentExpiredAt, in.Days)
		data := g.Map{
			cols.Level:     in.Level,
			cols.Status:    consts.StatusEnabled,
			cols.OpenedAt:  now,
			cols.ExpiredAt: expiredAt,
			cols.Remark:    in.Remark,
			cols.UpdatedAt: now,
		}
		if beforeEntity == nil || beforeEntity.Id <= 0 {
			data[cols.TenantId] = in.TenantId
			data[cols.CreatedAt] = now
			if _, err = tx.Model(pdao.YoubanPublishTenantVip.Table()).Safe().Ctx(ctx).Data(data).Insert(); err != nil {
				return gerror.Wrap(err, "保存租户会员失败")
			}
		} else if _, err = tx.Model(pdao.YoubanPublishTenantVip.Table()).Safe().Ctx(ctx).Where(cols.Id, beforeEntity.Id).Data(data).Update(); err != nil {
			return gerror.Wrap(err, "更新租户会员失败")
		}
		if err = s.writeTenantVipLogTx(ctx, tx, before, in.TenantId, in.Level, expiredAt, in.EventType, in.Remark); err != nil {
			return err
		}
		_, err = tx.Model(tenantVipEventTable).Safe().Ctx(ctx).
			Where("id", result.EventId).
			Data(g.Map{"before_expired_at": before.ExpiredAt, "after_expired_at": expiredAt, "updated_at": now}).
			Update()
		if err != nil {
			return gerror.Wrap(err, "更新会员变更事件失败")
		}
		result.Applied = true
		result.ExpiredAt = expiredAt
		return nil
	})
	if err != nil {
		if tenantVipDuplicateError(err) {
			return result, nil
		}
		return nil, err
	}
	if result.Applied {
		if cache.Initialized() {
			_, _ = cache.Instance().Remove(ctx, tenantVipCacheKey(in.TenantId))
			_, _ = cache.Instance().Remove(ctx, tenantVipFullCacheKey(in.TenantId))
		}
		if notifyErr := s.notifyTenantVipEvent(ctx, result.EventId); notifyErr != nil {
			g.Log().Warningf(ctx, "会员通知发送失败 eventId:%d err:%+v", result.EventId, notifyErr)
		}
	}
	return result, nil
}

func tenantVipExtensionExpiredAt(now *gtime.Time, currentExpiredAt *gtime.Time, days int) *gtime.Time {
	baseAt := now
	if currentExpiredAt != nil && currentExpiredAt.After(now) {
		baseAt = currentExpiredAt
	}
	return baseAt.AddDate(0, 0, days)
}

func (s *sSysPublish) applyTenantVipUntil(ctx context.Context, tenantId int64, level int, expiredAt *gtime.Time, remark string, eventType string) error {
	if tenantId <= 0 {
		return gerror.New("账号归属不能为空")
	}
	result := &tenantVipChangeResult{}
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		now := gtime.Now()
		eventKey := fmt.Sprintf("%s:%d:%d", eventType, tenantId, now.TimestampNano())
		eventId, err := tx.Model(tenantVipEventTable).Safe().Ctx(ctx).Data(g.Map{
			"event_key":        eventKey,
			"event_type":       eventType,
			"tenant_id":        tenantId,
			"change_days":      0,
			"after_expired_at": expiredAt,
			"notify_status":    "pending",
			"remark":           remark,
			"created_at":       now,
			"updated_at":       now,
		}).InsertAndGetId()
		if err != nil {
			return gerror.Wrap(err, "创建会员调整事件失败")
		}
		result.EventId = eventId

		cols := pdao.YoubanPublishTenantVip.Columns()
		var beforeEntity *entity.YoubanPublishTenantVip
		if err = tx.Model(pdao.YoubanPublishTenantVip.Table()).Safe().Ctx(ctx).
			Where(cols.TenantId, tenantId).
			WhereNull(cols.DeletedAt).
			LockUpdate().
			Scan(&beforeEntity); err != nil {
			return gerror.Wrap(err, "锁定租户会员失败")
		}
		before := tenantVipStatusFromEntity(tenantId, beforeEntity)
		status := consts.StatusDisable
		if level > 0 && expiredAt != nil && expiredAt.After(now) {
			status = consts.StatusEnabled
		}
		data := g.Map{
			cols.Level:     level,
			cols.Status:    status,
			cols.OpenedAt:  now,
			cols.ExpiredAt: expiredAt,
			cols.Remark:    remark,
			cols.UpdatedAt: now,
		}
		if beforeEntity == nil || beforeEntity.Id <= 0 {
			data[cols.TenantId] = tenantId
			data[cols.CreatedAt] = now
			if _, err = tx.Model(pdao.YoubanPublishTenantVip.Table()).Safe().Ctx(ctx).Data(data).Insert(); err != nil {
				return gerror.Wrap(err, "保存租户会员失败")
			}
		} else if _, err = tx.Model(pdao.YoubanPublishTenantVip.Table()).Safe().Ctx(ctx).Where(cols.Id, beforeEntity.Id).Data(data).Update(); err != nil {
			return gerror.Wrap(err, "更新租户会员失败")
		}
		if err = s.writeTenantVipLogTx(ctx, tx, before, tenantId, level, expiredAt, eventType, remark); err != nil {
			return err
		}
		_, err = tx.Model(tenantVipEventTable).Safe().Ctx(ctx).Where("id", result.EventId).Data(g.Map{
			"before_expired_at": before.ExpiredAt,
			"after_expired_at":  expiredAt,
			"updated_at":        now,
		}).Update()
		if err != nil {
			return gerror.Wrap(err, "更新会员调整事件失败")
		}
		result.Applied = true
		result.ExpiredAt = expiredAt
		return nil
	})
	if err != nil {
		return err
	}
	_, _ = cache.Instance().Remove(ctx, tenantVipCacheKey(tenantId))
	_, _ = cache.Instance().Remove(ctx, tenantVipFullCacheKey(tenantId))
	if notifyErr := s.notifyTenantVipEvent(ctx, result.EventId); notifyErr != nil {
		g.Log().Warningf(ctx, "会员调整通知失败 eventId:%d err:%+v", result.EventId, notifyErr)
	}
	return nil
}

type tenantVipEventRow struct {
	Id               int64       `json:"id"`
	EventType        string      `json:"event_type"`
	TenantId         int64       `json:"tenant_id"`
	AccountId        int64       `json:"account_id"`
	TriggerAccountId int64       `json:"trigger_account_id"`
	ChangeDays       int         `json:"change_days"`
	AfterExpiredAt   *gtime.Time `json:"after_expired_at"`
	Remark           string      `json:"remark"`
	NotifyRetryCount int         `json:"notify_retry_count"`
}

func (s *sSysPublish) notifyTenantVipEvent(ctx context.Context, eventId int64) error {
	if eventId <= 0 {
		return nil
	}
	var event *tenantVipEventRow
	if err := g.DB().Model(tenantVipEventTable).Safe().Ctx(ctx).Where("id", eventId).Scan(&event); err != nil {
		return gerror.Wrap(err, "读取会员通知事件失败")
	}
	if event == nil {
		return nil
	}
	if tenantVipExpiryReminderEvent(event.EventType) {
		valid, validateErr := s.tenantVipExpiryReminderValid(ctx, event)
		if validateErr != nil {
			return validateErr
		}
		if !valid {
			_, _ = g.DB().Model(tenantVipEventTable).Safe().Ctx(ctx).Where("id", event.Id).Data(g.Map{
				"notify_status": "skipped",
				"error_message": "会员到期时间已变更或会员已到期",
				"updated_at":    gtime.Now(),
			}).Update()
			return nil
		}
	}
	cfg, err := service.SysConfig().GetYoubanPublishVipActivity(ctx)
	if err != nil {
		return err
	}
	if !tenantVipEventNotifyEnabled(event.EventType, cfg) {
		_, _ = g.DB().Model(tenantVipEventTable).Safe().Ctx(ctx).Where("id", event.Id).Data(g.Map{"notify_status": "skipped", "updated_at": gtime.Now()}).Update()
		return nil
	}
	accountId, err := s.tenantVipNotifyAccountId(ctx, event.TenantId, event.AccountId)
	if err != nil {
		return err
	}
	if accountId <= 0 {
		_, _ = g.DB().Model(tenantVipEventTable).Safe().Ctx(ctx).Where("id", event.Id).Data(g.Map{"notify_status": "skipped", "error_message": "未找到通知账号", "updated_at": gtime.Now()}).Update()
		return nil
	}
	text := tenantVipEventNotifyText(event)
	err = botService.SysBot().NotifyAccount(ctx, &botsysin.NotifyAccountInp{
		BotStrategy:         "official",
		FallbackBoundBot:    true,
		IgnoreFeatureSwitch: true,
		RequireDelivery:     true,
		App:                 consts.AppApi,
		AccountId:           accountId,
		Text:                text,
		ParseMode:           "HTML",
	})
	now := gtime.Now()
	if err == nil {
		_, updateErr := g.DB().Model(tenantVipEventTable).Safe().Ctx(ctx).Where("id", event.Id).Data(g.Map{
			"notify_status":        "sent",
			"notified_at":          now,
			"notify_next_retry_at": nil,
			"error_message":        "",
			"updated_at":           now,
		}).Update()
		return gerror.Wrap(updateErr, "更新会员通知状态失败")
	}
	retryCount := event.NotifyRetryCount + 1
	_, _ = g.DB().Model(tenantVipEventTable).Safe().Ctx(ctx).Where("id", event.Id).Data(g.Map{
		"notify_status":        "failed",
		"notify_retry_count":   retryCount,
		"notify_next_retry_at": now.Add(time.Duration(retryCount) * 5 * time.Minute),
		"error_message":        err.Error(),
		"updated_at":           now,
	}).Update()
	return err
}

func tenantVipEventNotifyEnabled(eventType string, cfg *model.YoubanPublishVipActivityConfig) bool {
	if cfg == nil {
		return false
	}
	switch eventType {
	case tenantVipEventPay:
		return cfg.PayNotifyEnabled
	case tenantVipEventBindGift, tenantVipEventInviteBindGift, tenantVipEventInviteFirstPay:
		return cfg.GiftNotifyEnabled
	case tenantVipEventAdminAdjust:
		return cfg.AdminAdjustNotifyEnabled
	case tenantVipEventExpiringOneDay, tenantVipEventExpiringSixHour, tenantVipEventExpired:
		return cfg.ExpiredNotifyEnabled
	default:
		return false
	}
}

func tenantVipEventNotifyText(event *tenantVipEventRow) string {
	expiredAt := "-"
	if event.AfterExpiredAt != nil {
		expiredAt = event.AfterExpiredAt.Format("Y-m-d H:i:s")
	}
	switch event.EventType {
	case tenantVipEventBindGift:
		return fmt.Sprintf("🎁 <b>Telegram 绑定奖励已到账</b>\n会员时长已增加 %d 天。\n\n<b>到期时间：</b>%s", event.ChangeDays, expiredAt)
	case tenantVipEventInviteBindGift:
		return fmt.Sprintf("🎁 <b>邀请奖励已到账</b>\n好友已完成 Telegram 绑定，会员时长增加 %d 天。\n\n<b>到期时间：</b>%s", event.ChangeDays, expiredAt)
	case tenantVipEventInviteFirstPay:
		return fmt.Sprintf("🎁 <b>邀请开通奖励已到账</b>\n好友首次开通会员，会员时长增加 %d 天。\n\n<b>到期时间：</b>%s", event.ChangeDays, expiredAt)
	case tenantVipEventPay:
		return fmt.Sprintf("✅ <b>VIP 会员已到账</b>\n本次会员时长增加 %d 天，会员权益已生效。\n\n<b>到期时间：</b>%s", event.ChangeDays, expiredAt)
	case tenantVipEventAdminAdjust:
		if event.AfterExpiredAt == nil || !event.AfterExpiredAt.After(gtime.Now()) {
			return "ℹ️ <b>VIP 会员状态已调整</b>\n当前会员已关闭，相关会员功能将暂停使用。"
		}
		return fmt.Sprintf("ℹ️ <b>VIP 会员状态已调整</b>\n会员有效期已更新。\n\n<b>到期时间：</b>%s", expiredAt)
	case tenantVipEventExpiringOneDay:
		return fmt.Sprintf("⏳ <b>VIP 将在 1 天内到期</b>\n为避免会员功能中断，请及时续费。\n\n<b>到期时间：</b>%s", expiredAt)
	case tenantVipEventExpiringSixHour:
		return fmt.Sprintf("⚠️ <b>VIP 将在 6 小时内到期</b>\n会员即将到期，续费后可继续使用全部会员功能。\n\n<b>到期时间：</b>%s", expiredAt)
	case tenantVipEventExpired:
		return fmt.Sprintf("⏰ <b>VIP 会员已到期</b>\n相关会员功能已暂停，续费成功后将自动恢复。\n\n<b>到期时间：</b>%s", expiredAt)
	default:
		return event.Remark
	}
}

func tenantVipExpiryReminderEvent(eventType string) bool {
	return eventType == tenantVipEventExpiringOneDay || eventType == tenantVipEventExpiringSixHour
}

func (s *sSysPublish) tenantVipExpiryReminderValid(ctx context.Context, event *tenantVipEventRow) (bool, error) {
	if event == nil || event.TenantId <= 0 || event.AfterExpiredAt == nil {
		return false, nil
	}
	columns := pdao.YoubanPublishTenantVip.Columns()
	var vip *entity.YoubanPublishTenantVip
	if err := pdao.YoubanPublishTenantVip.Ctx(ctx).
		Where(columns.TenantId, event.TenantId).
		Where(columns.Status, consts.StatusEnabled).
		WhereNull(columns.DeletedAt).
		Scan(&vip); err != nil {
		return false, gerror.Wrap(err, "校验会员到期提醒失败")
	}
	if vip == nil || vip.ExpiredAt == nil || !vip.ExpiredAt.After(gtime.Now()) {
		return false, nil
	}
	return vip.ExpiredAt.Timestamp() == event.AfterExpiredAt.Timestamp(), nil
}

func (s *sSysPublish) tenantVipNotifyAccountId(ctx context.Context, tenantId int64, preferredAccountId int64) (int64, error) {
	if preferredAccountId > 0 {
		return preferredAccountId, nil
	}
	columns := pdao.YoubanPublishAccount.Columns()
	var account *struct {
		Id int64 `json:"id"`
	}
	err := pdao.YoubanPublishAccount.Ctx(ctx).
		Fields(columns.Id).
		Where(columns.TenantId, tenantId).
		Where(columns.AccountType, sysin.PublishAccountTypeAdmin).
		Where(columns.Status, consts.StatusEnabled).
		WhereNull(columns.DeletedAt).
		OrderAsc(columns.Id).
		Scan(&account)
	if err != nil {
		return 0, gerror.Wrap(err, "读取会员通知账号失败")
	}
	if account == nil {
		return 0, nil
	}
	return account.Id, nil
}

func (s *sSysPublish) ProcessTenantVipLifecycle(ctx context.Context, limit int) error {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	if err := s.reconcileTenantVipBindings(ctx, limit); err != nil {
		return err
	}
	if err := s.reconcilePaidTenantVipOrders(ctx, limit); err != nil {
		return err
	}
	if err := s.processTenantVipExpiryReminders(ctx, limit); err != nil {
		return err
	}
	if err := s.processExpiredTenantVips(ctx, limit); err != nil {
		return err
	}
	return s.retryTenantVipNotifications(ctx, limit)
}

type tenantVipExpiryReminder struct {
	EventType string
	Lower     time.Duration
	Upper     time.Duration
}

func tenantVipExpiryReminders() []tenantVipExpiryReminder {
	return []tenantVipExpiryReminder{
		{EventType: tenantVipEventExpiringOneDay, Lower: tenantVipReminderSixHour, Upper: tenantVipReminderOneDay},
		{EventType: tenantVipEventExpiringSixHour, Upper: tenantVipReminderSixHour},
	}
}

func (s *sSysPublish) processTenantVipExpiryReminders(ctx context.Context, limit int) error {
	for _, reminder := range tenantVipExpiryReminders() {
		if err := s.processTenantVipExpiryReminder(ctx, reminder, limit); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) processTenantVipExpiryReminder(ctx context.Context, reminder tenantVipExpiryReminder, limit int) error {
	now := gtime.Now()
	columns := pdao.YoubanPublishTenantVip.Columns()
	joinCondition := fmt.Sprintf("reminder.event_type='%s' AND reminder.tenant_id=vip.tenant_id AND reminder.after_expired_at=vip.expired_at", reminder.EventType)
	model := pdao.YoubanPublishTenantVip.Ctx(ctx).As("vip").
		LeftJoin(tenantVipEventTable+" reminder", joinCondition).
		Fields("vip.*").
		Where("vip."+columns.Status, consts.StatusEnabled).
		WhereGT("vip."+columns.ExpiredAt, now.Add(reminder.Lower)).
		WhereLTE("vip."+columns.ExpiredAt, now.Add(reminder.Upper)).
		WhereNull("vip." + columns.DeletedAt).
		WhereNull("reminder.id").
		OrderAsc("vip." + columns.ExpiredAt).
		Limit(limit)
	var rows []*entity.YoubanPublishTenantVip
	if err := model.Scan(&rows); err != nil {
		return gerror.Wrap(err, "扫描会员到期提醒失败")
	}
	for _, vip := range rows {
		if err := s.createTenantVipExpiryReminder(ctx, vip, reminder.EventType); err != nil {
			g.Log().Warningf(ctx, "创建会员到期提醒失败 tenantId:%d eventType:%s err:%+v", vip.TenantId, reminder.EventType, err)
		}
	}
	return nil
}

func (s *sSysPublish) createTenantVipExpiryReminder(ctx context.Context, vip *entity.YoubanPublishTenantVip, eventType string) error {
	if vip == nil || vip.TenantId <= 0 || vip.ExpiredAt == nil {
		return nil
	}
	now := gtime.Now()
	eventId, err := g.DB().Model(tenantVipEventTable).Safe().Ctx(ctx).Data(g.Map{
		"event_key":        fmt.Sprintf("%s:%d:%d", eventType, vip.TenantId, vip.ExpiredAt.Timestamp()),
		"event_type":       eventType,
		"tenant_id":        vip.TenantId,
		"after_expired_at": vip.ExpiredAt,
		"notify_status":    "pending",
		"remark":           "会员即将到期",
		"created_at":       now,
		"updated_at":       now,
	}).InsertAndGetId()
	if err != nil {
		if tenantVipDuplicateError(err) {
			return nil
		}
		return gerror.Wrap(err, "创建会员到期提醒事件失败")
	}
	if notifyErr := s.notifyTenantVipEvent(ctx, eventId); notifyErr != nil {
		g.Log().Warningf(ctx, "会员到期提醒通知失败 eventId:%d err:%+v", eventId, notifyErr)
	}
	return nil
}

func (s *sSysPublish) reconcileTenantVipBindings(ctx context.Context, limit int) error {
	cfg, err := service.SysConfig().GetYoubanPublishVipActivity(ctx)
	if err != nil {
		return err
	}
	bindEnabledAt := enabledTenantVipActivityTime(cfg.BindGiftEnabled, cfg.BindGiftEnabledAt)
	inviteBindEnabledAt := enabledTenantVipActivityTime(cfg.InviteBindGiftEnabled, cfg.InviteBindGiftEnabledAt)
	if bindEnabledAt == nil && inviteBindEnabledAt == nil {
		return nil
	}
	type bindRow struct {
		AccountId int64       `json:"account_id"`
		BoundAt   *gtime.Time `json:"bound_at"`
	}
	var rows []*bindRow
	accountTable := pdao.YoubanPublishAccount.Table()
	model := g.DB().Model("hg_youban_bot_account_bind b").Safe().Ctx(ctx).
		InnerJoin(accountTable+" a", "a.id=b.account_id AND a.deleted_at IS NULL").
		LeftJoin(tenantVipEventTable+" bind_event", "bind_event.event_type='"+tenantVipEventBindGift+"' AND bind_event.tenant_id=a.tenant_id").
		LeftJoin(botInviteUsageTable+" invite", "invite.used_tenant_id=a.tenant_id AND invite.deleted_at IS NULL").
		LeftJoin(tenantVipEventTable+" invite_event", "invite_event.event_type='"+tenantVipEventInviteBindGift+"' AND invite_event.tenant_id=invite.inviter_tenant_id AND invite_event.trigger_tenant_id=a.tenant_id").
		Fields("b.account_id,COALESCE(b.created_at,b.updated_at) AS bound_at").
		Where("b.app", consts.AppApi).
		Where("b.status", consts.StatusEnabled).
		WhereNull("b.deleted_at")
	conditions := make([]string, 0, 2)
	conditionArgs := make([]interface{}, 0, 2)
	if bindEnabledAt != nil {
		conditions = append(conditions, "(COALESCE(b.created_at,b.updated_at) >= ? AND bind_event.id IS NULL)")
		conditionArgs = append(conditionArgs, bindEnabledAt)
	}
	if inviteBindEnabledAt != nil {
		conditions = append(conditions, "(COALESCE(b.created_at,b.updated_at) >= ? AND invite.id IS NOT NULL AND invite_event.id IS NULL)")
		conditionArgs = append(conditionArgs, inviteBindEnabledAt)
	}
	if err = model.
		Where("("+strings.Join(conditions, " OR ")+")", conditionArgs...).
		OrderAsc("b.id").
		Limit(limit).
		Scan(&rows); err != nil {
		return gerror.Wrap(err, "扫描待补偿TG绑定活动失败")
	}
	for _, row := range rows {
		if err = s.handleTenantVipAccountBoundAt(ctx, row.AccountId, row.BoundAt); err != nil {
			g.Log().Warningf(ctx, "补偿TG绑定奖励失败 accountId:%d err:%+v", row.AccountId, err)
		}
	}
	return nil
}

func enabledTenantVipActivityTime(enabled bool, enabledAtText string) *gtime.Time {
	if !enabled {
		return nil
	}
	return gtime.NewFromStr(strings.TrimSpace(enabledAtText))
}

func (s *sSysPublish) reconcilePaidTenantVipOrders(ctx context.Context, limit int) error {
	cfg, err := service.SysConfig().GetYoubanPublishVipActivity(ctx)
	if err != nil {
		return err
	}
	trackingStartedAt := gtime.NewFromStr(strings.TrimSpace(cfg.EventTrackingStartedAt))
	if trackingStartedAt == nil {
		return nil
	}
	var orders []*baseentity.AdminOrder
	if err = g.DB().Model(dao.AdminOrder.Table()+" orders").Safe().Ctx(ctx).
		LeftJoin(tenantVipEventTable+" pay_event", tenantVipPayEventJoinCondition()).
		Fields("orders.*").
		Where("orders.order_type", tenantVipOrderType).
		Where("orders.status", consts.OrderStatusPay).
		WhereGTE("orders.updated_at", trackingStartedAt).
		WhereNull("pay_event.id").
		OrderAsc("orders.id").
		Limit(limit).
		Scan(&orders); err != nil {
		return gerror.Wrap(err, "扫描待补偿会员订单失败")
	}
	for _, order := range orders {
		if _, err := s.applyTenantVipExtension(ctx, &tenantVipChangeInp{
			EventKey:      fmt.Sprintf("%s:%d", tenantVipEventPay, order.Id),
			EventType:     tenantVipEventPay,
			TenantId:      order.ProductId,
			AccountId:     order.MemberId,
			ReferenceType: "order",
			ReferenceId:   fmt.Sprintf("%d", order.Id),
			Level:         1,
			Days:          30,
			Remark:        order.Remark,
		}); err != nil {
			g.Log().Warningf(ctx, "补偿会员订单失败 orderId:%d err:%+v", order.Id, err)
			continue
		}
		if order.Money <= 0 {
			continue
		}
		if err = s.rewardInviterVip(ctx, order.ProductId, order.MemberId, order.UpdatedAt, cfg); err != nil {
			g.Log().Warningf(ctx, "补偿邀请首付奖励失败 orderId:%d err:%+v", order.Id, err)
		}
	}
	return nil
}

func tenantVipPayEventJoinCondition() string {
	dbType := strings.ToLower(g.DB().GetConfig().Type)
	if dbType == consts.DBPgsql || strings.Contains(dbType, "sqlite") {
		return "pay_event.event_key=('" + tenantVipEventPay + ":' || CAST(orders.id AS TEXT))"
	}
	return "pay_event.event_key=CONCAT('" + tenantVipEventPay + ":', orders.id)"
}

func (s *sSysPublish) processExpiredTenantVips(ctx context.Context, limit int) error {
	columns := pdao.YoubanPublishTenantVip.Columns()
	var rows []*entity.YoubanPublishTenantVip
	if err := pdao.YoubanPublishTenantVip.Ctx(ctx).
		Where(columns.Status, consts.StatusEnabled).
		WhereLTE(columns.ExpiredAt, gtime.Now()).
		WhereNull(columns.DeletedAt).
		OrderAsc(columns.ExpiredAt).
		Limit(limit).
		Scan(&rows); err != nil {
		return gerror.Wrap(err, "扫描到期会员失败")
	}
	for _, vip := range rows {
		if err := s.processExpiredTenantVip(ctx, vip); err != nil {
			g.Log().Warningf(ctx, "处理会员到期失败 tenantId:%d err:%+v", vip.TenantId, err)
		}
	}
	return nil
}

func (s *sSysPublish) processExpiredTenantVip(ctx context.Context, vip *entity.YoubanPublishTenantVip) error {
	if vip == nil || vip.TenantId <= 0 || vip.ExpiredAt == nil {
		return nil
	}
	eventKey := fmt.Sprintf("%s:%d:%d", tenantVipEventExpired, vip.TenantId, vip.ExpiredAt.Timestamp())
	result := &tenantVipChangeResult{}
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		now := gtime.Now()
		_, err := tx.Model(tenantVipEventTable).Safe().Ctx(ctx).Data(g.Map{
			"event_key":        eventKey,
			"event_type":       tenantVipEventExpired,
			"tenant_id":        vip.TenantId,
			"after_expired_at": vip.ExpiredAt,
			"notify_status":    "pending",
			"remark":           "会员到期",
			"created_at":       now,
			"updated_at":       now,
		}).OnConflict("event_key").OnDuplicateEx("id").Save()
		if err != nil {
			return gerror.Wrap(err, "创建会员到期事件失败")
		}
		var event tenantVipEventRow
		if err = tx.Model(tenantVipEventTable).Safe().Ctx(ctx).Where("event_key", eventKey).Scan(&event); err != nil {
			return gerror.Wrap(err, "读取会员到期事件失败")
		}
		result.EventId = event.Id

		columns := pdao.YoubanPublishTenantVip.Columns()
		var locked *entity.YoubanPublishTenantVip
		if err = tx.Model(pdao.YoubanPublishTenantVip.Table()).Safe().Ctx(ctx).
			Where(columns.Id, vip.Id).
			LockUpdate().
			Scan(&locked); err != nil {
			return gerror.Wrap(err, "锁定到期会员失败")
		}
		if locked == nil || locked.Status != consts.StatusEnabled || locked.ExpiredAt == nil || locked.ExpiredAt.After(now) {
			return nil
		}
		before := tenantVipStatusFromEntity(locked.TenantId, locked)
		if _, err = tx.Model(pdao.YoubanPublishTenantVip.Table()).Safe().Ctx(ctx).Where(columns.Id, locked.Id).Data(g.Map{
			columns.Status:    consts.StatusDisable,
			columns.UpdatedAt: now,
		}).Update(); err != nil {
			return gerror.Wrap(err, "更新会员到期状态失败")
		}
		if err = s.writeTenantVipLogTx(ctx, tx, before, locked.TenantId, locked.Level, locked.ExpiredAt, tenantVipEventExpired, "会员到期"); err != nil {
			return err
		}
		result.Applied = true
		result.ExpiredAt = locked.ExpiredAt
		return nil
	})
	if err != nil {
		return err
	}
	if result.Applied {
		_, _ = cache.Instance().Remove(ctx, tenantVipCacheKey(vip.TenantId))
		_, _ = cache.Instance().Remove(ctx, tenantVipFullCacheKey(vip.TenantId))
	}
	if result.EventId > 0 {
		if notifyErr := s.notifyTenantVipEvent(ctx, result.EventId); notifyErr != nil {
			g.Log().Warningf(ctx, "会员到期通知失败 eventId:%d err:%+v", result.EventId, notifyErr)
		}
	}
	return nil
}

func (s *sSysPublish) retryTenantVipNotifications(ctx context.Context, limit int) error {
	var events []*tenantVipEventRow
	if err := g.DB().Model(tenantVipEventTable).Safe().Ctx(ctx).
		WhereIn("notify_status", []string{"pending", "failed"}).
		WhereLT("notify_retry_count", 5).
		Where("notify_next_retry_at IS NULL OR notify_next_retry_at <= ?", gtime.Now()).
		OrderAsc("id").
		Limit(limit).
		Scan(&events); err != nil {
		return gerror.Wrap(err, "扫描会员通知重试任务失败")
	}
	for _, event := range events {
		if err := s.notifyTenantVipEvent(ctx, event.Id); err != nil {
			g.Log().Warningf(ctx, "重试会员通知失败 eventId:%d err:%+v", event.Id, err)
		}
	}
	return nil
}

func tenantVipStatusFromEntity(tenantId int64, vip *entity.YoubanPublishTenantVip) *sysin.TenantVipStatusModel {
	res := &sysin.TenantVipStatusModel{TenantId: tenantId, Status: consts.StatusDisable}
	if vip == nil {
		return res
	}
	res.Level = vip.Level
	res.Status = vip.Status
	res.ExpiredAt = vip.ExpiredAt
	res.IsVip = vip.Status == consts.StatusEnabled && vip.Level > 0 && vip.ExpiredAt != nil && vip.ExpiredAt.After(gtime.Now())
	return res
}

func tenantVipDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate") || strings.Contains(message, "unique")
}
