// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectChannel is the golang structure of table hg_youban_publish_collect_channel for DAO operations like Where/Data.
type YoubanPublishCollectChannel struct {
	g.Meta         `orm:"table:hg_youban_publish_collect_channel, do:true"`
	Id             any         //
	TenantId       any         //
	TelegramChatId any         //
	ChatType       any         //
	Title          any         //
	Username       any         //
	InviteLink     any         //
	About          any         //
	PhotoFileId    any         //
	IsBroadcast    any         //
	IsMegagroup    any         //
	LastMessageId  any         //
	LastMessageAt  *gtime.Time //
	LastSeenAt     *gtime.Time //
	Status         any         //
	CreatedAt      *gtime.Time //
	UpdatedAt      *gtime.Time //
	DeletedAt      *gtime.Time //
}
