// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectAccountChannel is the golang structure of table hg_youban_publish_collect_account_channel for DAO operations like Where/Data.
type YoubanPublishCollectAccountChannel struct {
	g.Meta         `orm:"table:hg_youban_publish_collect_account_channel, do:true"`
	Id             any         //
	TenantId       any         //
	AccountId      any         //
	ChannelId      any         //
	SourceId       any         //
	CollectEnabled any         //
	Status         any         //
	ApprovalStatus any         //
	ApprovedAt     *gtime.Time //
	DisabledAt     *gtime.Time //
	DisabledReason any         //
	CreatedAt      *gtime.Time //
	UpdatedAt      *gtime.Time //
	DeletedAt      *gtime.Time //
}
