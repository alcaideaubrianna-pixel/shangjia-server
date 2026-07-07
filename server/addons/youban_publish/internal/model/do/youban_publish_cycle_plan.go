// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCyclePlan is the golang structure of table hg_youban_publish_cycle_plan for DAO operations like Where/Data.
type YoubanPublishCyclePlan struct {
	g.Meta           `orm:"table:hg_youban_publish_cycle_plan, do:true"`
	Id               any         //
	TenantId         any         //
	AccountId        any         //
	ProfileId        any         //
	TaskId           any         //
	Enabled          any         //
	IntervalSeconds  any         //
	PublishTime      any         //
	NextRunAt        *gtime.Time //
	LastRunAt        *gtime.Time //
	LastRunId        any         //
	Status           any         //
	Source           any         //
	LockedAt         *gtime.Time //
	LastErrorMessage any         //
	CreatedAt        *gtime.Time //
	UpdatedAt        *gtime.Time //
	DeletedAt        *gtime.Time //
}
