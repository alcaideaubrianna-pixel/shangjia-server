// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishMedia is the golang structure for table youban_publish_media.
type YoubanPublishMedia struct {
	Id           uint64      `json:"id"           orm:"id"            description:"主键"`
	MerchantId   int64       `json:"merchantId"   orm:"merchant_id"   description:"商家ID"`
	AccountId    int64       `json:"accountId"    orm:"account_id"    description:"账号ID"`
	TaskId       int64       `json:"taskId"       orm:"task_id"       description:"任务ID"`
	ProfileId    int64       `json:"profileId"    orm:"profile_id"    description:"资料ID"`
	AttachmentId int64       `json:"attachmentId" orm:"attachment_id" description:"HotGo附件ID"`
	MediaType    string      `json:"mediaType"    orm:"media_type"    description:"媒体类型"`
	Name         string      `json:"name"         orm:"name"          description:"文件名"`
	FileUrl      string      `json:"fileUrl"      orm:"file_url"      description:"访问地址"`
	StoragePath  string      `json:"storagePath"  orm:"storage_path"  description:"存储路径"`
	MimeType     string      `json:"mimeType"     orm:"mime_type"     description:"MIME"`
	Md5          string      `json:"md5"          orm:"md5"           description:"MD5"`
	Size         int64       `json:"size"         orm:"size"          description:"大小"`
	SortIndex    int         `json:"sortIndex"    orm:"sort_index"    description:"排序"`
	Status       int         `json:"status"       orm:"status"        description:"状态"`
	CreatedBy    int64       `json:"createdBy"    orm:"created_by"    description:"创建人"`
	UpdatedBy    int64       `json:"updatedBy"    orm:"updated_by"    description:"更新人"`
	DeletedBy    int64       `json:"deletedBy"    orm:"deleted_by"    description:"删除人"`
	CreatedAt    *gtime.Time `json:"createdAt"    orm:"created_at"    description:"创建时间"`
	UpdatedAt    *gtime.Time `json:"updatedAt"    orm:"updated_at"    description:"更新时间"`
	DeletedAt    *gtime.Time `json:"deletedAt"    orm:"deleted_at"    description:"删除时间"`
}
