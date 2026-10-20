// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotManagerState is the golang structure of table hg_youban_bot_manager_state for DAO operations like Where/Data.
type YoubanBotManagerState struct {
	g.Meta             `orm:"table:hg_youban_bot_manager_state, do:true"`
	Id                 any         //
	TenantId           any         //
	AccountId          any         //
	EntryBotId         any         //
	TelegramUserId     any         //
	SelectedChannelIds any         //
	SelectionSaved     any         //
	InputMode          any         //
	Keyword            any         //
	DraftBotId         any         //
	Revision           any         //
	SyncedAt           *gtime.Time //
	CreatedAt          *gtime.Time //
	UpdatedAt          *gtime.Time //
	DraftRightsJson    any         //
}
