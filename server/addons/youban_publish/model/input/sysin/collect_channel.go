package sysin

const (
	CollectChannelStatusActive    = "active"
	CollectChannelStatusArchived  = "archived"
	CollectBotChannelDiscovered   = "discovered"
	CollectBotChannelEnabled      = "enabled"
	CollectBotChannelDisabled     = "disabled"
	CollectBotChannelRemoved      = "bot_removed"
	CollectAccountChannelPending  = "pending"
	CollectAccountChannelEnabled  = "enabled"
	CollectAccountChannelDisabled = "disabled"
	CollectAccountChannelRemoved  = "bot_removed"
)

type CollectChannelModel struct {
	Id             int64  `json:"id"`
	TenantId       int64  `json:"tenantId"`
	TelegramChatId string `json:"telegramChatId"`
	ChatType       string `json:"chatType"`
	Title          string `json:"title"`
	Username       string `json:"username"`
	InviteLink     string `json:"inviteLink"`
	About          string `json:"about"`
	PhotoFileId    string `json:"photoFileId"`
	IsBroadcast    int    `json:"isBroadcast"`
	IsMegagroup    int    `json:"isMegagroup"`
	LastMessageId  int64  `json:"lastMessageId"`
	Status         string `json:"status"`
}
