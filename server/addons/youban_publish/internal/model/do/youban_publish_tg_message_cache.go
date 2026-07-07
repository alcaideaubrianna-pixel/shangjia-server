// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTgMessageCache is the golang structure of table hg_youban_publish_tg_message_cache for DAO operations like Where/Data.
type YoubanPublishTgMessageCache struct {
	g.Meta       `orm:"table:hg_youban_publish_tg_message_cache, do:true"`
	Id           any         //
	TenantId     any         //
	TgAccountId  any         //
	ChannelId    any         //
	TargetChatId any         //
	TgMessageId  any         //
	MessageText  any         //
	MessageDate  *gtime.Time //
	MediaGroupId any         //
	CreatedAt    *gtime.Time //
	UpdatedAt    *gtime.Time //
}
