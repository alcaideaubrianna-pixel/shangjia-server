// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishMaterialImportTask is the golang structure of table hg_youban_publish_material_import_task for DAO operations like Where/Data.
type YoubanPublishMaterialImportTask struct {
	g.Meta         `orm:"table:hg_youban_publish_material_import_task, do:true"`
	Id             any         //
	TenantId       any         //
	AccountId      any         //
	TgAccountId    any         //
	SourceChatId   any         //
	SourceTitle    any         //
	SourceUsername any         //
	Status         any         //
	Stage          any         //
	PullOffsetId   any         //
	PullLimitDays  any         //
	MessageTotal   any         //
	MessageDone    any         //
	GroupTotal     any         //
	GroupDone      any         //
	MediaTotal     any         //
	MediaDone      any         //
	MediaFailed    any         //
	Imported       any         //
	Duplicate      any         //
	ErrorMessage   any         //
	NextRunAt      *gtime.Time //
	ResultJson     any         //
	CreatedBy      any         //
	UpdatedBy      any         //
	StartedAt      *gtime.Time //
	FinishedAt     *gtime.Time //
	CreatedAt      *gtime.Time //
	UpdatedAt      *gtime.Time //
}
