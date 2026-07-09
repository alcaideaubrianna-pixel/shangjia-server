// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectEventMedia is the golang structure of table hg_youban_publish_collect_event_media for DAO operations like Where/Data.
type YoubanPublishCollectEventMedia struct {
	g.Meta           `orm:"table:hg_youban_publish_collect_event_media, do:true"`
	Id               any         //
	TenantId         any         //
	AccountId        any         //
	SourceId         any         //
	SourceType       any         //
	EventId          any         //
	SourceChatId     any         //
	SourceMessageId  any         //
	SourceGroupedId  any         //
	SourceMediaKey   any         //
	MediaType        any         //
	SourceRefType    any         //
	SourceFileId     any         //
	SourceMessageRef any         //
	BackupChannelId  any         //
	BackupChatId     any         //
	BackupMessageId  any         //
	FileUrl          any         //
	StoragePath      any         //
	PosterUrl        any         //
	MetaJson         any         //
	SortIndex        any         //
	CacheStatus      any         //
	ErrorMessage     any         //
	CreatedAt        *gtime.Time //
	UpdatedAt        *gtime.Time //
}
