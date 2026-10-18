// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotRelayEndpoint is the golang structure of table hg_youban_bot_relay_endpoint for DAO operations like Where/Data.
type YoubanBotRelayEndpoint struct {
	g.Meta    `orm:"table:hg_youban_bot_relay_endpoint, do:true"`
	Id        any         //
	BotId     any         //
	BindingId any         //
	ChatId    any         //
	CreatedAt *gtime.Time //
	UpdatedAt *gtime.Time //
}
