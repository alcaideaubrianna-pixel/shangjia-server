package sys

import (
	"fmt"
	"strings"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/model"
	baseentity "hotgo/internal/model/entity"
)

func tenantVipPlanByCode(code string, cfg *model.YoubanPublishVipConfig) *sysin.TenantVipPlanModel {
	if code == tenantVipPlanMonth {
		return tenantVipPlanByConfig(cfg)
	}
	return nil
}

func tenantVipPlanByConfig(cfg *model.YoubanPublishVipConfig) *sysin.TenantVipPlanModel {
	if cfg == nil {
		cfg = tenantVipDefaultConfig()
	}
	return &sysin.TenantVipPlanModel{
		Code:          tenantVipPlanMonth,
		CouponAmount:  cfg.CouponAmount,
		CouponEnabled: cfg.CouponEnabled,
		Currency:      tenantVipCurrency(cfg),
		Days:          30,
		Description:   "适合需要相似查重、搜索和自动化采集的团队",
		DiscountText:  cfg.DiscountText,
		Features:      tenantVipPaidFeatures(),
		PayItems:      []*sysin.TenantVipPayItemModel{tenantVipDefaultPayItem(cfg)},
		Level:         1,
		Name:          "VIP计划",
		OriginalPrice: cfg.OriginalPrice,
		Price:         cfg.MonthlyPrice,
	}
}

func tenantVipOrderAmount(price float64, couponCode string, cfg *model.YoubanPublishVipConfig) float64 {
	amount := price
	if cfg != nil && cfg.CouponEnabled && cfg.CouponAmount > 0 && strings.TrimSpace(cfg.CouponCode) != "" && strings.EqualFold(strings.TrimSpace(cfg.CouponCode), strings.TrimSpace(couponCode)) {
		amount -= cfg.CouponAmount
	}
	if amount < 0 {
		return 0
	}
	return amount
}

func tenantVipOrderModel(order *baseentity.AdminOrder, pay *baseentity.PayLog, plan *sysin.TenantVipPlanModel) *sysin.TenantVipOrderModel {
	res := &sysin.TenantVipOrderModel{Id: order.Id, OrderNo: order.OrderSn, Amount: order.Money, Status: order.Status, StatusTxt: tenantVipOrderStatusText(order.Status), CreatedAt: order.CreatedAt}
	if plan != nil {
		res.PlanCode = plan.Code
		res.PlanName = plan.Name
		res.Currency = plan.Currency
	}
	if pay != nil {
		res.TradeType = pay.TradeType
		res.PaidAt = pay.PayAt
	}
	return res
}

func tenantVipOrderStatusText(status int) string {
	if status == consts.OrderStatusPay {
		return "已支付"
	}
	if status == consts.OrderStatusNotPay {
		return "待支付"
	}
	return "已关闭"
}

func tenantVipDefaultConfig() *model.YoubanPublishVipConfig {
	return &model.YoubanPublishVipConfig{
		Enabled:        true,
		MonthlyPrice:   30,
		OriginalPrice:  60,
		DiscountText:   "限时半价",
		PaymentGateway: consts.PayTypeGMPay,
		Currency:       "USDT",
	}
}

func tenantVipPaymentGateway(cfg *model.YoubanPublishVipConfig) string {
	if cfg == nil {
		return consts.PayTypeGMPay
	}
	switch strings.ToLower(strings.TrimSpace(cfg.PaymentGateway)) {
	case consts.PayTypeRainbow, consts.PayTypeGMPay:
		return strings.ToLower(strings.TrimSpace(cfg.PaymentGateway))
	default:
		return consts.PayTypeGMPay
	}
}

func tenantVipCurrency(cfg *model.YoubanPublishVipConfig) string {
	if cfg == nil {
		return "USDT"
	}
	switch strings.ToUpper(strings.TrimSpace(cfg.Currency)) {
	case "RMB", "USDT":
		return strings.ToUpper(strings.TrimSpace(cfg.Currency))
	default:
		return "USDT"
	}
}

func tenantVipDefaultPayItem(cfg *model.YoubanPublishVipConfig) *sysin.TenantVipPayItemModel {
	if cfg == nil {
		cfg = tenantVipDefaultConfig()
	}
	gateway := tenantVipPaymentGateway(cfg)
	currency := tenantVipCurrency(cfg)
	switch gateway {
	case consts.PayTypeGMPay:
		return &sysin.TenantVipPayItemModel{Label: "GMPay", PayType: consts.PayTypeGMPay, TradeType: consts.TradeTypeRainbowUSDT, Enabled: true, Money: cfg.MonthlyPrice}
	default:
		tradeType := consts.TradeTypeRainbowUSDT
		if currency == "RMB" {
			tradeType = ""
		}
		return &sysin.TenantVipPayItemModel{Label: "彩虹易支付", PayType: consts.PayTypeRainbow, TradeType: tradeType, Enabled: true, Money: cfg.MonthlyPrice}
	}
}

func tenantVipFreeFeatures() []string {
	return []string{"基础资料管理", "频道全量推送", "频道自动删除关键字", "群聊消息推送", "资料管理搜索", "双向机器人", "汇率机器人", "机器人管理资料", "频道资料循环", "多账号管理"}
}

func tenantVipPaidFeatures() []string {
	return []string{"无限展示图片与随机推送", "防扫图", "资料相似查询", "图片搜索", "采集代理", "群聊关键字监听", "可联系管理员开启独立访问域名"}
}

func tenantVipCacheKey(tenantId int64) string {
	return fmt.Sprintf("youban_publish:tenant_vip:%d", tenantId)
}

func tenantVipFullCacheKey(tenantId int64) string {
	return fmt.Sprintf("youban_publish:tenant_vip_full:%d", tenantId)
}

func containsString(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}
