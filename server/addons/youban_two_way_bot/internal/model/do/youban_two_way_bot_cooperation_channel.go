// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanTwoWayBotCooperationChannel is the golang structure of table hg_youban_two_way_bot_cooperation_channel for DAO operations like Where/Data.
type YoubanTwoWayBotCooperationChannel struct {
	g.Meta    `orm:"table:hg_youban_two_way_bot_cooperation_channel, do:true"`
	Id        any         //
	TenantId  any         //
	ConfigId  any         //
	ChannelId any         //
	Status    any         //
	CreatedAt *gtime.Time //
	UpdatedAt *gtime.Time //
	DeletedAt *gtime.Time //
}
