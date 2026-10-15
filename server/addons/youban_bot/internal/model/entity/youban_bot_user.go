// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotUser is the golang structure for table youban_bot_user.
type YoubanBotUser struct {
	Id                int64       `json:"id"                orm:"id"                  ` //
	BotId             int64       `json:"botId"             orm:"bot_id"              ` //
	TelegramUserId    string      `json:"telegramUserId"    orm:"telegram_user_id"    ` //
	TelegramUsername  string      `json:"telegramUsername"  orm:"telegram_username"   ` //
	TelegramFirstName string      `json:"telegramFirstName" orm:"telegram_first_name" ` //
	TelegramLastName  string      `json:"telegramLastName"  orm:"telegram_last_name"  ` //
	ChatId            string      `json:"chatId"            orm:"chat_id"             ` //
	ChatType          string      `json:"chatType"          orm:"chat_type"           ` //
	ChatTitle         string      `json:"chatTitle"         orm:"chat_title"          ` //
	MessageCount      int         `json:"messageCount"      orm:"message_count"       ` //
	LastMessageText   string      `json:"lastMessageText"   orm:"last_message_text"   ` //
	LastMessageAt     *gtime.Time `json:"lastMessageAt"     orm:"last_message_at"     ` //
	Status            int         `json:"status"            orm:"status"              ` //
	IsSuperAdmin      int         `json:"isSuperAdmin"      orm:"is_super_admin"      ` //
	CreatedAt         *gtime.Time `json:"createdAt"         orm:"created_at"          ` //
	UpdatedAt         *gtime.Time `json:"updatedAt"         orm:"updated_at"          ` //
}
