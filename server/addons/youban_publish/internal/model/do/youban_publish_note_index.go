// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishNoteIndex is the golang structure of table hg_youban_publish_note_index for DAO operations like Where/Data.
type YoubanPublishNoteIndex struct {
	g.Meta          `orm:"table:hg_youban_publish_note_index, do:true"`
	Id              any         //
	TenantId        any         //
	AccountId       any         //
	ProfileId       any         //
	TaskId          any         //
	Uuid            any         //
	ProfileNo       any         //
	Title           any         //
	Summary         any         //
	PlainText       any         //
	Tag             any         //
	Province        any         //
	City            any         //
	Status          any         //
	Visibility      any         //
	ReviewStatus    any         //
	TaskStatus      any         //
	CoverMediaId    any         //
	PublishedAt     *gtime.Time //
	SourceUpdatedAt *gtime.Time //
	CreatedAt       *gtime.Time //
	UpdatedAt       *gtime.Time //
	DeletedAt       *gtime.Time //
}
