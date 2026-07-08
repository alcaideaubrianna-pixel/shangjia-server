// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectHistoryTask is the golang structure of table hg_youban_publish_collect_history_task for DAO operations like Where/Data.
type YoubanPublishCollectHistoryTask struct {
	g.Meta         `orm:"table:hg_youban_publish_collect_history_task, do:true"`
	Id             any         //
	TenantId       any         //
	AccountId      any         //
	SourceId       any         //
	TgAccountId    any         //
	SourceChatId   any         //
	Mode           any         //
	Days           any         //
	OffsetId       any         //
	ScannedCount   any         //
	EventCount     any         //
	DuplicateCount any         //
	FailedCount    any         //
	Status         any         //
	ErrorMessage   any         //
	NextRunAt      *gtime.Time //
	StartedAt      *gtime.Time //
	FinishedAt     *gtime.Time //
	CreatedAt      *gtime.Time //
	UpdatedAt      *gtime.Time //
}
