package sys

import relayconsts "hotgo/addons/youban_publish/consts"

const (
	relayChannelStatusEnabled  = relayconsts.RelayChannelStatusEnabled
	relayChannelStatusDisabled = relayconsts.RelayChannelStatusDisabled

	relayChannelHealthUnknown  = relayconsts.RelayStatusUnknown
	relayChannelHealthReady    = relayconsts.RelayStatusReady
	relayChannelHealthCooldown = relayconsts.RelayStatusCooldown
	relayChannelHealthError    = relayconsts.RelayStatusError

	relayBotPermissionUnknown = relayconsts.RelayStatusUnknown
	relayBotPermissionReady   = relayconsts.RelayStatusReady
	relayBotPermissionMissing = relayconsts.RelayStatusMissing
)

type relayChannelRecord struct {
	OperationStatus     string `json:"operationStatus"`
	Id                  int64  `json:"id"`
	TenantId            int64  `json:"tenantId"`
	TgAccountId         int64  `json:"tgAccountId"`
	ChannelId           string `json:"channelId"`
	AccessHash          string `json:"accessHash"`
	ChannelTitle        string `json:"channelTitle"`
	TargetChatId        string `json:"targetChatId"`
	BotId               int64  `json:"botId"`
	Status              string `json:"status"`
	HealthStatus        string `json:"healthStatus"`
	BotPermissionStatus string `json:"botPermissionStatus"`
	CooldownUntil       string `json:"cooldownUntil"`
	LastError           string `json:"lastError"`
	InFlightCount       int    `json:"inFlightCount"`
	LastUsedAt          string `json:"lastUsedAt"`
}
