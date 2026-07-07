// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishMedia is the golang structure for table youban_publish_media.
type YoubanPublishMedia struct {
	Id                   int64       `json:"id"                   orm:"id"                     description:""`
	TenantId             int64       `json:"tenantId"             orm:"tenant_id"              description:""`
	MerchantId           int64       `json:"merchantId"           orm:"merchant_id"            description:""`
	AccountId            int64       `json:"accountId"            orm:"account_id"             description:""`
	TaskId               int64       `json:"taskId"               orm:"task_id"                description:""`
	ProfileId            int64       `json:"profileId"            orm:"profile_id"             description:""`
	AttachmentId         int64       `json:"attachmentId"         orm:"attachment_id"          description:""`
	MediaType            string      `json:"mediaType"            orm:"media_type"             description:""`
	Name                 string      `json:"name"                 orm:"name"                   description:""`
	FileUrl              string      `json:"fileUrl"              orm:"file_url"               description:""`
	StoragePath          string      `json:"storagePath"          orm:"storage_path"           description:""`
	MimeType             string      `json:"mimeType"             orm:"mime_type"              description:""`
	Md5                  string      `json:"md5"                  orm:"md5"                    description:""`
	Size                 int64       `json:"size"                 orm:"size"                   description:""`
	SortIndex            int         `json:"sortIndex"            orm:"sort_index"             description:""`
	Status               int         `json:"status"               orm:"status"                 description:""`
	CreatedBy            int64       `json:"createdBy"            orm:"created_by"             description:""`
	UpdatedBy            int64       `json:"updatedBy"            orm:"updated_by"             description:""`
	DeletedBy            int64       `json:"deletedBy"            orm:"deleted_by"             description:""`
	CreatedAt            *gtime.Time `json:"createdAt"            orm:"created_at"             description:""`
	UpdatedAt            *gtime.Time `json:"updatedAt"            orm:"updated_at"             description:""`
	DeletedAt            *gtime.Time `json:"deletedAt"            orm:"deleted_at"             description:""`
	PerceptualHash       string      `json:"perceptualHash"       orm:"perceptual_hash"        description:""`
	Purpose              string      `json:"purpose"              orm:"purpose"                description:""`
	PosterUrl            string      `json:"posterUrl"            orm:"poster_url"             description:""`
	TgFileId             string      `json:"tgFileId"             orm:"tg_file_id"             description:""`
	TgThumbFileId        string      `json:"tgThumbFileId"        orm:"tg_thumb_file_id"       description:""`
	PosterStoragePath    string      `json:"posterStoragePath"    orm:"poster_storage_path"    description:""`
	OriginalAttachmentId int64       `json:"originalAttachmentId" orm:"original_attachment_id" description:""`
	OriginalFileUrl      string      `json:"originalFileUrl"      orm:"original_file_url"      description:""`
	OriginalStoragePath  string      `json:"originalStoragePath"  orm:"original_storage_path"  description:""`
	EditedAttachmentId   int64       `json:"editedAttachmentId"   orm:"edited_attachment_id"   description:""`
	EditedFileUrl        string      `json:"editedFileUrl"        orm:"edited_file_url"        description:""`
	EditedStoragePath    string      `json:"editedStoragePath"    orm:"edited_storage_path"    description:""`
	EditConfigJson       string      `json:"editConfigJson"       orm:"edit_config_json"       description:""`
	EditStatus           string      `json:"editStatus"           orm:"edit_status"            description:""`
	TgCacheAssetHash     string      `json:"tgCacheAssetHash"     orm:"tg_cache_asset_hash"    description:""`
	TgCacheStatus        string      `json:"tgCacheStatus"        orm:"tg_cache_status"        description:""`
}
