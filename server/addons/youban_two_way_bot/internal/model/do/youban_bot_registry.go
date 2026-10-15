// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotRegistry is the golang structure of table hg_youban_bot_registry for DAO operations like Where/Data.
type YoubanBotRegistry struct {
	g.Meta               `orm:"table:hg_youban_bot_registry, do:true"`
	Id                   any         //
	TenantId             any         //
	AccountId            any         //
	BotType              any         //
	BotName              any         //
	BotUsername          any         //
	BotToken             any         //
	BotUserId            any         //
	IsOfficial           any         //
	IsDefault            any         //
	RunMode              any         //
	WebhookUrl           any         //
	Remark               any         //
	Status               any         //
	CreatedBy            any         //
	UpdatedBy            any         //
	DeletedBy            any         //
	TgAccountId          any         //
	WelcomeMessage       any         //
	SupergroupId         any         //
	SupergroupAccessHash any         //
	SupergroupTitle      any         //
	InviteLink           any         //
	SetupStatus          any         //
	WebhookStatus        any         //
	ErrorMessage         any         //
	LastSetupAt          *gtime.Time //
	LastWebhookAt        *gtime.Time //
	CreatedAt            *gtime.Time //
	UpdatedAt            *gtime.Time //
	DeletedAt            *gtime.Time //
}
