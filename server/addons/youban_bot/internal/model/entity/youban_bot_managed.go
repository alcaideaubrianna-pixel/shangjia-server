// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotManaged is the golang structure for table youban_bot_managed.
type YoubanBotManaged struct {
	Id            int64       `json:"id"            orm:"id"              ` //
	TenantId      int64       `json:"tenantId"      orm:"tenant_id"       ` //
	AccountId     int64       `json:"accountId"     orm:"account_id"      ` //
	TelegramBotId int64       `json:"telegramBotId" orm:"telegram_bot_id" ` //
	BotName       string      `json:"botName"       orm:"bot_name"        ` //
	BotUsername   string      `json:"botUsername"   orm:"bot_username"    ` //
	BotToken      string      `json:"botToken"      orm:"bot_token"       ` //
	PublishBotId  int64       `json:"publishBotId"  orm:"publish_bot_id"  ` //
	Status        int         `json:"status"        orm:"status"          ` //
	CreatedAt     *gtime.Time `json:"createdAt"     orm:"created_at"      ` //
	UpdatedAt     *gtime.Time `json:"updatedAt"     orm:"updated_at"      ` //
	DeletedAt     *gtime.Time `json:"deletedAt"     orm:"deleted_at"      ` //
	TokenOwnerId  int64       `json:"tokenOwnerId"  orm:"token_owner_id"  ` //
}
