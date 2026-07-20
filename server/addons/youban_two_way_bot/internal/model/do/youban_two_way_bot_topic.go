// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanTwoWayBotTopic is the golang structure of table hg_youban_two_way_bot_topic for DAO operations like Where/Data.
type YoubanTwoWayBotTopic struct {
	g.Meta            `orm:"table:hg_youban_two_way_bot_topic, do:true"`
	Id                any         //
	TenantId          any         //
	BotId             any         //
	TelegramUserId    any         //
	TelegramUsername  any         //
	TelegramFirstName any         //
	TelegramLastName  any         //
	ThreadId          any         //
	Title             any         //
	Closed            any         //
	LastMessageAt     *gtime.Time //
	CreatedAt         *gtime.Time //
	UpdatedAt         *gtime.Time //
	DeletedAt         *gtime.Time //
}
