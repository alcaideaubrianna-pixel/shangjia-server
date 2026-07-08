// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishImportMatchItem is the golang structure of table hg_youban_publish_import_match_item for DAO operations like Where/Data.
type YoubanPublishImportMatchItem struct {
	g.Meta          `orm:"table:hg_youban_publish_import_match_item, do:true"`
	Id              any         //
	MatchRunId      any         //
	ImportRunId     any         //
	TenantId        any         //
	AccountId       any         //
	ProfileId       any         //
	TaskId          any         //
	ChannelId       any         //
	DisplayGroupKey any         //
	VerifyGroupKey  any         //
	DisplayScore    any         //
	VerifyScore     any         //
	TotalScore      any         //
	MatchStatus     any         //
	MatchMode       any         //
	ReasonJson      any         //
	CreatedAt       *gtime.Time //
	UpdatedAt       *gtime.Time //
	DeletedAt       *gtime.Time //
}
