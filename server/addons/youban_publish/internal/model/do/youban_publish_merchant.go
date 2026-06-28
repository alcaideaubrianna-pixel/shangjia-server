// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishMerchant is the golang structure of table hg_youban_publish_merchant for DAO operations like Where/Data.
type YoubanPublishMerchant struct {
	g.Meta       `orm:"table:hg_youban_publish_merchant, do:true"`
	Id           any         //
	Name         any         //
	ContactName  any         //
	ContactPhone any         //
	Remark       any         //
	Status       any         //
	CreatedBy    any         //
	UpdatedBy    any         //
	DeletedBy    any         //
	CreatedAt    *gtime.Time //
	UpdatedAt    *gtime.Time //
	DeletedAt    *gtime.Time //
}
