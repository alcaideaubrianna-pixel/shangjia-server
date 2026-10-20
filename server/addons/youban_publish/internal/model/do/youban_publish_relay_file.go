// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishRelayFile is the golang structure of table hg_youban_publish_relay_file for DAO operations like Where/Data.
type YoubanPublishRelayFile struct {
	g.Meta         `orm:"table:hg_youban_publish_relay_file, do:true"`
	Id             any         //
	TenantId       any         //
	TelegramUserId any         //
	AssetKey       any         //
	FileId         any         //
	FileUniqueId   any         //
	CreatedAt      *gtime.Time //
	UpdatedAt      *gtime.Time //
}
