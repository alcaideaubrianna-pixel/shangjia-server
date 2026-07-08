// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectHistoryLog is the golang structure of table hg_youban_publish_collect_history_log for DAO operations like Where/Data.
type YoubanPublishCollectHistoryLog struct {
	g.Meta    `orm:"table:hg_youban_publish_collect_history_log, do:true"`
	Id        any         //
	TaskId    any         //
	TenantId  any         //
	AccountId any         //
	Level     any         //
	Stage     any         //
	Message   any         //
	MetaJson  any         //
	CreatedAt *gtime.Time //
}
