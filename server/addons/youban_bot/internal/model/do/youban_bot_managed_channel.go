// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotManagedChannel is the golang structure of table hg_youban_bot_managed_channel for DAO operations like Where/Data.
type YoubanBotManagedChannel struct {
	g.Meta           `orm:"table:hg_youban_bot_managed_channel, do:true"`
	Id               any         //
	TenantId         any         //
	AccountId        any         //
	PublishChannelId any         //
	TgAccountId      any         //
	ChatId           any         //
	Title            any         //
	AccessHash       any         //
	VisitUrl         any         //
	AdminCount       any         //
	AdminLimit       any         //
	BotCount         any         //
	CanManage        any         //
	SyncStatus       any         //
	ErrorMessage     any         //
	LastSyncedAt     *gtime.Time //
	LastAssignedAt   *gtime.Time //
	CreatedAt        *gtime.Time //
	UpdatedAt        *gtime.Time //
}
