// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanTwoWayBotMessage is the golang structure of table hg_youban_two_way_bot_message for DAO operations like Where/Data.
type YoubanTwoWayBotMessage struct {
	g.Meta          `orm:"table:hg_youban_two_way_bot_message, do:true"`
	Id              any         //
	TenantId        any         //
	BotId           any         //
	Direction       any         //
	TelegramUserId  any         //
	ThreadId        any         //
	SourceChatId    any         //
	SourceMessageId any         //
	TargetChatId    any         //
	TargetMessageId any         //
	MediaGroupId    any         //
	Status          any         //
	ErrorMessage    any         //
	CreatedAt       *gtime.Time //
	UpdatedAt       *gtime.Time //
}
