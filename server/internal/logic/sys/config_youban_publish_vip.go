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
		{key: "youbanPublishVipMonthlyPrice", name: "上架VIP月价", typ: consts.ConfigTypeFloat64, value: defaultCfg.MonthlyPrice, sort: 2},
		{key: "youbanPublishVipOriginalPrice", name: "上架VIP原价", typ: consts.ConfigTypeFloat64, value: defaultCfg.OriginalPrice, sort: 3},
		{key: "youbanPublishVipDiscountText", name: "上架VIP折扣文案", typ: consts.ConfigTypeString, value: defaultCfg.DiscountText, sort: 4},
		{key: "youbanPublishVipCouponEnabled", name: "上架VIP优惠券开关", typ: consts.ConfigTypeBool, value: defaultCfg.CouponEnabled, sort: 5},
		{key: "youbanPublishVipCouponCode", name: "上架VIP优惠券码", typ: consts.ConfigTypeString, value: defaultCfg.CouponCode, sort: 6},
		{key: "youbanPublishVipCouponAmount", name: "上架VIP优惠金额", typ: consts.ConfigTypeFloat64, value: defaultCfg.CouponAmount, sort: 7},
		{key: "youbanPublishVipInviteRewardDays", name: "邀请奖励天数", typ: consts.ConfigTypeInt, value: defaultCfg.InviteRewardDays, sort: 8},
		{key: "youbanPublishVipActivityTitle", name: "上架VIP活动标题", typ: consts.ConfigTypeString, value: defaultCfg.ActivityTitle, sort: 9},
		{key: "youbanPublishVipActivityText", name: "上架VIP活动说明", typ: consts.ConfigTypeString, value: defaultCfg.ActivityText, sort: 10},
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
		Enabled:          true,
		MonthlyPrice:     30,
		OriginalPrice:    60,
		DiscountText:     "限时半价",
		CouponEnabled:    true,
		CouponCode:       "",
		CouponAmount:     0,
		InviteRewardDays: 30,
		ActivityTitle:    "邀请返会员",
		ActivityText:     "邀请好友注册并完成首月付款后，邀请人自动获得 1 个月 VIP，有效期可叠加。",
	}
}
