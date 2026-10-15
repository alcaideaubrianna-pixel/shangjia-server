// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotRelayBinding is the golang structure for table youban_bot_relay_binding.
type YoubanBotRelayBinding struct {
	Id          int64       `json:"id"          orm:"id"            ` //
	BotId       int64       `json:"botId"       orm:"bot_id"        ` //
	PairKey     string      `json:"pairKey"     orm:"pair_key"      ` //
	LeftChatId  string      `json:"leftChatId"  orm:"left_chat_id"  ` //
	RightChatId string      `json:"rightChatId" orm:"right_chat_id" ` //
	LeftName    string      `json:"leftName"    orm:"left_name"     ` //
	RightName   string      `json:"rightName"   orm:"right_name"    ` //
	CreatedAt   *gtime.Time `json:"createdAt"   orm:"created_at"    ` //
	UpdatedAt   *gtime.Time `json:"updatedAt"   orm:"updated_at"    ` //
}
