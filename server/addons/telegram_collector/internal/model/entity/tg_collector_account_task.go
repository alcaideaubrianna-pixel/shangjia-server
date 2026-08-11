// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// TgCollectorAccountTask is the golang structure for table tg_collector_account_task.
type TgCollectorAccountTask struct {
	Id                  int64       `json:"id"                  orm:"id"                     description:""`
	TenantId            int64       `json:"tenantId"            orm:"tenant_id"              description:""`
	AccountId           int64       `json:"accountId"           orm:"account_id"             description:""`
	TaskType            string      `json:"taskType"            orm:"task_type"              description:""`
	TaskKey             string      `json:"taskKey"             orm:"task_key"               description:""`
	Priority            int         `json:"priority"            orm:"priority"               description:""`
	Status              string      `json:"status"              orm:"status"                 description:""`
	AttemptCount        int         `json:"attemptCount"        orm:"attempt_count"          description:""`
	MaxAttempts         int         `json:"maxAttempts"         orm:"max_attempts"           description:""`
	NextRunAt           *gtime.Time `json:"nextRunAt"           orm:"next_run_at"            description:""`
	LeaseOwner          string      `json:"leaseOwner"          orm:"lease_owner"            description:""`
	LeaseEpoch          int64       `json:"leaseEpoch"          orm:"lease_epoch"            description:""`
	LeaseUntil          *gtime.Time `json:"leaseUntil"          orm:"lease_until"            description:""`
	ErrorMessage        string      `json:"errorMessage"        orm:"error_message"          description:""`
	CompletedAt         *gtime.Time `json:"completedAt"         orm:"completed_at"           description:""`
	CreatedAt           *gtime.Time `json:"createdAt"           orm:"created_at"             description:""`
	UpdatedAt           *gtime.Time `json:"updatedAt"           orm:"updated_at"             description:""`
	HistoryTaskId       int64       `json:"historyTaskId"       orm:"history_task_id"        description:""`
	MediaOwnerAccountId int64       `json:"mediaOwnerAccountId" orm:"media_owner_account_id" description:""`
	MediaType           string      `json:"mediaType"           orm:"media_type"             description:""`
	MediaPurpose        string      `json:"mediaPurpose"        orm:"media_purpose"          description:""`
	SourceFileId        string      `json:"sourceFileId"        orm:"source_file_id"         description:""`
	FileUrl             string      `json:"fileUrl"             orm:"file_url"               description:""`
	StoragePath         string      `json:"storagePath"         orm:"storage_path"           description:""`
	PosterUrl           string      `json:"posterUrl"           orm:"poster_url"             description:""`
	FileMd5             string      `json:"fileMd5"             orm:"file_md5"               description:""`
	FilePhash           string      `json:"filePhash"           orm:"file_phash"             description:""`
	SourceKind          string      `json:"sourceKind"          orm:"source_kind"            description:""`
	SourceMediaId       int64       `json:"sourceMediaId"       orm:"source_media_id"        description:""`
	SourceAccessHash    int64       `json:"sourceAccessHash"    orm:"source_access_hash"     description:""`
	SourceFileReference string      `json:"sourceFileReference" orm:"source_file_reference"  description:""`
	SourceThumbSize     string      `json:"sourceThumbSize"     orm:"source_thumb_size"      description:""`
	SourceMimeType      string      `json:"sourceMimeType"      orm:"source_mime_type"       description:""`
	SourceDcId          int         `json:"sourceDcId"          orm:"source_dc_id"           description:""`
	SourceSize          int64       `json:"sourceSize"          orm:"source_size"            description:""`
	DebugMetaText       string      `json:"debugMetaText"       orm:"debug_meta_text"        description:""`
	AttachmentId        int64       `json:"attachmentId"        orm:"attachment_id"          description:""`
	ResultErrorCode     string      `json:"resultErrorCode"     orm:"result_error_code"      description:""`
}
