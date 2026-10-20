// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotManagedChannel is the golang structure for table youban_bot_managed_channel.
type YoubanBotManagedChannel struct {
	Id               int64       `json:"id"               orm:"id"                 ` //
	TenantId         int64       `json:"tenantId"         orm:"tenant_id"          ` //
	AccountId        int64       `json:"accountId"        orm:"account_id"         ` //
	PublishChannelId int64       `json:"publishChannelId" orm:"publish_channel_id" ` //
	TgAccountId      int64       `json:"tgAccountId"      orm:"tg_account_id"      ` //
	ChatId           string      `json:"chatId"           orm:"chat_id"            ` //
	Title            string      `json:"title"            orm:"title"              ` //
	AccessHash       string      `json:"accessHash"       orm:"access_hash"        ` //
	VisitUrl         string      `json:"visitUrl"         orm:"visit_url"          ` //
	AdminCount       int         `json:"adminCount"       orm:"admin_count"        ` //
	AdminLimit       int         `json:"adminLimit"       orm:"admin_limit"        ` //
	BotCount         int         `json:"botCount"         orm:"bot_count"          ` //
	CanManage        int         `json:"canManage"        orm:"can_manage"         ` //
	SyncStatus       string      `json:"syncStatus"       orm:"sync_status"        ` //
	ErrorMessage     string      `json:"errorMessage"     orm:"error_message"      ` //
	LastSyncedAt     *gtime.Time `json:"lastSyncedAt"     orm:"last_synced_at"     ` //
	LastAssignedAt   *gtime.Time `json:"lastAssignedAt"   orm:"last_assigned_at"   ` //
	CreatedAt        *gtime.Time `json:"createdAt"        orm:"created_at"         ` //
	UpdatedAt        *gtime.Time `json:"updatedAt"        orm:"updated_at"         ` //
}
