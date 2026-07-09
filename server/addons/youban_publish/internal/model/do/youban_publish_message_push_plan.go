// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishMessagePushPlan is the golang structure of table hg_youban_publish_message_push_plan for DAO operations like Where/Data.
type YoubanPublishMessagePushPlan struct {
	g.Meta          `orm:"table:hg_youban_publish_message_push_plan, do:true"`
	Id              any         //
	TenantId        any         //
	Name            any         //
	AccountId       any         //
	TemplateIds     any         //
	TargetChatIds   any         //
	Times           any         //
	IntervalSeconds any         //
	Status          any         //
	NextRunAt       *gtime.Time //
	LastRunAt       *gtime.Time //
	LastResult      any         //
	LockedAt        *gtime.Time //
	CreatedBy       any         //
	UpdatedBy       any         //
	DeletedBy       any         //
	CreatedAt       *gtime.Time //
	UpdatedAt       *gtime.Time //
	DeletedAt       *gtime.Time //
}
