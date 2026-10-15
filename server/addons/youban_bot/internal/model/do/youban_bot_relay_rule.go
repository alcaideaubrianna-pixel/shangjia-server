// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotRelayRule is the golang structure of table hg_youban_bot_relay_rule for DAO operations like Where/Data.
type YoubanBotRelayRule struct {
	g.Meta    `orm:"table:hg_youban_bot_relay_rule, do:true"`
	Id        any         //
	BotId     any         //
	Kind      any         //
	Value     any         //
	Label     any         //
	CreatedAt *gtime.Time //
	UpdatedAt *gtime.Time //
}
