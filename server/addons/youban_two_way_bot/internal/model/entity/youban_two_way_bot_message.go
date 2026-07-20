// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanTwoWayBotMessage is the golang structure for table youban_two_way_bot_message.
type YoubanTwoWayBotMessage struct {
	Id              int64       `json:"id"              orm:"id"                description:""`
	TenantId        int64       `json:"tenantId"        orm:"tenant_id"         description:""`
	BotId           int64       `json:"botId"           orm:"bot_id"            description:""`
	Direction       string      `json:"direction"       orm:"direction"         description:""`
	TelegramUserId  string      `json:"telegramUserId"  orm:"telegram_user_id"  description:""`
	ThreadId        int64       `json:"threadId"        orm:"thread_id"         description:""`
	SourceChatId    string      `json:"sourceChatId"    orm:"source_chat_id"    description:""`
	SourceMessageId int         `json:"sourceMessageId" orm:"source_message_id" description:""`
	TargetChatId    string      `json:"targetChatId"    orm:"target_chat_id"    description:""`
	TargetMessageId int         `json:"targetMessageId" orm:"target_message_id" description:""`
	MediaGroupId    string      `json:"mediaGroupId"    orm:"media_group_id"    description:""`
	Status          string      `json:"status"          orm:"status"            description:""`
	ErrorMessage    string      `json:"errorMessage"    orm:"error_message"     description:""`
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"        description:""`
	UpdatedAt       *gtime.Time `json:"updatedAt"       orm:"updated_at"        description:""`
}
