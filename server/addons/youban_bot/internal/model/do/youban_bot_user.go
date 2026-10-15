// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotUser is the golang structure of table hg_youban_bot_user for DAO operations like Where/Data.
type YoubanBotUser struct {
	g.Meta            `orm:"table:hg_youban_bot_user, do:true"`
	Id                any         //
	BotId             any         //
	TelegramUserId    any         //
	TelegramUsername  any         //
	TelegramFirstName any         //
	TelegramLastName  any         //
	ChatId            any         //
	ChatType          any         //
	ChatTitle         any         //
	MessageCount      any         //
	LastMessageText   any         //
	LastMessageAt     *gtime.Time //
	Status            any         //
	IsSuperAdmin      any         //
	CreatedAt         *gtime.Time //
	UpdatedAt         *gtime.Time //
}
