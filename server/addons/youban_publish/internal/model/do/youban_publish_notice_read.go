// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishNoticeRead is the golang structure of table hg_youban_publish_notice_read for DAO operations like Where/Data.
type YoubanPublishNoticeRead struct {
	g.Meta    `orm:"table:hg_youban_publish_notice_read, do:true"`
	Id        any         //
	NoticeId  any         //
	AccountId any         //
	Clicks    any         //
	CreatedAt *gtime.Time //
	UpdatedAt *gtime.Time //
}
