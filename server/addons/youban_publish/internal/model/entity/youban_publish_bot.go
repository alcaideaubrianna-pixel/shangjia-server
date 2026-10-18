// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishBot is the golang structure for table youban_publish_bot.
type YoubanPublishBot struct {
	Id          int64       `json:"id"          orm:"id"           ` //
	TenantId    int64       `json:"tenantId"    orm:"tenant_id"    ` //
	BotName     string      `json:"botName"     orm:"bot_name"     ` //
	BotUsername string      `json:"botUsername" orm:"bot_username" ` //
	BotToken    string      `json:"botToken"    orm:"bot_token"    ` //
	Remark      string      `json:"remark"      orm:"remark"       ` //
	Status      int         `json:"status"      orm:"status"       ` //
	CreatedBy   int64       `json:"createdBy"   orm:"created_by"   ` //
	UpdatedBy   int64       `json:"updatedBy"   orm:"updated_by"   ` //
	DeletedBy   int64       `json:"deletedBy"   orm:"deleted_by"   ` //
	CreatedAt   *gtime.Time `json:"createdAt"   orm:"created_at"   ` //
	UpdatedAt   *gtime.Time `json:"updatedAt"   orm:"updated_at"   ` //
	DeletedAt   *gtime.Time `json:"deletedAt"   orm:"deleted_at"   ` //
	AccountId   int64       `json:"accountId"   orm:"account_id"   ` //
	BotType     string      `json:"botType"     orm:"bot_type"     ` //
}
