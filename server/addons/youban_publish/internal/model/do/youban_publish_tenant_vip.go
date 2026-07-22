// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTenantVip is the golang structure of table hg_youban_publish_tenant_vip for DAO operations like Where/Data.
type YoubanPublishTenantVip struct {
	g.Meta    `orm:"table:hg_youban_publish_tenant_vip, do:true"`
	Id        any         //
	TenantId  any         //
	Level     any         //
	Status    any         //
	OpenedAt  *gtime.Time //
	ExpiredAt *gtime.Time //
	Remark    any         //
	CreatedAt *gtime.Time //
	UpdatedAt *gtime.Time //
	DeletedAt *gtime.Time //
}
