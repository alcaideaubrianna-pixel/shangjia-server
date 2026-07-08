// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishImportMatchCandidate is the golang structure of table hg_youban_publish_import_match_candidate for DAO operations like Where/Data.
type YoubanPublishImportMatchCandidate struct {
	g.Meta         `orm:"table:hg_youban_publish_import_match_candidate, do:true"`
	Id             any         //
	MatchRunId     any         //
	TenantId       any         //
	ChannelId      any         //
	GroupKey       any         //
	MediaGroupId   any         //
	FirstMessageId any         //
	LastMessageId  any         //
	MessageDate    *gtime.Time //
	CaptionText    any         //
	MediaCount     any         //
	MediaTypes     any         //
	PreviewJson    any         //
	CreatedAt      *gtime.Time //
	UpdatedAt      *gtime.Time //
	DeletedAt      *gtime.Time //
}
