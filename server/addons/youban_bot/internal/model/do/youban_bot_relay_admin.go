// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotRelayAdmin is the golang structure of table hg_youban_bot_relay_admin for DAO operations like Where/Data.
type YoubanBotRelayAdmin struct {
	g.Meta    `orm:"table:hg_youban_bot_relay_admin, do:true"`
	Id        any         //
	BotId     any         //
	UserId    any         //
	Username  any         //
	CreatedAt *gtime.Time //
	UpdatedAt *gtime.Time //
}
