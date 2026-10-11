package sys

const (
	relayChannelStatusEnabled  = "enabled"
	relayChannelStatusDisabled = "disabled"

	relayChannelHealthUnknown  = "unknown"
	relayChannelHealthReady    = "ready"
	relayChannelHealthCooldown = "cooldown"
	relayChannelHealthError    = "error"

	relayBotPermissionUnknown = "unknown"
	relayBotPermissionReady   = "ready"
	relayBotPermissionMissing = "missing"
)

type relayChannelRecord struct {
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
