// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotBot is the golang structure of table hg_youban_bot_bot for DAO operations like Where/Data.
type YoubanBotBot struct {
	g.Meta      `orm:"table:hg_youban_bot_bot, do:true"`
	Id          any         //
	BotName     any         //
	BotUsername any         //
	BotToken    any         //
	IsOfficial  any         //
	IsDefault   any         //
	RunMode     any         //
	WebhookUrl  any         //
	Remark      any         //
	Status      any         //
	CreatedBy   any         //
	UpdatedBy   any         //
	DeletedBy   any         //
	CreatedAt   *gtime.Time //
	UpdatedAt   *gtime.Time //
	DeletedAt   *gtime.Time //
}
