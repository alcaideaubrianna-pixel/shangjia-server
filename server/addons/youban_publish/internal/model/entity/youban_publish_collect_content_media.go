// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectContentMedia is the golang structure for table youban_publish_collect_content_media.
type YoubanPublishCollectContentMedia struct {
	Id              int64       `json:"id"              orm:"id"                description:""`
	TenantId        int64       `json:"tenantId"        orm:"tenant_id"         description:""`
	AccountId       int64       `json:"accountId"       orm:"account_id"        description:""`
	ContentId       int64       `json:"contentId"       orm:"content_id"        description:""`
	MediaType       string      `json:"mediaType"       orm:"media_type"        description:""`
	SourceFileId    string      `json:"sourceFileId"    orm:"source_file_id"    description:""`
	SourceUniqueKey string      `json:"sourceUniqueKey" orm:"source_unique_key" description:""`
	FileMd5         string      `json:"fileMd5"         orm:"file_md5"          description:""`
	FilePhash       string      `json:"filePhash"       orm:"file_phash"        description:""`
	SortIndex       int         `json:"sortIndex"       orm:"sort_index"        description:""`
	Status          string      `json:"status"          orm:"status"            description:""`
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"        description:""`
	UpdatedAt       *gtime.Time `json:"updatedAt"       orm:"updated_at"        description:""`
}
