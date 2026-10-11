// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectSource is the golang structure of table hg_youban_publish_collect_source for DAO operations like Where/Data.
type YoubanPublishCollectSource struct {
	g.Meta             `orm:"table:hg_youban_publish_collect_source, do:true"`
	Id                 any         //
	TenantId           any         //
	AccountId          any         //
	SourceType         any         //
	Title              any         //
	SourceChatId       any         //
	SourceUsername     any         //
	TgAccountId        any         //
	BotId              any         //
	BotCollectScope    any         //
	FollowAccountId    any         //
	CollectEnabled     any         //
	Status             any         //
	EventTotal         any         //
	SuccessTotal       any         //
	FailedTotal        any         //
	LastEventAt        *gtime.Time //
	Remark             any         //
	CreatedBy          any         //
	UpdatedBy          any         //
	DeletedBy          any         //
	CreatedAt          *gtime.Time //
	UpdatedAt          *gtime.Time //
	DeletedAt          *gtime.Time //
	ChannelId          any         //
	ChannelBlacklisted any         //
	ChannelSuffix      any         //
}
