// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgMessageRepairRun is the golang structure of table hg_youban_publish_tg_message_repair_run for DAO operations like Where/Data.
type YoubanPublishTgMessageRepairRun struct {
	g.Meta       `orm:"table:hg_youban_publish_tg_message_repair_run, do:true"`
	Id           any         //
	TenantId     any         //
	AccountId    any         //
	ProfileId    any         //
	TaskId       any         //
	Status       any         //
	Stage        any         //
	Progress     any         //
	ChannelCount any         //
	ScannedCount any         //
	MatchedCount any         //
	ErrorMessage any         //
	CreatedAt    *gtime.Time //
	UpdatedAt    *gtime.Time //
	FinishedAt   *gtime.Time //
}
