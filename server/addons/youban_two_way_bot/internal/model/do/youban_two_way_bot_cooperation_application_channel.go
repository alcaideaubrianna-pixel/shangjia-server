// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanTwoWayBotCooperationApplicationChannel is the golang structure of table hg_youban_two_way_bot_cooperation_application_channel for DAO operations like Where/Data.
type YoubanTwoWayBotCooperationApplicationChannel struct {
	g.Meta        `orm:"table:hg_youban_two_way_bot_cooperation_application_channel, do:true"`
	Id            any         //
	TenantId      any         //
	ApplicationId any         //
	ChannelId     any         //
	Status        any         //
	ErrorMessage  any         //
	RetryCount    any         //
	JoinedAt      *gtime.Time //
	CreatedAt     *gtime.Time //
	UpdatedAt     *gtime.Time //
}
