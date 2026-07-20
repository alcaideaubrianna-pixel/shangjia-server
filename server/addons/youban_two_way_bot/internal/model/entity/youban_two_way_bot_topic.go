// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanTwoWayBotTopic is the golang structure for table youban_two_way_bot_topic.
type YoubanTwoWayBotTopic struct {
	Id                int64       `json:"id"                orm:"id"                  description:""`
	TenantId          int64       `json:"tenantId"          orm:"tenant_id"           description:""`
	BotId             int64       `json:"botId"             orm:"bot_id"              description:""`
	TelegramUserId    string      `json:"telegramUserId"    orm:"telegram_user_id"    description:""`
	TelegramUsername  string      `json:"telegramUsername"  orm:"telegram_username"   description:""`
	TelegramFirstName string      `json:"telegramFirstName" orm:"telegram_first_name" description:""`
	TelegramLastName  string      `json:"telegramLastName"  orm:"telegram_last_name"  description:""`
	ThreadId          int64       `json:"threadId"          orm:"thread_id"           description:""`
	Title             string      `json:"title"             orm:"title"               description:""`
	Closed            int         `json:"closed"            orm:"closed"              description:""`
	LastMessageAt     *gtime.Time `json:"lastMessageAt"     orm:"last_message_at"     description:""`
	CreatedAt         *gtime.Time `json:"createdAt"         orm:"created_at"          description:""`
	UpdatedAt         *gtime.Time `json:"updatedAt"         orm:"updated_at"          description:""`
	DeletedAt         *gtime.Time `json:"deletedAt"         orm:"deleted_at"          description:""`
}
