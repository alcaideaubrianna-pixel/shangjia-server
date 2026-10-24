// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishDailyReport is the golang structure of table hg_youban_publish_daily_report for DAO operations like Where/Data.
type YoubanPublishDailyReport struct {
	g.Meta              `orm:"table:hg_youban_publish_daily_report, do:true"`
	Id                  any         //
	TenantId            any         //
	AccountId           any         //
	ReportDate          *gtime.Time //
	CollectedTotal      any         //
	PublishSuccess      any         //
	PublishFailed       any         //
	PublishProcessing   any         //
	SourceBreakdownJson any         //
	GeneratedAt         *gtime.Time //
	NotifyStatus        any         //
	NotifyRetryCount    any         //
	NotifyNextRetryAt   *gtime.Time //
	NotifyLeaseUntil    *gtime.Time //
	NotifyToken         any         //
	NotifyCursor        any         //
	NotifyError         any         //
}
