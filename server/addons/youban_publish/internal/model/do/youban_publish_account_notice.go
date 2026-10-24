// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishAccountNotice is the golang structure of table hg_youban_publish_account_notice for DAO operations like Where/Data.
type YoubanPublishAccountNotice struct {
	g.Meta       `orm:"table:hg_youban_publish_account_notice, do:true"`
	Id           any         //
	TenantId     any         //
	AccountId    any         //
	EventKey     any         //
	Category     any         //
	TextContent  any         //
	Status       any         //
	Attempts     any         //
	NextRetryAt  *gtime.Time //
	LeaseUntil   *gtime.Time //
	LeaseToken   any         //
	ErrorMessage any         //
	CreatedAt    *gtime.Time //
	UpdatedAt    *gtime.Time //
}
