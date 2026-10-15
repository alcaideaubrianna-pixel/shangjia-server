// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotRelayDelivery is the golang structure of table hg_youban_bot_relay_delivery for DAO operations like Where/Data.
type YoubanBotRelayDelivery struct {
	g.Meta          `orm:"table:hg_youban_bot_relay_delivery, do:true"`
	Id              any         //
	BotId           any         //
	BindingId       any         //
	SourceChatId    any         //
	SourceMessageId any         //
	MediaGroupId    any         //
	TargetChatId    any         //
	Kind            any         //
	PairKey         any         //
	TopicTitle      any         //
	RawJson         any         //
	Status          any         //
	Attempts        any         //
	AvailableAt     *gtime.Time //
	LeaseToken      any         //
	TargetMessageId any         //
	ErrorMessage    any         //
	CreatedAt       *gtime.Time //
	UpdatedAt       *gtime.Time //
}
