// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTask is the golang structure of table hg_youban_publish_task for DAO operations like Where/Data.
type YoubanPublishTask struct {
	g.Meta          `orm:"table:hg_youban_publish_task, do:true"`
	Id              any         //
	TenantId        any         //
	MerchantId      any         //
	AccountId       any         //
	ProfileId       any         //
	ClientRequestId any         //
	Title           any         //
	Province        any         //
	City            any         //
	PlainText       any         //
	MediaCount      any         //
	TgPushEnabled   any         //
	TgStatus        any         //
	Status          any         //
	ErrorMessage    any         //
	SubmittedAt     *gtime.Time //
	PublishedAt     *gtime.Time //
	CreatedBy       any         //
	UpdatedBy       any         //
	DeletedBy       any         //
	CreatedAt       *gtime.Time //
	UpdatedAt       *gtime.Time //
	DeletedAt       *gtime.Time //
	ChannelIdJson   any         //
	CustomerRemark  any         //
	AntiScanEnabled any         //
	TgOperationNo   any         //
}
