package consts

import (
	"hotgo/internal/library/dict"
	"hotgo/internal/model"
)

const (
	RelayChannelStatusEnabled  = "enabled"
	RelayChannelStatusDisabled = "disabled"
	RelayDefaultCapacity       = 2
	RelayMaxCapacity           = 20
	RelaySendPhasePreparing    = "relay_preparing"
	RelaySendPhaseReady        = "relay_ready"
	RelaySendPhaseSending      = "package_sending"
	RelaySendPhaseConfirmed    = "package_confirmed"
	RelayStatusUnknown         = "unknown"
	RelayStatusReady           = "ready"
	RelayStatusCooldown        = "cooldown"
	RelayStatusError           = "error"
	RelayStatusMissing         = "missing"
	RelayStatusPending         = "pending"
	RelayStatusCreating        = "creating"
	RelayStatusChecking        = "checking"
	RelayStatusFailed          = "failed"
	RelayStatusPreparing       = "preparing"
	RelayStatusCopying         = "copying"
	RelayStatusConfirmed       = "confirmed"
	RelayStatusRetryWait       = "retry_wait"
	RelayStatusCanceled        = "canceled"
)

var RelayChannelStatusOptions = []*model.Option{dict.GenSuccessOption(RelayChannelStatusEnabled, "启用"), dict.GenDefaultOption(RelayChannelStatusDisabled, "停用")}
var RelayChannelHealthOptions = []*model.Option{dict.GenDefaultOption(RelayStatusUnknown, "未检查"), dict.GenSuccessOption(RelayStatusReady, "就绪"), dict.GenWarningOption(RelayStatusCooldown, "冷却中"), dict.GenErrorOption(RelayStatusError, "异常"), dict.GenErrorOption(RelayStatusMissing, "缺少权限")}
var RelayOperationStatusOptions = []*model.Option{dict.GenDefaultOption("", "未提交"), dict.GenInfoOption(RelayStatusPending, "等待处理"), dict.GenInfoOption(RelayStatusCreating, "正在创建"), dict.GenInfoOption(RelayStatusChecking, "正在检查"), dict.GenSuccessOption(RelayStatusReady, "已完成"), dict.GenErrorOption(RelayStatusFailed, "失败，需检查")}

func init() {
	dict.RegisterEnums("YoubanRelayChannelStatusOptions", "中转频道状态", RelayChannelStatusOptions)
	dict.RegisterEnums("YoubanRelayChannelHealthOptions", "中转频道健康", RelayChannelHealthOptions)
	dict.RegisterEnums("YoubanRelayOperationStatusOptions", "中转管理任务状态", RelayOperationStatusOptions)
}
