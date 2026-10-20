// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotManagedMember is the golang structure of table hg_youban_bot_managed_member for DAO operations like Where/Data.
type YoubanBotManagedMember struct {
	g.Meta       `orm:"table:hg_youban_bot_managed_member, do:true"`
	Id           any         //
	TenantId     any         //
	AccountId    any         //
	ManagedBotId any         //
	ChannelId    any         //
	MemberStatus any         //
	RightsJson   any         //
	LastSyncedAt *gtime.Time //
	CreatedAt    *gtime.Time //
	UpdatedAt    *gtime.Time //
}
