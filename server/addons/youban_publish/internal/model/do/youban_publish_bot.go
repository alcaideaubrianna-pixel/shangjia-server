// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishBot is the golang structure of table hg_youban_publish_bot for DAO operations like Where/Data.
type YoubanPublishBot struct {
	g.Meta      `orm:"table:hg_youban_publish_bot, do:true"`
	Id          any         //
	BotName     any         //
	BotUsername any         //
	BotToken    any         //
	Remark      any         //
	Status      any         //
	CreatedBy   any         //
	UpdatedBy   any         //
	DeletedBy   any         //
	CreatedAt   *gtime.Time //
	UpdatedAt   *gtime.Time //
	DeletedAt   *gtime.Time //
	TenantId    any         //
}
