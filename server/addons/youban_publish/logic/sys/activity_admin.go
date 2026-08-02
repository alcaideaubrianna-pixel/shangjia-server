package sys

import (
	"context"
	"fmt"
	"strings"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	"hotgo/internal/model"
	basesysin "hotgo/internal/model/input/sysin"
	isc "hotgo/internal/service"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const activityGenerationTable = "hg_youban_publish_activity_generation"

type activityAdminHandler struct {
	code        string
	name        string
	description string
	enabledKey  string
	daysKey     string
	enabled     func(*model.YoubanPublishVipActivityConfig) bool
	days        func(*model.YoubanPublishVipActivityConfig) int
	enabledAt   func(*model.YoubanPublishVipActivityConfig) string
}

func activityAdminHandlers() []*activityAdminHandler {
	return []*activityAdminHandler{
		{
			code: tenantVipEventBindGift, name: "绑定 TG 赠会员", description: "用户首次绑定 Telegram 后自动赠送会员。",
			enabledKey: "youbanPublishVipBindGiftEnabled", daysKey: "youbanPublishVipBindGiftDays",
			enabled:   func(cfg *model.YoubanPublishVipActivityConfig) bool { return cfg.BindGiftEnabled },
			days:      func(cfg *model.YoubanPublishVipActivityConfig) int { return cfg.BindGiftDays },
			enabledAt: func(cfg *model.YoubanPublishVipActivityConfig) string { return cfg.BindGiftEnabledAt },
		},
		{
			code: tenantVipEventInviteBindGift, name: "邀请绑定 TG", description: "邀请好友注册并绑定 Telegram 后赠送会员。",
			enabledKey: "youbanPublishVipInviteBindGiftEnabled", daysKey: "youbanPublishVipInviteBindGiftDays",
			enabled:   func(cfg *model.YoubanPublishVipActivityConfig) bool { return cfg.InviteBindGiftEnabled },
			days:      func(cfg *model.YoubanPublishVipActivityConfig) int { return cfg.InviteBindGiftDays },
			enabledAt: func(cfg *model.YoubanPublishVipActivityConfig) string { return cfg.InviteBindGiftEnabledAt },
		},
		{
			code: tenantVipEventInviteFirstPay, name: "邀请好友首付", description: "邀请好友首次真实付费开通会员后赠送会员。",
			enabledKey: "youbanPublishVipInviteFirstPayGiftEnabled", daysKey: "youbanPublishVipInviteFirstPayGiftDays",
			enabled:   func(cfg *model.YoubanPublishVipActivityConfig) bool { return cfg.InviteFirstPayEnabled },
			days:      func(cfg *model.YoubanPublishVipActivityConfig) int { return cfg.InviteFirstPayDays },
			enabledAt: func(cfg *model.YoubanPublishVipActivityConfig) string { return cfg.InviteFirstPayEnabledAt },
		},
	}
}

func activityAdminHandlerByCode(code string) *activityAdminHandler {
	code = strings.TrimSpace(code)
	for _, handler := range activityAdminHandlers() {
		if handler.code == code {
			return handler
		}
	}
	return nil
}

func (s *sSysPublish) AdminActivityList(ctx context.Context) ([]*sysin.ActivityModel, error) {
	if err := ensureTenantVipTables(ctx); err != nil {
		return nil, err
	}
	cfg, err := isc.SysConfig().GetYoubanPublishVipActivity(ctx)
	if err != nil {
		return nil, err
	}
	type summaryRow struct {
		EventType    string      `json:"event_type"`
		Count        int         `json:"count"`
		Days         int         `json:"days"`
		LastRewardAt *gtime.Time `json:"last_reward_at"`
	}
	var rows []*summaryRow
	if err = g.DB().Model(tenantVipEventTable).Safe().Ctx(ctx).
		Fields("event_type,COUNT(*) AS count,COALESCE(SUM(change_days),0) AS days,MAX(created_at) AS last_reward_at").
		WhereIn("event_type", []string{tenantVipEventBindGift, tenantVipEventInviteBindGift, tenantVipEventInviteFirstPay}).
		Group("event_type").Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取活动奖励统计失败")
	}
	summaries := make(map[string]*summaryRow, len(rows))
	for _, row := range rows {
		summaries[row.EventType] = row
	}
	list := make([]*sysin.ActivityModel, 0, len(activityAdminHandlers()))
	for _, handler := range activityAdminHandlers() {
		item := &sysin.ActivityModel{
			Code: handler.code, Name: handler.name, Description: handler.description,
			Enabled: handler.enabled(cfg), RewardDays: handler.days(cfg), EnabledAt: handler.enabledAt(cfg),
		}
		if summary := summaries[handler.code]; summary != nil {
			item.RewardCount = summary.Count
			item.RewardDaysTotal = summary.Days
			item.LastRewardAt = summary.LastRewardAt
		}
		list = append(list, item)
	}
	return list, nil
}

func (s *sSysPublish) AdminActivitySave(ctx context.Context, in *sysin.ActivitySaveInp) error {
	if in == nil {
		return gerror.New("活动配置不能为空")
	}
	handler := activityAdminHandlerByCode(in.Code)
	if handler == nil {
		return gerror.New("活动不存在")
	}
	if in.RewardDays <= 0 || in.RewardDays > 3650 {
		return gerror.New("奖励天数必须在1到3650之间")
	}
	return isc.SysConfig().UpdateConfigByGroup(ctx, &basesysin.UpdateConfigInp{
		Group: "youban_publish_vip_activity",
		List:  g.Map{handler.enabledKey: in.Enabled, handler.daysKey: in.RewardDays},
	})
}

func (s *sSysPublish) AdminActivityRewardList(ctx context.Context, in *sysin.ActivityRewardListInp) (list []*sysin.ActivityRewardModel, totalCount int, err error) {
	if err = ensureTenantVipTables(ctx); err != nil {
		return
	}
	if in == nil {
		in = &sysin.ActivityRewardListInp{}
	}
	activityCode := "COALESCE(NULLIF(e.activity_code,''),e.event_type)"
	mod := g.DB().Model(tenantVipEventTable+" e").Safe().Ctx(ctx).
		LeftJoin(pdao.YoubanPublishTenant.Table()+" t", "t.id=e.tenant_id AND t.deleted_at IS NULL").
		LeftJoin(pdao.YoubanPublishAccount.Table()+" a", "a.id=e.account_id AND a.deleted_at IS NULL").
		Fields("e.id,"+activityCode+" AS activity_code,e.activity_generation,e.tenant_id,COALESCE(t.name,'') AS tenant_name,e.account_id,COALESCE(a.username,'') AS account_username,e.trigger_tenant_id,e.trigger_account_id,e.change_days,e.after_expired_at,e.notify_status,e.notify_retry_count,e.error_message,e.remark,e.created_at").
		WhereIn("e.event_type", []string{tenantVipEventBindGift, tenantVipEventInviteBindGift, tenantVipEventInviteFirstPay})
	if code := strings.TrimSpace(in.ActivityCode); code != "" {
		if activityAdminHandlerByCode(code) == nil {
			return nil, 0, gerror.New("活动不存在")
		}
		mod = mod.Where(activityCode+"=?", code)
	}
	if in.TenantId > 0 {
		mod = mod.Where("e.tenant_id", in.TenantId)
	}
	if status := strings.TrimSpace(in.NotifyStatus); status != "" {
		mod = mod.Where("e.notify_status", status)
	}
	if keyword := strings.TrimSpace(in.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		mod = mod.Where("(t.name LIKE ? OR a.username LIKE ?)", like, like)
	}
	if err = mod.Page(in.Page, in.PerPage).OrderDesc("e.id").ScanAndCount(&list, &totalCount, false); err != nil {
		return nil, 0, gerror.Wrap(err, "读取活动奖励记录失败")
	}
	for _, item := range list {
		if handler := activityAdminHandlerByCode(item.ActivityCode); handler != nil {
			item.ActivityName = handler.name
		}
		if item.ActivityGeneration <= 0 {
			item.ActivityGeneration = 1
		}
	}
	return
}

func (s *sSysPublish) AdminActivityUserStatus(ctx context.Context, in *sysin.ActivityUserStatusInp) ([]*sysin.ActivityUserStatusModel, error) {
	if in == nil || in.TenantId <= 0 {
		return nil, gerror.New("请选择账号归属")
	}
	cfg, err := isc.SysConfig().GetYoubanPublishVipActivity(ctx)
	if err != nil {
		return nil, err
	}
	list := make([]*sysin.ActivityUserStatusModel, 0, len(activityAdminHandlers()))
	for _, handler := range activityAdminHandlers() {
		status, statusErr := s.activityUserStatus(ctx, handler, cfg, in.TenantId)
		if statusErr != nil {
			return nil, statusErr
		}
		list = append(list, status)
	}
	return list, nil
}

func (s *sSysPublish) AdminActivityDebug(ctx context.Context, in *sysin.ActivityDebugInp) (*sysin.ActivityUserStatusModel, error) {
	if in == nil || in.TenantId <= 0 {
		return nil, gerror.New("请选择账号归属")
	}
	handler := activityAdminHandlerByCode(in.Code)
	if handler == nil {
		return nil, gerror.New("活动不存在")
	}
	cfg, err := isc.SysConfig().GetYoubanPublishVipActivity(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Action) == "retry" {
		if err = s.retryActivityForTenant(ctx, handler, cfg, in.TenantId); err != nil {
			return nil, err
		}
	}
	return s.activityUserStatus(ctx, handler, cfg, in.TenantId)
}

func (s *sSysPublish) AdminActivityReset(ctx context.Context, in *sysin.ActivityResetInp) error {
	if in == nil || in.TenantId <= 0 {
		return gerror.New("请选择账号归属")
	}
	if activityAdminHandlerByCode(in.Code) == nil {
		return gerror.New("活动不存在")
	}
	if strings.TrimSpace(in.Reason) == "" {
		return gerror.New("请填写重置原因")
	}
	if err := ensureTenantVipTables(ctx); err != nil {
		return err
	}
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		now := gtime.Now()
		var row struct {
			Id         int64 `json:"id"`
			Generation int   `json:"generation"`
		}
		if err := tx.Model(activityGenerationTable).Ctx(ctx).
			Where("activity_code", in.Code).Where("tenant_id", in.TenantId).LockUpdate().Scan(&row); err != nil {
			return gerror.Wrap(err, "锁定用户活动失败")
		}
		data := g.Map{
			"generation":   row.Generation + 1,
			"reset_reason": strings.TrimSpace(in.Reason),
			"updated_by":   contexts.GetUserId(ctx),
			"updated_at":   now,
		}
		if row.Id > 0 {
			_, err := tx.Model(activityGenerationTable).Ctx(ctx).Where("id", row.Id).Data(data).Update()
			return gerror.Wrap(err, "重置用户活动失败")
		}
		data["activity_code"] = in.Code
		data["tenant_id"] = in.TenantId
		data["generation"] = 2
		data["created_at"] = now
		_, err := tx.Model(activityGenerationTable).Ctx(ctx).Data(data).Insert()
		return gerror.Wrap(err, "重置用户活动失败")
	})
}

func (s *sSysPublish) tenantVipActivityEventIdentity(ctx context.Context, code string, tenantId int64, baseKey string) (string, int, error) {
	generation, err := s.tenantVipActivityGeneration(ctx, code, tenantId)
	if err != nil {
		return "", 0, err
	}
	return tenantVipActivityEventKey(baseKey, generation), generation, nil
}

func tenantVipActivityEventKey(baseKey string, generation int) string {
	if generation <= 1 {
		return baseKey
	}
	return fmt.Sprintf("%s:g%d", baseKey, generation)
}

func (s *sSysPublish) tenantVipActivityGeneration(ctx context.Context, code string, tenantId int64) (int, error) {
	if err := ensureTenantVipTables(ctx); err != nil {
		return 0, err
	}
	value, err := g.DB().Model(activityGenerationTable).Safe().Ctx(ctx).
		Fields("generation").Where("activity_code", code).Where("tenant_id", tenantId).Value()
	if err != nil {
		return 0, gerror.Wrap(err, "读取用户活动代次失败")
	}
	if value.IsNil() || value.Int() <= 0 {
		return 1, nil
	}
	return value.Int(), nil
}

func (s *sSysPublish) activityUserStatus(ctx context.Context, handler *activityAdminHandler, cfg *model.YoubanPublishVipActivityConfig, tenantId int64) (*sysin.ActivityUserStatusModel, error) {
	generation, err := s.tenantVipActivityGeneration(ctx, handler.code, tenantId)
	if err != nil {
		return nil, err
	}
	type rewardSummary struct {
		Count        int         `json:"count"`
		Days         int         `json:"days"`
		LastRewardAt *gtime.Time `json:"last_reward_at"`
	}
	var reward rewardSummary
	mod := g.DB().Model(tenantVipEventTable).Safe().Ctx(ctx).
		Fields("COUNT(*) AS count,COALESCE(SUM(change_days),0) AS days,MAX(created_at) AS last_reward_at").
		Where("tenant_id", tenantId).Where("event_type", handler.code)
	if generation <= 1 {
		mod = mod.Where("COALESCE(activity_generation,1)=?", 1)
	} else {
		mod = mod.Where("activity_generation", generation)
	}
	if err = mod.Scan(&reward); err != nil {
		return nil, gerror.Wrap(err, "读取用户活动奖励失败")
	}
	eligibleCount, err := s.activityEligibleCount(ctx, handler, cfg, tenantId)
	if err != nil {
		return nil, err
	}
	res := &sysin.ActivityUserStatusModel{
		Code: handler.code, Name: handler.name, Enabled: handler.enabled(cfg), Generation: generation,
		EligibleCount: eligibleCount, RewardCount: reward.Count, RewardDays: reward.Days, LastRewardAt: reward.LastRewardAt,
		Status: "pending", Reason: "尚未满足活动条件",
	}
	if !res.Enabled {
		res.Status, res.Reason = "disabled", "活动当前已关闭"
	} else if reward.Count > 0 {
		res.Status, res.Reason = "rewarded", "当前代次已发放奖励"
	} else if eligibleCount > 0 {
		res.Status, res.Reason = "eligible", "已满足条件，可执行奖励重试"
	}
	return res, nil
}

func (s *sSysPublish) activityEligibleCount(ctx context.Context, handler *activityAdminHandler, cfg *model.YoubanPublishVipActivityConfig, tenantId int64) (int, error) {
	enabledAt := gtime.NewFromStr(strings.TrimSpace(handler.enabledAt(cfg)))
	switch handler.code {
	case tenantVipEventBindGift:
		mod := g.DB().Model("hg_youban_bot_account_bind b").Safe().Ctx(ctx).
			InnerJoin(pdao.YoubanPublishAccount.Table()+" a", "a.id=b.account_id AND a.deleted_at IS NULL").
			Where("a.tenant_id", tenantId).Where("b.app", consts.AppApi).Where("b.status", consts.StatusEnabled).WhereNull("b.deleted_at")
		if enabledAt != nil {
			mod = mod.Where("COALESCE(b.created_at,b.updated_at)>=?", enabledAt)
		}
		return mod.Count()
	case tenantVipEventInviteBindGift:
		mod := g.DB().Model(webInviteTable+" i").Safe().Ctx(ctx).
			InnerJoin(pdao.YoubanPublishAccount.Table()+" a", "a.tenant_id=i.used_tenant_id AND a.account_type='admin' AND a.deleted_at IS NULL").
			InnerJoin("hg_youban_bot_account_bind b", "b.account_id=a.id AND b.app='api' AND b.status=1 AND b.deleted_at IS NULL").
			Where("i.inviter_tenant_id", tenantId).Where("i.status", webInviteStatusUsed).WhereNull("i.deleted_at")
		if enabledAt != nil {
			mod = mod.Where("COALESCE(b.created_at,b.updated_at)>=?", enabledAt)
		}
		return mod.Count()
	case tenantVipEventInviteFirstPay:
		mod := g.DB().Model(webInviteTable+" i").Safe().Ctx(ctx).
			InnerJoin(dao.AdminOrder.Table()+" o", "o.product_id=i.used_tenant_id AND o.order_type='"+tenantVipOrderType+"' AND o.status="+fmt.Sprint(consts.OrderStatusPay)+" AND o.money>0").
			Where("i.inviter_tenant_id", tenantId).Where("i.status", webInviteStatusUsed).WhereNull("i.deleted_at")
		if enabledAt != nil {
			mod = mod.WhereGTE("o.updated_at", enabledAt)
		}
		return mod.Count()
	default:
		return 0, nil
	}
}

func (s *sSysPublish) retryActivityForTenant(ctx context.Context, handler *activityAdminHandler, cfg *model.YoubanPublishVipActivityConfig, tenantId int64) error {
	if !handler.enabled(cfg) {
		return gerror.New("活动当前已关闭")
	}
	switch handler.code {
	case tenantVipEventBindGift:
		var rows []struct {
			AccountId int64       `json:"account_id"`
			BoundAt   *gtime.Time `json:"bound_at"`
		}
		if err := g.DB().Model("hg_youban_bot_account_bind b").Safe().Ctx(ctx).
			InnerJoin(pdao.YoubanPublishAccount.Table()+" a", "a.id=b.account_id AND a.deleted_at IS NULL").
			Fields("b.account_id,COALESCE(b.created_at,b.updated_at) AS bound_at").Where("a.tenant_id", tenantId).
			Where("b.app", consts.AppApi).Where("b.status", consts.StatusEnabled).WhereNull("b.deleted_at").Scan(&rows); err != nil {
			return gerror.Wrap(err, "读取TG绑定失败")
		}
		for _, row := range rows {
			account, err := s.tenantVipAccountById(ctx, row.AccountId)
			if err != nil {
				return err
			}
			if err = s.applyTenantVipBindGift(ctx, account, row.BoundAt, cfg); err != nil {
				return err
			}
		}
	case tenantVipEventInviteBindGift:
		var rows []struct {
			AccountId int64       `json:"account_id"`
			BoundAt   *gtime.Time `json:"bound_at"`
		}
		if err := g.DB().Model(webInviteTable+" i").Safe().Ctx(ctx).
			InnerJoin(pdao.YoubanPublishAccount.Table()+" a", "a.tenant_id=i.used_tenant_id AND a.account_type='admin' AND a.deleted_at IS NULL").
			InnerJoin("hg_youban_bot_account_bind b", "b.account_id=a.id AND b.app='api' AND b.status=1 AND b.deleted_at IS NULL").
			Fields("a.id AS account_id,COALESCE(b.created_at,b.updated_at) AS bound_at").Where("i.inviter_tenant_id", tenantId).
			Where("i.status", webInviteStatusUsed).WhereNull("i.deleted_at").Scan(&rows); err != nil {
			return gerror.Wrap(err, "读取邀请绑定记录失败")
		}
		for _, row := range rows {
			account, err := s.tenantVipAccountById(ctx, row.AccountId)
			if err != nil {
				return err
			}
			if err = s.applyTenantVipInviteBindGift(ctx, account, row.BoundAt, cfg); err != nil {
				return err
			}
		}
	case tenantVipEventInviteFirstPay:
		var rows []struct {
			TenantId  int64       `json:"tenant_id"`
			AccountId int64       `json:"account_id"`
			PaidAt    *gtime.Time `json:"paid_at"`
		}
		if err := g.DB().Model(webInviteTable+" i").Safe().Ctx(ctx).
			InnerJoin(dao.AdminOrder.Table()+" o", "o.product_id=i.used_tenant_id AND o.order_type='"+tenantVipOrderType+"' AND o.status="+fmt.Sprint(consts.OrderStatusPay)+" AND o.money>0").
			Fields("o.product_id AS tenant_id,o.member_id AS account_id,o.updated_at AS paid_at").
			Where("i.inviter_tenant_id", tenantId).Where("i.status", webInviteStatusUsed).WhereNull("i.deleted_at").Scan(&rows); err != nil {
			return gerror.Wrap(err, "读取邀请付费记录失败")
		}
		for _, row := range rows {
			if err := s.rewardInviterVip(ctx, row.TenantId, row.AccountId, row.PaidAt, cfg); err != nil {
				return err
			}
		}
	}
	return nil
}
