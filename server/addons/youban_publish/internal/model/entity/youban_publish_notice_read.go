// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishNoticeRead is the golang structure for table youban_publish_notice_read.
type YoubanPublishNoticeRead struct {
	Id        int64       `json:"id"        orm:"id"         description:""`
	NoticeId  int64       `json:"noticeId"  orm:"notice_id"  description:""`
	AccountId int64       `json:"accountId" orm:"account_id" description:""`
	Clicks    int         `json:"clicks"    orm:"clicks"     description:""`
	CreatedAt *gtime.Time `json:"createdAt" orm:"created_at" description:""`
	UpdatedAt *gtime.Time `json:"updatedAt" orm:"updated_at" description:""`
}
