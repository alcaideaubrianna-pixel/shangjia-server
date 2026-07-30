// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishNotice is the golang structure of table hg_youban_publish_notice for DAO operations like Where/Data.
type YoubanPublishNotice struct {
	g.Meta    `orm:"table:hg_youban_publish_notice, do:true"`
	Id        any         //
	Type      any         //
	Title     any         //
	Content   any         //
	Tag       any         //
	Receiver  any         //
	Remark    any         //
	Sort      any         //
	Status    any         //
	PublishAt *gtime.Time //
	ExpireAt  *gtime.Time //
	CreatedBy any         //
	UpdatedBy any         //
	DeletedBy any         //
	CreatedAt *gtime.Time //
	UpdatedAt *gtime.Time //
	DeletedAt *gtime.Time //
}
