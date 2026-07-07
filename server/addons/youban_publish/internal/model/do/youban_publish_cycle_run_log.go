// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCycleRunLog is the golang structure of table hg_youban_publish_cycle_run_log for DAO operations like Where/Data.
type YoubanPublishCycleRunLog struct {
	g.Meta      `orm:"table:hg_youban_publish_cycle_run_log, do:true"`
	Id          any         //
	RunId       any         //
	PlanId      any         //
	TenantId    any         //
	AccountId   any         //
	ProfileId   any         //
	Level       any         //
	Stage       any         //
	Message     any         //
	ContextJson *gjson.Json //
	CreatedAt   *gtime.Time //
}
