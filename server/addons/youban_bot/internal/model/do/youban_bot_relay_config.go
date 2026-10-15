// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanBotRelayConfig is the golang structure of table hg_youban_bot_relay_config for DAO operations like Where/Data.
type YoubanBotRelayConfig struct {
	g.Meta        `orm:"table:hg_youban_bot_relay_config, do:true"`
	Id            any         //
	BotId         any         //
	TenantId      any         //
	AccountId     any         //
	Enabled       any         //
	BlockLinks    any         //
	AuditDatabase any         //
	AuditGroupId  any         //
	CreatedAt     *gtime.Time //
	UpdatedAt     *gtime.Time //
}
