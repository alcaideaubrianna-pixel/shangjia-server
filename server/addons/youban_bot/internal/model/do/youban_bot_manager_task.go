// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotManagerTask is the golang structure of table hg_youban_bot_manager_task for DAO operations like Where/Data.
type YoubanBotManagerTask struct {
	g.Meta         `orm:"table:hg_youban_bot_manager_task, do:true"`
	Id             any         //
	TenantId       any         //
	AccountId      any         //
	EntryBotId     any         //
	TelegramUserId any         //
	BatchKey       any         //
	Action         any         //
	ManagedBotId   any         //
	ChannelId      any         //
	RightsJson     any         //
	Status         any         //
	AccountTaskId  any         //
	ErrorMessage   any         //
	Notified       any         //
	CreatedAt      *gtime.Time //
	UpdatedAt      *gtime.Time //
}
