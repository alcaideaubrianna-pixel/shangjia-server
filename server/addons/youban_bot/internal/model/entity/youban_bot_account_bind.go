// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotAccountBind is the golang structure for table youban_bot_account_bind.
type YoubanBotAccountBind struct {
	Id                int64       `json:"id"                orm:"id"                  ` //
	App               string      `json:"app"               orm:"app"                 ` //
	AccountId         int64       `json:"accountId"         orm:"account_id"          ` //
	TelegramUserId    string      `json:"telegramUserId"    orm:"telegram_user_id"    ` //
	TelegramUsername  string      `json:"telegramUsername"  orm:"telegram_username"   ` //
	TelegramFirstName string      `json:"telegramFirstName" orm:"telegram_first_name" ` //
	TelegramLastName  string      `json:"telegramLastName"  orm:"telegram_last_name"  ` //
	BotId             int64       `json:"botId"             orm:"bot_id"              ` //
	Status            int         `json:"status"            orm:"status"              ` //
	CreatedAt         *gtime.Time `json:"createdAt"         orm:"created_at"          ` //
	UpdatedAt         *gtime.Time `json:"updatedAt"         orm:"updated_at"          ` //
	DeletedAt         *gtime.Time `json:"deletedAt"         orm:"deleted_at"          ` //
}
