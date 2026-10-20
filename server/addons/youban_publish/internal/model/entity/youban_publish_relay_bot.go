// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishRelayBot is the golang structure for table youban_publish_relay_bot.
type YoubanPublishRelayBot struct {
	Id             int64       `json:"id"             orm:"id"               ` //
	TenantId       int64       `json:"tenantId"       orm:"tenant_id"        ` //
	RelayChannelId int64       `json:"relayChannelId" orm:"relay_channel_id" ` //
	BotId          int64       `json:"botId"          orm:"bot_id"           ` //
	TelegramUserId int64       `json:"telegramUserId" orm:"telegram_user_id" ` //
	Status         string      `json:"status"         orm:"status"           ` //
	CheckedAt      *gtime.Time `json:"checkedAt"      orm:"checked_at"       ` //
	LastError      string      `json:"lastError"      orm:"last_error"       ` //
	CreatedAt      *gtime.Time `json:"createdAt"      orm:"created_at"       ` //
	UpdatedAt      *gtime.Time `json:"updatedAt"      orm:"updated_at"       ` //
}
