package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/internal/model/entity"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
	"hotgo/internal/library/contexts"
	"hotgo/internal/library/payment"
	"hotgo/internal/model"
	baseentity "hotgo/internal/model/entity"
	"hotgo/internal/model/input/payin"
	"hotgo/internal/service"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const tenantVipCacheTTL = 5 * time.Minute
const tenantVipOrderGroup = "youban_tenant_vip"
const tenantVipOrderType = "youban_tenant_vip"
const tenantVipPlanMonth = "vip_month"

func (s *sSysPublish) TenantVipStatus(ctx context.Context) (*sysin.TenantVipStatusModel, error) {
	account, err := s.currentAccount(ctx)
	if err != nil {
		return nil, err
	}
	return s.tenantVipStatus(ctx, account.TenantId)
}

func (s *sSysPublish) TenantVipPlans(ctx context.Context) ([]*sysin.TenantVipPlanModel, error) {
	cfg, err := service.SysConfig().GetYoubanPublishVip(ctx)
	if err != nil {
		return nil, err
	}
	return []*sysin.TenantVipPlanModel{
		{Code: "free", Name: "免费计划", Level: 0, Days: 0, Price: 0, Currency: tenantVipCurrency(cfg), Description: "适合日常上架资料管理", Features: tenantVipFreeFeatures()},
		tenantVipPlanByConfig(cfg),
	}, nil
}

func (s *sSysPublish) TenantVipOrderCreate(ctx context.Context, in *sysin.TenantVipOrderCreateInp) (*sysin.TenantVipOrderModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := service.SysConfig().GetYoubanPublishVip(ctx)
	if err != nil {
		return nil, err
	}
	if cfg == nil || !cfg.Enabled {
		return nil, gerror.New("上架VIP暂未开放")
	}
	plan := tenantVipPlanByCode(strings.TrimSpace(in.PlanCode), cfg)
	if plan == nil || plan.Level <= 0 {
		return nil, gerror.New("请选择有效的会员套餐")
	}
	amount, couponCode, couponDiscount, err := s.tenantVipOrderAmountWithCoupon(ctx, plan.Price, in.CouponCode, cfg)
	if err != nil {
		return nil, err
	}
	if in.PayType == "" || in.TradeType == "" {
		payItem := tenantVipDefaultPayItem(cfg)
		if payItem != nil {
			if in.PayType == "" {
				in.PayType = payItem.PayType
			}
			if in.TradeType == "" {
				in.TradeType = payItem.TradeType
			}
		}
	}
	if strings.TrimSpace(in.PayType) == "" {
		in.PayType = consts.PayTypeRainbow
	}
	orderSn := fmt.Sprintf("YBPVIP%d%d", account.TenantId, time.Now().UnixNano())
	subject := fmt.Sprintf("上架系统VIP会员:%d天", plan.Days)
	now := gtime.Now()
	var res *sysin.TenantVipOrderModel
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		result, err := dao.AdminOrder.Ctx(ctx).Data(baseentity.AdminOrder{
			MemberId:  contexts.GetUserId(ctx),
			OrderType: tenantVipOrderType,
			ProductId: account.TenantId,
			OrderSn:   orderSn,
			Money:     amount,
			Remark:    subject,
			Status:    consts.OrderStatusNotPay,
			CreatedAt: now,
			UpdatedAt: now,
		}).OmitEmptyData().Insert()
		if err != nil {
			return err
		}
		orderId, _ := result.LastInsertId()
		create, err := service.Pay().Create(ctx, payin.PayCreateInp{
			Subject:    subject,
			Detail:     gjson.New(g.Map{"couponCode": couponCode, "couponDiscount": couponDiscount, "days": plan.Days, "level": plan.Level, "planCode": plan.Code, "tenantId": account.TenantId}),
			OrderSn:    orderSn,
			OrderGroup: tenantVipOrderGroup,
			PayType:    in.PayType,
			TradeType:  in.TradeType,
			PayAmount:  amount,
			ReturnUrl:  in.ReturnUrl,
		})
		if err != nil {
			return err
		}
		res = tenantVipOrderModel(&baseentity.AdminOrder{Id: orderId, OrderSn: orderSn, Money: amount, Status: consts.OrderStatusNotPay, CreatedAt: now}, nil, plan)
		res.Order = create.Order
		if create.Order != nil {
			res.PayUrl = create.Order.PayURL
			res.TradeType = create.Order.TradeType
		}
		if amount <= 0 {
			if err = s.openTenantVip(ctx, account.TenantId, plan.Level, plan.Days, subject, "free"); err != nil {
				return err
			}
			if err = s.markTenantVipCouponUsed(ctx, gjson.New(g.Map{
				"couponCode": couponCode,
			})); err != nil {
				return err
			}
			res.Status = consts.OrderStatusPay
			res.StatusTxt = tenantVipOrderStatusText(consts.OrderStatusPay)
			res.PaidAt = now
		}
		return nil
	})
	return res, err
}

func (s *sSysPublish) TenantVipOrderList(ctx context.Context, in *sysin.TenantVipOrderListInp) (list []*sysin.TenantVipOrderModel, totalCount int, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, 0, err
	}
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.PerPage <= 0 || in.PerPage > 50 {
		in.PerPage = 10
	}
	orderCols := dao.AdminOrder.Columns()
	mod := dao.AdminOrder.Ctx(ctx).
		Where(orderCols.OrderType, tenantVipOrderType).
		Where(orderCols.ProductId, account.TenantId)
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "读取会员订单数量失败")
	}
	var orders []*baseentity.AdminOrder
	if err = mod.Page(in.Page, in.PerPage).OrderDesc(orderCols.Id).Scan(&orders); err != nil {
		return nil, 0, gerror.Wrap(err, "读取会员订单列表失败")
	}
	orderNos := make([]string, 0, len(orders))
	for _, item := range orders {
		orderNos = append(orderNos, item.OrderSn)
	}
	payMap := map[string]*baseentity.PayLog{}
	if len(orderNos) > 0 {
		var pays []*baseentity.PayLog
		if err = dao.PayLog.Ctx(ctx).WhereIn(dao.PayLog.Columns().OrderSn, orderNos).Scan(&pays); err != nil {
			return nil, 0, gerror.Wrap(err, "读取会员支付记录失败")
		}
		for _, item := range pays {
			payMap[item.OrderSn] = item
		}
	}
	cfg, err := service.SysConfig().GetYoubanPublishVip(ctx)
	if err != nil {
		return nil, 0, err
	}
	plan := tenantVipPlanByCode(tenantVipPlanMonth, cfg)
	list = make([]*sysin.TenantVipOrderModel, 0, len(orders))
	for _, item := range orders {
		list = append(list, tenantVipOrderModel(item, payMap[item.OrderSn], plan))
	}
	return list, totalCount, nil
}

func (s *sSysPublish) TenantVipOrderPay(ctx context.Context, in *sysin.TenantVipOrderPayInp) (*sysin.TenantVipOrderModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in.Id <= 0 {
		return nil, gerror.New("订单ID不能为空")
	}
	var order *baseentity.AdminOrder
	orderCols := dao.AdminOrder.Columns()
	if err = dao.AdminOrder.Ctx(ctx).
		Where(orderCols.Id, in.Id).
		Where(orderCols.OrderType, tenantVipOrderType).
		Where(orderCols.ProductId, account.TenantId).
		Scan(&order); err != nil {
		return nil, gerror.Wrap(err, "读取会员订单失败")
	}
	if order == nil {
		return nil, gerror.New("会员订单不存在")
	}
	if order.Status != consts.OrderStatusNotPay {
		return nil, gerror.New("当前订单不可支付")
	}
	var pay *baseentity.PayLog
	payCols := dao.PayLog.Columns()
	if err = dao.PayLog.Ctx(ctx).Where(payCols.OrderSn, order.OrderSn).Scan(&pay); err != nil {
		return nil, gerror.Wrap(err, "读取会员支付记录失败")
	}
	if pay == nil {
		return nil, gerror.New("会员支付记录不存在")
	}
	if strings.TrimSpace(in.ReturnUrl) != "" {
		pay.ReturnUrl = strings.TrimSpace(in.ReturnUrl)
	}
	payOrder, err := payment.New(pay.PayType).CreateOrder(ctx, payin.CreateOrderInp{Pay: pay})
	if err != nil {
		return nil, err
	}
	cfg, err := service.SysConfig().GetYoubanPublishVip(ctx)
	if err != nil {
		return nil, err
	}
	res := tenantVipOrderModel(order, pay, tenantVipPlanByCode(tenantVipPlanMonth, cfg))
	res.Order = payOrder
	if payOrder != nil {
		res.PayUrl = payOrder.PayURL
		res.TradeType = payOrder.TradeType
	}
	return res, nil
}

func (s *sSysPublish) TenantVipPayNotify(ctx context.Context, in *payin.NotifyCallFuncInp) error {
	if in == nil || in.Pay == nil {
		return gerror.New("支付回调参数为空")
	}
	var order *baseentity.AdminOrder
	cols := dao.AdminOrder.Columns()
	if err := dao.AdminOrder.Ctx(ctx).Where(cols.OrderSn, in.Pay.OrderSn).Scan(&order); err != nil {
		return err
	}
	if order == nil {
		return gerror.Newf("会员订单[%s]不存在", in.Pay.OrderSn)
	}
	if order.Status != consts.OrderStatusNotPay {
		return nil
	}
	shouldReward, err := s.shouldRewardInviterVip(ctx, order.ProductId)
	if err != nil {
		return err
	}
	cfg, err := service.SysConfig().GetYoubanPublishVip(ctx)
	if err != nil {
		return err
	}
	plan := tenantVipPlanByCode(tenantVipPlanMonth, cfg)
	_, err = dao.AdminOrder.Ctx(ctx).
		Where(cols.Id, order.Id).
		Where(cols.Status, consts.OrderStatusNotPay).
		Data(g.Map{cols.Status: consts.OrderStatusPay, cols.UpdatedAt: gtime.Now()}).
		Update()
	if err != nil {
		return err
	}
	if err = s.openTenantVip(ctx, order.ProductId, plan.Level, plan.Days, order.Remark, "pay"); err != nil {
		return err
	}
	if err = s.markTenantVipCouponUsed(ctx, in.Pay.Detail); err != nil {
		return err
	}
	if !shouldReward {
		return nil
	}
	return s.rewardInviterVip(ctx, order.ProductId, cfg)
}

func (s *sSysPublish) ensureTenantVipFeature(ctx context.Context, tenantId int64, featureCode string) error {
	status, err := s.tenantVipStatus(ctx, tenantId)
	if err != nil {
		return err
	}
	if !status.IsVip || !containsString(status.Features, featureCode) {
		return gerror.New("当前功能需要开通VIP会员")
	}
	return nil
}

func (s *sSysPublish) tenantVipStatus(ctx context.Context, tenantId int64) (*sysin.TenantVipStatusModel, error) {
	if tenantId <= 0 {
		return nil, gerror.New("租户ID不能为空")
	}
	if err := ensureTenantVipTables(ctx); err != nil {
		return nil, err
	}
	cacheKey := tenantVipCacheKey(tenantId)
	if value, err := cache.Instance().Get(ctx, cacheKey); err == nil && !value.IsNil() {
		var cached sysin.TenantVipStatusModel
		if scanErr := value.Scan(&cached); scanErr == nil && cached.TenantId > 0 {
			return &cached, nil
		}
	}
	status, err := s.loadTenantVipStatus(ctx, tenantId)
	if err != nil {
		return nil, err
	}
	_ = cache.Instance().Set(ctx, cacheKey, status, tenantVipCacheTTL)
	return status, nil
}

func (s *sSysPublish) loadTenantVipStatus(ctx context.Context, tenantId int64) (*sysin.TenantVipStatusModel, error) {
	cols := pdao.YoubanPublishTenantVip.Columns()
	var vip *entity.YoubanPublishTenantVip
	if err := pdao.YoubanPublishTenantVip.Ctx(ctx).
		Where(cols.TenantId, tenantId).
		WhereNull(cols.DeletedAt).
		Scan(&vip); err != nil {
		return nil, gerror.Wrap(err, "读取租户会员失败")
	}
	res := &sysin.TenantVipStatusModel{TenantId: tenantId, Status: consts.StatusDisable}
	if vip == nil {
		return res, nil
	}
	res.Level = vip.Level
	res.Status = vip.Status
	res.ExpiredAt = vip.ExpiredAt
	res.IsVip = vip.Status == consts.StatusEnabled && vip.Level > 0 && (vip.ExpiredAt == nil || vip.ExpiredAt.After(gtime.Now()))
	if res.IsVip {
		res.Features = []string{sysin.TenantVipFeatureSimilarMedia}
	}
	return res, nil
}

func (s *sSysPublish) openTenantVip(ctx context.Context, tenantId int64, level int, days int, remark string, source string) error {
	if tenantId <= 0 || level <= 0 || days <= 0 {
		return gerror.New("会员开通参数不完整")
	}
	before, err := s.tenantVipStatus(ctx, tenantId)
	if err != nil {
		return err
	}
	expiredAt := gtime.Now().AddDate(0, 0, days)
	now := gtime.Now()
	data := do.YoubanPublishTenantVip{
		TenantId:  tenantId,
		Level:     level,
		Status:    consts.StatusEnabled,
		OpenedAt:  now,
		ExpiredAt: expiredAt,
		Remark:    remark,
		UpdatedAt: now,
	}
	cols := pdao.YoubanPublishTenantVip.Columns()
	count, err := pdao.YoubanPublishTenantVip.Ctx(ctx).Where(cols.TenantId, tenantId).Count()
	if err != nil {
		return gerror.Wrap(err, "检查租户会员失败")
	}
	if count > 0 {
		if _, err = pdao.YoubanPublishTenantVip.Ctx(ctx).Where(cols.TenantId, tenantId).Data(data).Update(); err != nil {
			return gerror.Wrap(err, "更新租户会员失败")
		}
	} else {
		data.CreatedAt = now
		if _, err = pdao.YoubanPublishTenantVip.Ctx(ctx).Data(data).Insert(); err != nil {
			return gerror.Wrap(err, "保存租户会员失败")
		}
	}
	_, _ = cache.Instance().Remove(ctx, tenantVipCacheKey(tenantId))
	return s.writeTenantVipLog(ctx, before, tenantId, level, expiredAt, source, remark)
}

func (s *sSysPublish) writeTenantVipLog(ctx context.Context, before *sysin.TenantVipStatusModel, tenantId int64, level int, expiredAt *gtime.Time, source string, remark string) error {
	if before == nil {
		before = &sysin.TenantVipStatusModel{}
	}
	afterStatus := consts.StatusDisable
	if level > 0 && expiredAt != nil && expiredAt.After(gtime.Now()) {
		afterStatus = consts.StatusEnabled
	}
	action := "open"
	if afterStatus != consts.StatusEnabled {
		action = "cancel"
	} else if before.IsVip {
		action = "adjust"
	}
	_, err := pdao.YoubanPublishTenantVipLog.Ctx(ctx).Data(do.YoubanPublishTenantVipLog{
		TenantId:        tenantId,
		OperatorId:      contexts.GetUserId(ctx),
		Source:          source,
		Action:          action,
		BeforeStatus:    before.Status,
		BeforeLevel:     before.Level,
		BeforeExpiredAt: before.ExpiredAt,
		AfterStatus:     afterStatus,
		AfterLevel:      level,
		AfterExpiredAt:  expiredAt,
		Remark:          remark,
		CreatedAt:       gtime.Now(),
	}).Insert()
	return gerror.Wrap(err, "写入租户会员日志失败")
}

func (s *sSysPublish) rewardInviterVip(ctx context.Context, paidTenantId int64, cfg *model.YoubanPublishVipConfig) error {
	if cfg == nil || !cfg.InviteRewardEnabled || cfg.InviteRewardDays <= 0 || paidTenantId <= 0 {
		return nil
	}
	var row *webInviteRow
	if err := g.DB().Model(webInviteTable).Safe().Ctx(ctx).
		Where("used_tenant_id", paidTenantId).
		Where("status", webInviteStatusUsed).
		WhereNull("deleted_at").
		OrderDesc("used_at").
		Scan(&row); err != nil {
		return gerror.Wrap(err, "读取邀请关系失败")
	}
	if row == nil || row.InviterTenantId <= 0 || row.InviterTenantId == paidTenantId {
		return nil
	}
	return s.openTenantVip(ctx, row.InviterTenantId, 1, cfg.InviteRewardDays, "邀请用户付款奖励", "invite_reward")
}

func (s *sSysPublish) shouldRewardInviterVip(ctx context.Context, tenantId int64) (bool, error) {
	cols := dao.AdminOrder.Columns()
	count, err := dao.AdminOrder.Ctx(ctx).
		Where(cols.OrderType, tenantVipOrderType).
		Where(cols.ProductId, tenantId).
		Where(cols.Status, consts.OrderStatusPay).
		Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查会员首单失败")
	}
	return count == 0, nil
}
