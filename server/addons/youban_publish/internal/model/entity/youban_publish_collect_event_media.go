// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectEventMedia is the golang structure for table youban_publish_collect_event_media.
type YoubanPublishCollectEventMedia struct {
	Id               int64       `json:"id"               orm:"id"                 description:""`
	TenantId         int64       `json:"tenantId"         orm:"tenant_id"          description:""`
	AccountId        int64       `json:"accountId"        orm:"account_id"         description:""`
	SourceId         int64       `json:"sourceId"         orm:"source_id"          description:""`
	SourceType       string      `json:"sourceType"       orm:"source_type"        description:""`
	EventId          int64       `json:"eventId"          orm:"event_id"           description:""`
	SourceChatId     string      `json:"sourceChatId"     orm:"source_chat_id"     description:""`
	SourceMessageId  int64       `json:"sourceMessageId"  orm:"source_message_id"  description:""`
	SourceGroupedId  string      `json:"sourceGroupedId"  orm:"source_grouped_id"  description:""`
	SourceMediaKey   string      `json:"sourceMediaKey"   orm:"source_media_key"   description:""`
	MediaType        string      `json:"mediaType"        orm:"media_type"         description:""`
	SourceRefType    string      `json:"sourceRefType"    orm:"source_ref_type"    description:""`
	SourceFileId     string      `json:"sourceFileId"     orm:"source_file_id"     description:""`
	SourceMessageRef string      `json:"sourceMessageRef" orm:"source_message_ref" description:""`
	BackupChannelId  int64       `json:"backupChannelId"  orm:"backup_channel_id"  description:""`
	BackupChatId     string      `json:"backupChatId"     orm:"backup_chat_id"     description:""`
	BackupMessageId  int64       `json:"backupMessageId"  orm:"backup_message_id"  description:""`
	FileUrl          string      `json:"fileUrl"          orm:"file_url"           description:""`
	StoragePath      string      `json:"storagePath"      orm:"storage_path"       description:""`
	PosterUrl        string      `json:"posterUrl"        orm:"poster_url"         description:""`
	MetaJson         string      `json:"metaJson"         orm:"meta_json"          description:""`
	SortIndex        int         `json:"sortIndex"        orm:"sort_index"         description:""`
	CacheStatus      string      `json:"cacheStatus"      orm:"cache_status"       description:""`
	ErrorMessage     string      `json:"errorMessage"     orm:"error_message"      description:""`
	CreatedAt        *gtime.Time `json:"createdAt"        orm:"created_at"         description:""`
	UpdatedAt        *gtime.Time `json:"updatedAt"        orm:"updated_at"         description:""`
}
