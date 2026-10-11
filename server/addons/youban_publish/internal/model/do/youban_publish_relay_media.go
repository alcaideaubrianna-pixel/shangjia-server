// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishRelayMedia is the golang structure of table hg_youban_publish_relay_media for DAO operations like Where/Data.
type YoubanPublishRelayMedia struct {
	g.Meta         `orm:"table:hg_youban_publish_relay_media, do:true"`
	Id             any         //
	TenantId       any         //
	MediaId        any         //
	Purpose        any         //
	MediaVersion   any         //
	RelayChannelId any         //
	RelayChatId    any         //
	MessageId      any         //
	FileId         any         //
	Status         any         //
	LeaseToken     any         //
	LeaseUntil     *gtime.Time //
	LastError      any         //
	ExpiresAt      *gtime.Time //
	CreatedAt      *gtime.Time //
	UpdatedAt      *gtime.Time //
}
