// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotFeature is the golang structure of table hg_youban_bot_feature for DAO operations like Where/Data.
type YoubanBotFeature struct {
	g.Meta      `orm:"table:hg_youban_bot_feature, do:true"`
	Id          any         //
	FeatureKey  any         //
	Name        any         //
	Command     any         //
	Description any         //
	ConfigJson  any         //
	Sort        any         //
	Status      any         //
	CreatedAt   *gtime.Time //
	UpdatedAt   *gtime.Time //
	DeletedAt   *gtime.Time //
}
