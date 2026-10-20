// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishRelayChannel is the golang structure of table hg_youban_publish_relay_channel for DAO operations like Where/Data.
type YoubanPublishRelayChannel struct {
	g.Meta              `orm:"table:hg_youban_publish_relay_channel, do:true"`
	Id                  any         //
	TenantId            any         //
	TgAccountId         any         //
	ChannelId           any         //
	TargetChatId        any         //
	Title               any         //
	Status              any         //
	HealthStatus        any         //
	BotPermissionStatus any         //
	CooldownUntil       *gtime.Time //
	LastError           any         //
	InFlightCount       any         //
	LastUsedAt          *gtime.Time //
	CreatedAt           *gtime.Time //
	UpdatedAt           *gtime.Time //
	DeletedAt           *gtime.Time //
	AccessHash          any         //
	BotId               any         //
	Capacity            any         //
	OperationKind       any         //
	OperationStatus     any         //
	TaskId              any         //
}
