package sys

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/model/entity"
)

func tenantVipRenewalOrderNo(account *sysin.AccountModel, key string) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s", account.TenantId, account.Id, key)))
	return fmt.Sprintf("YBPB%x", digest[:16])
}

// The order and payment log survive gateway timeouts and Bot-session write
// failures. Retries reuse the persisted out_trade_no instead of creating a new
// payment identity. Ownership and immutable purchase parameters are rechecked.
func (s *sSysPublish) loadTenantVipRenewalOrder(ctx context.Context, account *sysin.AccountModel, in *sysin.TenantVipOrderCreateInp) (*entity.AdminOrder, *entity.PayLog, error) {
	var order *entity.AdminOrder
	c := dao.AdminOrder.Columns()
	err := dao.AdminOrder.Ctx(ctx).Where(c.OrderSn, tenantVipRenewalOrderNo(account, in.RequestKey)).Where(c.OrderType, tenantVipOrderType).Where(c.ProductId, account.TenantId).Where(c.MemberId, account.Id).Scan(&order)
	if err != nil {
		return nil, nil, gerror.Wrap(err, "读取续费确认订单失败")
	}
	if order == nil {
		return nil, nil, nil
	}
	var pay *entity.PayLog
	pc := dao.PayLog.Columns()
	if err := dao.PayLog.Ctx(ctx).Where(pc.OrderSn, order.OrderSn).Where(pc.OrderGroup, tenantVipOrderGroup).Where(pc.MemberId, account.Id).Scan(&pay); err != nil {
		return nil, nil, gerror.Wrap(err, "读取续费支付记录失败")
	}
	if pay == nil || pay.Detail == nil {
		return nil, nil, gerror.New("续费订单支付记录不完整，请联系管理员；请勿重复创建订单")
	}
	if pay.Detail.Get("planCode").String() != strings.TrimSpace(in.PlanCode) || !strings.EqualFold(pay.Detail.Get("couponCode").String(), strings.TrimSpace(in.CouponCode)) {
		return nil, nil, gerror.New("本次确认已关联其他套餐或优惠码，请重新打开续费")
	}
	return order, pay, nil
}

func (s *sSysPublish) resumeTenantVipRenewalOrder(ctx context.Context, account *sysin.AccountModel, order *entity.AdminOrder, pay *entity.PayLog) (*sysin.TenantVipOrderModel, error) {
	plan := &sysin.TenantVipPlanModel{Code: pay.Detail.Get("planCode").String(), Name: pay.Detail.Get("planName").String(), Currency: pay.Detail.Get("currency").String(), Days: pay.Detail.Get("days").Int(), Level: pay.Detail.Get("level").Int()}
	res := tenantVipOrderModel(order, pay, plan)
	if order.Money <= 0 && order.Status == consts.OrderStatusPay {
		_, err := s.applyTenantVipExtension(ctx, &tenantVipChangeInp{EventKey: fmt.Sprintf("%s:%d", tenantVipEventPay, order.Id), EventType: tenantVipEventPay, TenantId: account.TenantId, AccountId: account.Id, ReferenceType: "order", ReferenceId: fmt.Sprint(order.Id), Level: plan.Level, Days: plan.Days, Remark: order.Remark})
		return res, err
	}
	if order.Status == consts.OrderStatusPay {
		return res, nil
	}
	if order.Status != consts.OrderStatusNotPay {
		return nil, gerror.New("本次续费订单已关闭，请重新选择套餐")
	}
	result, err := s.TenantVipOrderPay(ctx, &sysin.TenantVipOrderPayInp{Id: order.Id})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, gerror.New("支付服务未返回订单")
	}
	res.Order = result.Order
	res.PayUrl = result.PayUrl
	res.TradeType = result.TradeType
	return res, nil
}
