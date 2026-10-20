// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishProfileOrigin is the golang structure of table hg_youban_publish_profile_origin for DAO operations like Where/Data.
type YoubanPublishProfileOrigin struct {
	g.Meta          `orm:"table:hg_youban_publish_profile_origin, do:true"`
	Id              any         //
	TenantId        any         //
	AccountId       any         //
	ProfileId       any         //
	OriginKey       any         //
	OriginKind      any         //
	CollectMethod   any         //
	SourceId        any         //
	EventId         any         //
	BotId           any         //
	TgAccountId     any         //
	FollowAccountId any         //
	ChatIdRaw       any         //
	ChatId          any         //
	MessageId       any         //
	GroupedId       any         //
	ChannelTitle    any         //
	ChannelUsername any         //
	ReceivedAt      *gtime.Time //
	EvidenceKind    any         //
	Completeness    any         //
	CreatedAt       *gtime.Time //
}
