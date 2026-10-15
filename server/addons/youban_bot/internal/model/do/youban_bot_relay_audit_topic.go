// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotRelayAuditTopic is the golang structure of table hg_youban_bot_relay_audit_topic for DAO operations like Where/Data.
type YoubanBotRelayAuditTopic struct {
	g.Meta    `orm:"table:hg_youban_bot_relay_audit_topic, do:true"`
	Id        any         //
	BotId     any         //
	GroupId   any         //
	PairKey   any         //
	ThreadId  any         //
	CreatedAt *gtime.Time //
	UpdatedAt *gtime.Time //
}
