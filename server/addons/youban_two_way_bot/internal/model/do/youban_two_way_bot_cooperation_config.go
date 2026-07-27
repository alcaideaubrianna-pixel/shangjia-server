// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanTwoWayBotCooperationConfig is the golang structure of table hg_youban_two_way_bot_cooperation_config for DAO operations like Where/Data.
type YoubanTwoWayBotCooperationConfig struct {
	g.Meta           `orm:"table:hg_youban_two_way_bot_cooperation_config, do:true"`
	Id               any         //
	TenantId         any         //
	AccountId        any         //
	BotId            any         //
	TwoWayBotId      any         //
	NotificationType any         //
	ReviewRequired   any         //
	Status           any         //
	CreatedBy        any         //
	UpdatedBy        any         //
	CreatedAt        *gtime.Time //
	UpdatedAt        *gtime.Time //
	DeletedAt        *gtime.Time //
}
