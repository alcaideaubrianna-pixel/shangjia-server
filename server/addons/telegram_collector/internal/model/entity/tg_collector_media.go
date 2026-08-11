// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// TgCollectorMedia is the golang structure for table tg_collector_media.
type TgCollectorMedia struct {
	Id                int64       `json:"id"                orm:"id"                  description:""`
	TenantId          int64       `json:"tenantId"          orm:"tenant_id"           description:""`
	Fingerprint       string      `json:"fingerprint"       orm:"fingerprint"         description:""`
	Kind              string      `json:"kind"              orm:"kind"                description:""`
	MimeType          string      `json:"mimeType"          orm:"mime_type"           description:""`
	Size              int64       `json:"size"              orm:"size"                description:""`
	PipelineVersion   string      `json:"pipelineVersion"   orm:"pipeline_version"    description:""`
	Status            string      `json:"status"            orm:"status"              description:""`
	StoragePath       string      `json:"storagePath"       orm:"storage_path"        description:""`
	PosterStoragePath string      `json:"posterStoragePath" orm:"poster_storage_path" description:""`
	Phash             string      `json:"phash"             orm:"phash"               description:""`
	Dhash             string      `json:"dhash"             orm:"dhash"               description:""`
	AttemptCount      int         `json:"attemptCount"      orm:"attempt_count"       description:""`
	NextRunAt         *gtime.Time `json:"nextRunAt"         orm:"next_run_at"         description:""`
	LeaseOwner        string      `json:"leaseOwner"        orm:"lease_owner"         description:""`
	LeaseUntil        *gtime.Time `json:"leaseUntil"        orm:"lease_until"         description:""`
	ErrorMessage      string      `json:"errorMessage"      orm:"error_message"       description:""`
	CreatedAt         *gtime.Time `json:"createdAt"         orm:"created_at"          description:""`
	UpdatedAt         *gtime.Time `json:"updatedAt"         orm:"updated_at"          description:""`
}
