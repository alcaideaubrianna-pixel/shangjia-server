// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotRelayAdmin is the golang structure for table youban_bot_relay_admin.
type YoubanBotRelayAdmin struct {
	Id        int64       `json:"id"        orm:"id"         ` //
	BotId     int64       `json:"botId"     orm:"bot_id"     ` //
	UserId    string      `json:"userId"    orm:"user_id"    ` //
	Username  string      `json:"username"  orm:"username"   ` //
	CreatedAt *gtime.Time `json:"createdAt" orm:"created_at" ` //
	UpdatedAt *gtime.Time `json:"updatedAt" orm:"updated_at" ` //
}
