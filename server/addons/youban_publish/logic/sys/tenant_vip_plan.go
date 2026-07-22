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
		ActivityText:  cfg.ActivityText,
		ActivityTitle: cfg.ActivityTitle,
		Code:          tenantVipPlanMonth,
		CouponAmount:  cfg.CouponAmount,
		CouponEnabled: cfg.CouponEnabled,
		Currency:      "U",
		Days:          30,
		Description:   "适合需要相似查重、搜索和自动化采集的团队",
		DiscountText:  cfg.DiscountText,
		Features:      tenantVipPaidFeatures(),
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
	return &model.YoubanPublishVipConfig{Enabled: true, MonthlyPrice: 30, OriginalPrice: 60, DiscountText: "限时半价", InviteRewardDays: 30, ActivityTitle: "邀请返会员", ActivityText: "邀请好友注册并完成首月付款后，邀请人自动获得 1 个月 VIP，有效期可叠加。"}
}

func tenantVipFreeFeatures() []string {
	return []string{"基础资料管理", "频道全量推送", "频道自动删除关键字", "群聊消息推送", "防扫图", "资料管理搜索", "双向机器人", "汇率机器人", "机器人管理资料", "频道资料循环", "多账号管理"}
}

func tenantVipPaidFeatures() []string {
	return []string{"资料相似查询", "图片搜索", "采集代理", "群聊关键字监听", "可联系管理员开启独立访问域名"}
}

func tenantVipCacheKey(tenantId int64) string {
	return fmt.Sprintf("youban_publish:tenant_vip:%d", tenantId)
}

func containsString(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}
