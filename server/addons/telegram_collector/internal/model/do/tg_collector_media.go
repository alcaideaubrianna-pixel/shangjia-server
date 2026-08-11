// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// TgCollectorMedia is the golang structure of table hg_tg_collector_media for DAO operations like Where/Data.
type TgCollectorMedia struct {
	g.Meta            `orm:"table:hg_tg_collector_media, do:true"`
	Id                any         //
	TenantId          any         //
	Fingerprint       any         //
	Kind              any         //
	MimeType          any         //
	Size              any         //
	PipelineVersion   any         //
	Status            any         //
	StoragePath       any         //
	PosterStoragePath any         //
	Phash             any         //
	Dhash             any         //
	AttemptCount      any         //
	NextRunAt         *gtime.Time //
	LeaseOwner        any         //
	LeaseUntil        *gtime.Time //
	ErrorMessage      any         //
	CreatedAt         *gtime.Time //
	UpdatedAt         *gtime.Time //
}
