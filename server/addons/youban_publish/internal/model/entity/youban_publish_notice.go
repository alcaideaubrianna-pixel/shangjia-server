// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishNotice is the golang structure for table youban_publish_notice.
type YoubanPublishNotice struct {
	Id        int64       `json:"id"        orm:"id"         description:""`
	Type      int         `json:"type"      orm:"type"       description:""`
	Title     string      `json:"title"     orm:"title"      description:""`
	Content   string      `json:"content"   orm:"content"    description:""`
	Tag       int64       `json:"tag"       orm:"tag"        description:""`
	Receiver  string      `json:"receiver"  orm:"receiver"   description:""`
	Remark    string      `json:"remark"    orm:"remark"     description:""`
	Sort      int64       `json:"sort"      orm:"sort"       description:""`
	Status    int         `json:"status"    orm:"status"     description:""`
	PublishAt *gtime.Time `json:"publishAt" orm:"publish_at" description:""`
	ExpireAt  *gtime.Time `json:"expireAt"  orm:"expire_at"  description:""`
	CreatedBy int64       `json:"createdBy" orm:"created_by" description:""`
	UpdatedBy int64       `json:"updatedBy" orm:"updated_by" description:""`
	DeletedBy int64       `json:"deletedBy" orm:"deleted_by" description:""`
	CreatedAt *gtime.Time `json:"createdAt" orm:"created_at" description:""`
	UpdatedAt *gtime.Time `json:"updatedAt" orm:"updated_at" description:""`
	DeletedAt *gtime.Time `json:"deletedAt" orm:"deleted_at" description:""`
}
