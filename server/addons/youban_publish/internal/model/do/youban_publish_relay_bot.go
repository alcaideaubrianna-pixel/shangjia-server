// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishRelayBot is the golang structure of table hg_youban_publish_relay_bot for DAO operations like Where/Data.
type YoubanPublishRelayBot struct {
	g.Meta         `orm:"table:hg_youban_publish_relay_bot, do:true"`
	Id             any         //
	TenantId       any         //
	RelayChannelId any         //
	BotId          any         //
	TelegramUserId any         //
	Status         any         //
	CheckedAt      *gtime.Time //
	LastError      any         //
	CreatedAt      *gtime.Time //
	UpdatedAt      *gtime.Time //
}
