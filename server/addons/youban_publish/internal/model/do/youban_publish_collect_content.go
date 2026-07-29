// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectContent is the golang structure of table hg_youban_publish_collect_content for DAO operations like Where/Data.
type YoubanPublishCollectContent struct {
	g.Meta         `orm:"table:hg_youban_publish_collect_content, do:true"`
	Id             any         //
	TenantId       any         //
	AccountId      any         //
	FirstEventId   any         //
	LastEventId    any         //
	SourceType     any         //
	RawText        any         //
	NormalizedText any         //
	MediaCount     any         //
	MediaJson      any         //
	TextHash       any         //
	DedupeKey      any         //
	DuplicateTotal any         //
	Status         any         //
	FirstSeenAt    *gtime.Time //
	PreviousSeenAt *gtime.Time //
	LastSeenAt     *gtime.Time //
	CreatedAt      *gtime.Time //
	UpdatedAt      *gtime.Time //
}
