// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishImportMatchRun is the golang structure of table hg_youban_publish_import_match_run for DAO operations like Where/Data.
type YoubanPublishImportMatchRun struct {
	g.Meta         `orm:"table:hg_youban_publish_import_match_run, do:true"`
	Id             any         //
	ImportRunId    any         //
	TenantId       any         //
	AccountId      any         //
	Status         any         //
	Stage          any         //
	ChannelIdJson  any         //
	ScanDays       any         //
	Threshold      any         //
	ProfileTotal   any         //
	ProfileDone    any         //
	CandidateTotal any         //
	AutoMatched    any         //
	ManualPending  any         //
	Confirmed      any         //
	Skipped        any         //
	ErrorMessage   any         //
	CreatedAt      *gtime.Time //
	UpdatedAt      *gtime.Time //
	FinishedAt     *gtime.Time //
	DeletedAt      *gtime.Time //
}
