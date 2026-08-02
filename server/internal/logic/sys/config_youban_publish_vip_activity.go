package sys

import (
	"context"

	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/model"
	"hotgo/internal/model/entity"
	"hotgo/internal/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"
)

const youbanPublishVipActivityConfigGroup = "youban_publish_vip_activity"

func (s *sSysConfig) GetYoubanPublishVipActivity(ctx context.Context) (conf *model.YoubanPublishVipActivityConfig, err error) {
	if err = s.ensureYoubanPublishVipActivityConfig(ctx); err != nil {
		return
	}
	models, err := s.GetConfigByGroup(ctx, &sysin.GetConfigInp{Group: youbanPublishVipActivityConfigGroup})
	if err != nil {
		return nil, err
	}
	err = gconv.Scan(models.List, &conf)
	if conf == nil {
		conf = defaultYoubanPublishVipActivityConfig()
	}
	return
}

func (s *sSysConfig) ensureYoubanPublishVipActivityConfig(ctx context.Context) (err error) {
	defaultCfg := defaultYoubanPublishVipActivityConfig()
	nowText := gtime.Now().Format("Y-m-d H:i:s")
	defaults := []struct {
		key   string
		name  string
		typ   string
		value interface{}
		sort  int
	}{
		{key: "youbanPublishVipBindGiftEnabled", name: "绑定TG赠会员开关", typ: consts.ConfigTypeBool, value: defaultCfg.BindGiftEnabled, sort: 1},
		{key: "youbanPublishVipBindGiftDays", name: "绑定TG奖励天数", typ: consts.ConfigTypeInt, value: defaultCfg.BindGiftDays, sort: 2},
		{key: "youbanPublishVipBindGiftEnabledAt", name: "绑定TG活动启用时间", typ: consts.ConfigTypeString, value: nowText, sort: 2},
		{key: "youbanPublishVipInviteBindGiftEnabled", name: "邀请绑定奖励开关", typ: consts.ConfigTypeBool, value: defaultCfg.InviteBindGiftEnabled, sort: 3},
		{key: "youbanPublishVipInviteBindGiftDays", name: "邀请绑定奖励天数", typ: consts.ConfigTypeInt, value: defaultCfg.InviteBindGiftDays, sort: 4},
		{key: "youbanPublishVipInviteBindGiftEnabledAt", name: "邀请绑定活动启用时间", typ: consts.ConfigTypeString, value: nowText, sort: 4},
		{key: "youbanPublishVipInviteFirstPayGiftEnabled", name: "邀请首付奖励开关", typ: consts.ConfigTypeBool, value: defaultCfg.InviteFirstPayEnabled, sort: 5},
		{key: "youbanPublishVipInviteFirstPayGiftDays", name: "邀请首付奖励天数", typ: consts.ConfigTypeInt, value: defaultCfg.InviteFirstPayDays, sort: 6},
		{key: "youbanPublishVipInviteFirstPayGiftEnabledAt", name: "邀请首付活动启用时间", typ: consts.ConfigTypeString, value: nowText, sort: 6},
		{key: "youbanPublishVipEventTrackingStartedAt", name: "会员事件追踪起点", typ: consts.ConfigTypeString, value: nowText, sort: 6},
		{key: "youbanPublishVipActivityBannerTitle", name: "会员活动标题", typ: consts.ConfigTypeString, value: defaultCfg.ActivityBannerTitle, sort: 7},
		{key: "youbanPublishVipActivityBannerText", name: "会员活动说明", typ: consts.ConfigTypeString, value: defaultCfg.ActivityBannerText, sort: 8},
		{key: "youbanPublishVipPayNotifyEnabled", name: "会员充值通知开关", typ: consts.ConfigTypeBool, value: defaultCfg.PayNotifyEnabled, sort: 9},
		{key: "youbanPublishVipGiftNotifyEnabled", name: "会员赠送通知开关", typ: consts.ConfigTypeBool, value: defaultCfg.GiftNotifyEnabled, sort: 10},
		{key: "youbanPublishVipAdminAdjustNotifyEnabled", name: "后台调整通知开关", typ: consts.ConfigTypeBool, value: defaultCfg.AdminAdjustNotifyEnabled, sort: 11},
		{key: "youbanPublishVipExpiredNotifyEnabled", name: "会员到期通知开关", typ: consts.ConfigTypeBool, value: defaultCfg.ExpiredNotifyEnabled, sort: 12},
	}

	cols := dao.SysConfig.Columns()
	var rows []*entity.SysConfig
	if err = dao.SysConfig.Ctx(ctx).Where(cols.Group, youbanPublishVipActivityConfigGroup).Scan(&rows); err != nil {
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
			cols.Group:        youbanPublishVipActivityConfigGroup,
			cols.Key:          item.key,
			cols.Name:         item.name,
			cols.Type:         item.typ,
			cols.Value:        normalizeConfigValue(item.value),
			cols.DefaultValue: normalizeConfigValue(item.value),
			cols.IsDefault:    0,
			cols.Sort:         item.sort,
			cols.Tip:          "上架系统VIP活动配置",
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

func (s *sSysConfig) refreshYoubanPublishVipActivityEnabledAt(ctx context.Context, tx gdb.TX, list g.Map, rows []*entity.SysConfig) error {
	mappings := []struct {
		enabledKey string
		enabledAt  string
	}{
		{enabledKey: "youbanPublishVipBindGiftEnabled", enabledAt: "youbanPublishVipBindGiftEnabledAt"},
		{enabledKey: "youbanPublishVipInviteBindGiftEnabled", enabledAt: "youbanPublishVipInviteBindGiftEnabledAt"},
		{enabledKey: "youbanPublishVipInviteFirstPayGiftEnabled", enabledAt: "youbanPublishVipInviteFirstPayGiftEnabledAt"},
	}
	columns := dao.SysConfig.Columns()
	for _, mapping := range mappings {
		newValue, exists := list[mapping.enabledKey]
		if !exists || !gconv.Bool(newValue) {
			continue
		}
		enabledRow := s.getConfigByKey(mapping.enabledKey, rows)
		if enabledRow != nil && gconv.Bool(enabledRow.Value) {
			continue
		}
		enabledAtRow := s.getConfigByKey(mapping.enabledAt, rows)
		if enabledAtRow == nil {
			continue
		}
		if _, err := tx.Model(dao.SysConfig.Table()).Ctx(ctx).Where(columns.Id, enabledAtRow.Id).Data(g.Map{
			columns.Value:     gtime.Now().Format("Y-m-d H:i:s"),
			columns.UpdatedAt: gtime.Now(),
		}).Update(); err != nil {
			return err
		}
	}
	return nil
}

func defaultYoubanPublishVipActivityConfig() *model.YoubanPublishVipActivityConfig {
	return &model.YoubanPublishVipActivityConfig{
		BindGiftEnabled:          true,
		BindGiftDays:             1,
		InviteBindGiftEnabled:    true,
		InviteBindGiftDays:       3,
		InviteFirstPayEnabled:    true,
		InviteFirstPayDays:       30,
		ActivityBannerTitle:      "邀请好友，会员时长持续叠加",
		ActivityBannerText:       "绑定 Telegram 赠送 1 天，邀请好友完成绑定赠送 3 天，好友首次开通月卡再赠送 1 个月。",
		PayNotifyEnabled:         true,
		GiftNotifyEnabled:        true,
		AdminAdjustNotifyEnabled: true,
		ExpiredNotifyEnabled:     true,
	}
}
