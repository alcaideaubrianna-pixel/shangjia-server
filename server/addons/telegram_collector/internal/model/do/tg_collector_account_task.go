// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// TgCollectorAccountTask is the golang structure of table hg_tg_collector_account_task for DAO operations like Where/Data.
type TgCollectorAccountTask struct {
	g.Meta              `orm:"table:hg_tg_collector_account_task, do:true"`
	Id                  any         //
	TenantId            any         //
	AccountId           any         //
	TaskType            any         //
	TaskKey             any         //
	Priority            any         //
	Status              any         //
	AttemptCount        any         //
	MaxAttempts         any         //
	NextRunAt           *gtime.Time //
	LeaseOwner          any         //
	LeaseEpoch          any         //
	LeaseUntil          *gtime.Time //
	ErrorMessage        any         //
	CompletedAt         *gtime.Time //
	CreatedAt           *gtime.Time //
	UpdatedAt           *gtime.Time //
	HistoryTaskId       any         //
	MediaOwnerAccountId any         //
	MediaType           any         //
	MediaPurpose        any         //
	SourceFileId        any         //
	FileUrl             any         //
	StoragePath         any         //
	PosterUrl           any         //
	FileMd5             any         //
	FilePhash           any         //
	SourceKind          any         //
	SourceMediaId       any         //
	SourceAccessHash    any         //
	SourceFileReference any         //
	SourceThumbSize     any         //
	SourceMimeType      any         //
	SourceDcId          any         //
	SourceSize          any         //
	DebugMetaText       any         //
	AttachmentId        any         //
	ResultErrorCode     any         //
}
