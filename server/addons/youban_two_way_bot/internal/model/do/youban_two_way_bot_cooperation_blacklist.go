// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanTwoWayBotCooperationBlacklist is the golang structure of table hg_youban_two_way_bot_cooperation_blacklist for DAO operations like Where/Data.
type YoubanTwoWayBotCooperationBlacklist struct {
	g.Meta             `orm:"table:hg_youban_two_way_bot_cooperation_blacklist, do:true"`
	Id                 any         //
	TenantId           any         //
	ConfigId           any         //
	ApplicantTgUserId  any         //
	ApplicantUsername  any         //
	ApplicantFirstName any         //
	ApplicantLastName  any         //
	Reason             any         //
	Status             any         //
	CreatedBy          any         //
	UpdatedBy          any         //
	CreatedAt          *gtime.Time //
	UpdatedAt          *gtime.Time //
}
