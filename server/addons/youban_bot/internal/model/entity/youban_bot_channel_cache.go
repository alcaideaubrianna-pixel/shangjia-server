// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotChannelCache is the golang structure for table youban_bot_channel_cache.
type YoubanBotChannelCache struct {
	Id              int64       `json:"id"              orm:"id"                ` //
	BotId           int64       `json:"botId"           orm:"bot_id"            ` //
	ChatId          string      `json:"chatId"          orm:"chat_id"           ` //
	ChatType        string      `json:"chatType"        orm:"chat_type"         ` //
	ChatTitle       string      `json:"chatTitle"       orm:"chat_title"        ` //
	ChatUsername    string      `json:"chatUsername"    orm:"chat_username"     ` //
	IsBroadcast     int         `json:"isBroadcast"     orm:"is_broadcast"      ` //
	IsMegagroup     int         `json:"isMegagroup"     orm:"is_megagroup"      ` //
	MessageCount    int         `json:"messageCount"    orm:"message_count"     ` //
	LastMessageText string      `json:"lastMessageText" orm:"last_message_text" ` //
	LastMessageAt   *gtime.Time `json:"lastMessageAt"   orm:"last_message_at"   ` //
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"        ` //
	UpdatedAt       *gtime.Time `json:"updatedAt"       orm:"updated_at"        ` //
}
