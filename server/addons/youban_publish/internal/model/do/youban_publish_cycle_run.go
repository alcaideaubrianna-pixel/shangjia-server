// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCycleRun is the golang structure of table hg_youban_publish_cycle_run for DAO operations like Where/Data.
type YoubanPublishCycleRun struct {
	g.Meta       `orm:"table:hg_youban_publish_cycle_run, do:true"`
	Id           any         //
	PlanId       any         //
	TenantId     any         //
	AccountId    any         //
	ProfileId    any         //
	TaskId       any         //
	Status       any         //
	Stage        any         //
	ScheduledAt  *gtime.Time //
	StartedAt    *gtime.Time //
	FinishedAt   *gtime.Time //
	ErrorMessage any         //
	RetryCount   any         //
	CreatedAt    *gtime.Time //
	UpdatedAt    *gtime.Time //
}
