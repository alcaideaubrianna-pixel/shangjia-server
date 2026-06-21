// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// ContentMedia is the golang structure for table content_media.
type ContentMedia struct {
	Id                  int64       `json:"id"                  orm:"id"                    description:"ID"`
	ProfileId           int64       `json:"profileId"           orm:"profile_id"            description:"资料ID"`
	SourceAssetId       int64       `json:"sourceAssetId"       orm:"source_asset_id"       description:"FeiNiu资源ID"`
	DuplicateOfMediaId  int64       `json:"duplicateOfMediaId"  orm:"duplicate_of_media_id" description:"重复媒体ID"`
	MediaType           string      `json:"mediaType"           orm:"media_type"            description:"媒体类型"`
	SortIndex           int         `json:"sortIndex"           orm:"sort_index"            description:"排序"`
	OriginalStoragePath string      `json:"originalStoragePath" orm:"original_storage_path" description:"原始存储路径"`
	DisplayStoragePath  string      `json:"displayStoragePath"  orm:"display_storage_path"  description:"展示存储路径"`
	PreviewStoragePath  string      `json:"previewStoragePath"  orm:"preview_storage_path"  description:"预览存储路径"`
	BinaryMd5           string      `json:"binaryMd5"           orm:"binary_md5"            description:"文件MD5"`
	PerceptualHash      string      `json:"perceptualHash"      orm:"perceptual_hash"       description:"感知哈希"`
	Width               int         `json:"width"               orm:"width"                 description:"宽度"`
	Height              int         `json:"height"              orm:"height"                description:"高度"`
	Duration            int         `json:"duration"            orm:"duration"              description:"时长"`
	ProcessStatus       string      `json:"processStatus"       orm:"process_status"        description:"处理状态"`
	EncryptStatus       string      `json:"encryptStatus"       orm:"encrypt_status"        description:"加密状态"`
	Status              int         `json:"status"              orm:"status"                description:"状态"`
	CreatedAt           *gtime.Time `json:"createdAt"           orm:"created_at"            description:"创建时间"`
	UpdatedAt           *gtime.Time `json:"updatedAt"           orm:"updated_at"            description:"更新时间"`
	DeletedAt           *gtime.Time `json:"deletedAt"           orm:"deleted_at"            description:"删除时间"`
}
