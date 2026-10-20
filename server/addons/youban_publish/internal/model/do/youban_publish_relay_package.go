// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishRelayPackage is the golang structure of table hg_youban_publish_relay_package for DAO operations like Where/Data.
type YoubanPublishRelayPackage struct {
	g.Meta         `orm:"table:hg_youban_publish_relay_package, do:true"`
	Id             any         //
	TenantId       any         //
	JobId          any         //
	BotId          any         //
	TelegramUserId any         //
	RelayChannelId any         //
	Generation     any         //
	Status         any         //
	Payload        any         //
	Messages       any         //
	AttemptId      any         //
	TaskId         any         //
	LeaseUntil     *gtime.Time //
	NextRetryAt    *gtime.Time //
	LastError      any         //
	CreatedAt      *gtime.Time //
	UpdatedAt      *gtime.Time //
}
