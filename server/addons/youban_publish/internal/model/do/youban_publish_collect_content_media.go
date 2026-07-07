// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectContentMedia is the golang structure of table hg_youban_publish_collect_content_media for DAO operations like Where/Data.
type YoubanPublishCollectContentMedia struct {
	g.Meta          `orm:"table:hg_youban_publish_collect_content_media, do:true"`
	Id              any         //
	TenantId        any         //
	AccountId       any         //
	ContentId       any         //
	MediaType       any         //
	SourceFileId    any         //
	SourceUniqueKey any         //
	FileMd5         any         //
	FilePhash       any         //
	SortIndex       any         //
	Status          any         //
	CreatedAt       *gtime.Time //
	UpdatedAt       *gtime.Time //
}
