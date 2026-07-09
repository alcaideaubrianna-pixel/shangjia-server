// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectEventLog is the golang structure of table hg_youban_publish_collect_event_log for DAO operations like Where/Data.
type YoubanPublishCollectEventLog struct {
	g.Meta     `orm:"table:hg_youban_publish_collect_event_log, do:true"`
	Id         any         //
	TenantId   any         //
	AccountId  any         //
	EventId    any         //
	DispatchId any         //
	Stage      any         //
	Status     any         //
	Message    any         //
	MetaText   any         //
	CreatedAt  *gtime.Time //
}
