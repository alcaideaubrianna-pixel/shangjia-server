// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotAccountBind is the golang structure of table hg_youban_bot_account_bind for DAO operations like Where/Data.
type YoubanBotAccountBind struct {
	g.Meta            `orm:"table:hg_youban_bot_account_bind, do:true"`
	Id                any         //
	App               any         //
	AccountId         any         //
	TelegramUserId    any         //
	TelegramUsername  any         //
	TelegramFirstName any         //
	TelegramLastName  any         //
	BotId             any         //
	Status            any         //
	CreatedAt         *gtime.Time //
	UpdatedAt         *gtime.Time //
	DeletedAt         *gtime.Time //
}
