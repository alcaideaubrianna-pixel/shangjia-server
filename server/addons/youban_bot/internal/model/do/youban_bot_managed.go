// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotManaged is the golang structure of table hg_youban_bot_managed for DAO operations like Where/Data.
type YoubanBotManaged struct {
	g.Meta        `orm:"table:hg_youban_bot_managed, do:true"`
	Id            any         //
	TenantId      any         //
	AccountId     any         //
	TelegramBotId any         //
	BotName       any         //
	BotUsername   any         //
	BotToken      any         //
	PublishBotId  any         //
	Status        any         //
	CreatedAt     *gtime.Time //
	UpdatedAt     *gtime.Time //
	DeletedAt     *gtime.Time //
	TokenOwnerId  any         //
}
