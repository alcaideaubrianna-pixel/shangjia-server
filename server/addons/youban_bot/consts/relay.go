package consts

import (
	"hotgo/internal/library/dict"
	"hotgo/internal/model"
)

const (
	RelayFeatureKey  = "two_way_relay"
	RelayRuleUser    = "user"
	RelayRuleGroup   = "group"
	RelayRuleKeyword = "keyword"
	RelayPending     = "pending"
	RelaySending     = "sending"
	RelaySent        = "sent"
	RelayFailed      = "failed"
	RelayUncertain   = "uncertain"
	RelayStored      = "stored"
	RelayCancelled   = "cancelled"
	RelayKindChat    = "relay"
	RelayKindAudit   = "audit"
	RelayKindArchive = "archive"
)

var RelaySwitchOptions = []*model.Option{dict.GenSuccessOption(1, "启用"), dict.GenDefaultOption(0, "停用")}
var RelayRuleOptions = []*model.Option{dict.GenDefaultOption(RelayRuleUser, "用户"), dict.GenDefaultOption(RelayRuleGroup, "群聊"), dict.GenDefaultOption(RelayRuleKeyword, "关键词")}

var RelayDeliveryStatusOptions = []*model.Option{
	dict.GenInfoOption(RelayPending, "待发送"), dict.GenPrimaryOption(RelaySending, "发送中"), dict.GenSuccessOption(RelaySent, "已发送"), dict.GenErrorOption(RelayFailed, "发送失败"), dict.GenWarningOption(RelayUncertain, "结果不确定"), dict.GenDefaultOption(RelayCancelled, "已取消"), dict.GenInfoOption(RelayStored, "已存档"),
}
var RelayDeliveryKindOptions = []*model.Option{dict.GenDefaultOption(RelayKindChat, "双向传递"), dict.GenInfoOption(RelayKindAudit, "审计转发"), dict.GenInfoOption(RelayKindArchive, "聊天存档")}

func init() {
	dict.RegisterEnums("YoubanBotRelayDeliveryStatusOptions", "聊天投递状态", RelayDeliveryStatusOptions)
	dict.RegisterEnums("YoubanBotRelayDeliveryKindOptions", "聊天记录类型", RelayDeliveryKindOptions)
	dict.RegisterEnums("YoubanBotRelaySwitchOptions", "双向机器人开关", RelaySwitchOptions)
	dict.RegisterEnums("YoubanBotRelayRuleOptions", "双向机器人规则", RelayRuleOptions)
}
