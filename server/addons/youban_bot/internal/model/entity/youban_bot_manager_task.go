// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotManagerTask is the golang structure for table youban_bot_manager_task.
type YoubanBotManagerTask struct {
	Id             int64       `json:"id"             orm:"id"               ` //
	TenantId       int64       `json:"tenantId"       orm:"tenant_id"        ` //
	AccountId      int64       `json:"accountId"      orm:"account_id"       ` //
	EntryBotId     int64       `json:"entryBotId"     orm:"entry_bot_id"     ` //
	TelegramUserId int64       `json:"telegramUserId" orm:"telegram_user_id" ` //
	BatchKey       string      `json:"batchKey"       orm:"batch_key"        ` //
	Action         string      `json:"action"         orm:"action"           ` //
	ManagedBotId   int64       `json:"managedBotId"   orm:"managed_bot_id"   ` //
	ChannelId      int64       `json:"channelId"      orm:"channel_id"       ` //
	RightsJson     string      `json:"rightsJson"     orm:"rights_json"      ` //
	Status         string      `json:"status"         orm:"status"           ` //
	AccountTaskId  int64       `json:"accountTaskId"  orm:"account_task_id"  ` //
	ErrorMessage   string      `json:"errorMessage"   orm:"error_message"    ` //
	Notified       int         `json:"notified"       orm:"notified"         ` //
	CreatedAt      *gtime.Time `json:"createdAt"      orm:"created_at"       ` //
	UpdatedAt      *gtime.Time `json:"updatedAt"      orm:"updated_at"       ` //
}
