// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTag is the golang structure for table youban_publish_tag.
type YoubanPublishTag struct {
	Id           uint64      `json:"id"           orm:"id"            description:"主键"`
	Name         string      `json:"name"         orm:"name"          description:"标签名称"`
	ReviewStatus string      `json:"reviewStatus" orm:"review_status" description:"审核状态"`
	Status       int         `json:"status"       orm:"status"        description:"状态"`
	UseCount     int         `json:"useCount"     orm:"use_count"     description:"使用数量"`
	CreatedBy    int64       `json:"createdBy"    orm:"created_by"    description:"创建人"`
	UpdatedBy    int64       `json:"updatedBy"    orm:"updated_by"    description:"更新人"`
	DeletedBy    int64       `json:"deletedBy"    orm:"deleted_by"    description:"删除人"`
	CreatedAt    *gtime.Time `json:"createdAt"    orm:"created_at"    description:"创建时间"`
	UpdatedAt    *gtime.Time `json:"updatedAt"    orm:"updated_at"    description:"更新时间"`
	DeletedAt    *gtime.Time `json:"deletedAt"    orm:"deleted_at"    description:"删除时间"`
}
