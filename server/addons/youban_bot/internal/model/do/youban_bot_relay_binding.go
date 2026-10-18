// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotRelayBinding is the golang structure of table hg_youban_bot_relay_binding for DAO operations like Where/Data.
type YoubanBotRelayBinding struct {
	g.Meta      `orm:"table:hg_youban_bot_relay_binding, do:true"`
	Id          any         //
	BotId       any         //
	PairKey     any         //
	LeftChatId  any         //
	RightChatId any         //
	LeftName    any         //
	RightName   any         //
	CreatedAt   *gtime.Time //
	UpdatedAt   *gtime.Time //
}
