// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanTwoWayBotBot is the golang structure of table hg_youban_two_way_bot_bot for DAO operations like Where/Data.
type YoubanTwoWayBotBot struct {
	g.Meta               `orm:"table:hg_youban_two_way_bot_bot, do:true"`
	Id                   any         //
	TenantId             any         //
	AccountId            any         //
	TgAccountId          any         //
	Name                 any         //
	BotToken             any         //
	BotUserId            any         //
	BotUsername          any         //
	SupergroupId         any         //
	SupergroupAccessHash any         //
	SupergroupTitle      any         //
	InviteLink           any         //
	SetupStatus          any         //
	WebhookStatus        any         //
	Status               any         //
	ErrorMessage         any         //
	LastSetupAt          *gtime.Time //
	LastWebhookAt        *gtime.Time //
	CreatedAt            *gtime.Time //
	UpdatedAt            *gtime.Time //
	DeletedAt            *gtime.Time //
}
