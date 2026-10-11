// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectBotChannel is the golang structure of table hg_youban_publish_collect_bot_channel for DAO operations like Where/Data.
type YoubanPublishCollectBotChannel struct {
	g.Meta        `orm:"table:hg_youban_publish_collect_bot_channel, do:true"`
	Id            any         //
	TenantId      any         //
	BotId         any         //
	ChannelId     any         //
	Status        any         //
	BotRole       any         //
	FirstSeenAt   *gtime.Time //
	LastSeenAt    *gtime.Time //
	RemovedAt     *gtime.Time //
	RemovedReason any         //
	LastUpdateId  any         //
	CreatedAt     *gtime.Time //
	UpdatedAt     *gtime.Time //
	DeletedAt     *gtime.Time //
}
