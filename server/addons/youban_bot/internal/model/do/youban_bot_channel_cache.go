// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotChannelCache is the golang structure of table hg_youban_bot_channel_cache for DAO operations like Where/Data.
type YoubanBotChannelCache struct {
	g.Meta          `orm:"table:hg_youban_bot_channel_cache, do:true"`
	Id              any         //
	BotId           any         //
	ChatId          any         //
	ChatType        any         //
	ChatTitle       any         //
	ChatUsername    any         //
	IsBroadcast     any         //
	IsMegagroup     any         //
	MessageCount    any         //
	LastMessageText any         //
	LastMessageAt   *gtime.Time //
	CreatedAt       *gtime.Time //
	UpdatedAt       *gtime.Time //
}
