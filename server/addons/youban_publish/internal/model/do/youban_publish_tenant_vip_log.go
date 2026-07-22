// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTenantVipLog is the golang structure of table hg_youban_publish_tenant_vip_log for DAO operations like Where/Data.
type YoubanPublishTenantVipLog struct {
	g.Meta          `orm:"table:hg_youban_publish_tenant_vip_log, do:true"`
	Id              any         //
	TenantId        any         //
	OperatorId      any         //
	Source          any         //
	Action          any         //
	BeforeStatus    any         //
	BeforeLevel     any         //
	BeforeExpiredAt *gtime.Time //
	AfterStatus     any         //
	AfterLevel      any         //
	AfterExpiredAt  *gtime.Time //
	Remark          any         //
	CreatedAt       *gtime.Time //
}
