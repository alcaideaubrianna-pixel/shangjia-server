package sys

import (
	"context"

	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/model"
	"hotgo/internal/model/entity"
	"hotgo/internal/model/input/sysin"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"
)

func (s *sSysConfig) GetYoubanPublishVip(ctx context.Context) (conf *model.YoubanPublishVipConfig, err error) {
	if err = s.ensureYoubanPublishVipConfig(ctx); err != nil {
		return
	}
	models, err := s.GetConfigByGroup(ctx, &sysin.GetConfigInp{Group: "youban_publish_vip"})
	if err != nil {
		return
	}
	err = gconv.Scan(models.List, &conf)
	if conf == nil {
		conf = defaultYoubanPublishVipConfig()
	}
	return
}

func (s *sSysConfig) ensureYoubanPublishVipConfig(ctx context.Context) (err error) {
	defaultCfg := defaultYoubanPublishVipConfig()
	defaults := []struct {
		key   string
		name  string
		typ   string
		value interface{}
		sort  int
	}{
		{key: "youbanPublishVipEnabled", name: "上架VIP开关", typ: consts.ConfigTypeBool, value: defaultCfg.Enabled, sort: 1},
		{key: "youbanPublishVipMonthlyPrice", name: "上架VIP月价", typ: consts.ConfigTypeFloat64, value: defaultCfg.MonthlyPrice, sort: 3},
		{key: "youbanPublishVipOriginalPrice", name: "上架VIP原价", typ: consts.ConfigTypeFloat64, value: defaultCfg.OriginalPrice, sort: 4},
		{key: "youbanPublishVipDiscountText", name: "上架VIP折扣文案", typ: consts.ConfigTypeString, value: defaultCfg.DiscountText, sort: 5},
		{key: "youbanPublishVipCouponEnabled", name: "上架VIP优惠券开关", typ: consts.ConfigTypeBool, value: defaultCfg.CouponEnabled, sort: 6},
		{key: "youbanPublishVipCouponCode", name: "上架VIP优惠券码", typ: consts.ConfigTypeString, value: defaultCfg.CouponCode, sort: 7},
		{key: "youbanPublishVipCouponAmount", name: "上架VIP优惠金额", typ: consts.ConfigTypeFloat64, value: defaultCfg.CouponAmount, sort: 8},
		{key: "youbanPublishVipPaymentGateway", name: "上架VIP支付网关", typ: consts.ConfigTypeString, value: defaultCfg.PaymentGateway, sort: 9},
		{key: "youbanPublishVipCurrency", name: "上架VIP币种", typ: consts.ConfigTypeString, value: defaultCfg.Currency, sort: 10},
	}

	cols := dao.SysConfig.Columns()
	var rows []*entity.SysConfig
	if err = dao.SysConfig.Ctx(ctx).Where(cols.Group, "youban_publish_vip").Scan(&rows); err != nil {
		return
	}
	exists := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		exists[row.Key] = struct{}{}
	}
	for _, item := range defaults {
		if _, ok := exists[item.key]; ok {
			continue
		}
		_, err = dao.SysConfig.Ctx(ctx).Data(g.Map{
			cols.Group:        "youban_publish_vip",
			cols.Key:          item.key,
			cols.Name:         item.name,
			cols.Type:         item.typ,
			cols.Value:        normalizeConfigValue(item.value),
			cols.DefaultValue: normalizeConfigValue(item.value),
			cols.IsDefault:    0,
			cols.Sort:         item.sort,
			cols.Tip:          "上架系统VIP配置",
			cols.Status:       consts.StatusEnabled,
			cols.CreatedAt:    gtime.Now(),
			cols.UpdatedAt:    gtime.Now(),
		}).Insert()
		if err != nil {
			return
		}
	}
	return
}

func defaultYoubanPublishVipConfig() *model.YoubanPublishVipConfig {
	return &model.YoubanPublishVipConfig{
		Enabled:        true,
		MonthlyPrice:   30,
		OriginalPrice:  60,
		DiscountText:   "限时半价",
		CouponEnabled:  true,
		CouponCode:     "",
		CouponAmount:   0,
		PaymentGateway: consts.PayTypeGMPay,
		Currency:       "USDT",
	}
}
