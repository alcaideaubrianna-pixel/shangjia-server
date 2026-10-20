// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishRelayFile is the golang structure for table youban_publish_relay_file.
type YoubanPublishRelayFile struct {
	Id             int64       `json:"id"             orm:"id"               ` //
	TenantId       int64       `json:"tenantId"       orm:"tenant_id"        ` //
	TelegramUserId int64       `json:"telegramUserId" orm:"telegram_user_id" ` //
	AssetKey       string      `json:"assetKey"       orm:"asset_key"        ` //
	FileId         string      `json:"fileId"         orm:"file_id"          ` //
	FileUniqueId   string      `json:"fileUniqueId"   orm:"file_unique_id"   ` //
	CreatedAt      *gtime.Time `json:"createdAt"      orm:"created_at"       ` //
	UpdatedAt      *gtime.Time `json:"updatedAt"      orm:"updated_at"       ` //
}
