// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishNoteIndex is the golang structure for table youban_publish_note_index.
type YoubanPublishNoteIndex struct {
	Id              int64       `json:"id"              orm:"id"                description:""`
	TenantId        int64       `json:"tenantId"        orm:"tenant_id"         description:""`
	AccountId       int64       `json:"accountId"       orm:"account_id"        description:""`
	ProfileId       int64       `json:"profileId"       orm:"profile_id"        description:""`
	TaskId          int64       `json:"taskId"          orm:"task_id"           description:""`
	Uuid            string      `json:"uuid"            orm:"uuid"              description:""`
	ProfileNo       string      `json:"profileNo"       orm:"profile_no"        description:""`
	Title           string      `json:"title"           orm:"title"             description:""`
	Summary         string      `json:"summary"         orm:"summary"           description:""`
	PlainText       string      `json:"plainText"       orm:"plain_text"        description:""`
	Tag             string      `json:"tag"             orm:"tag"               description:""`
	Province        string      `json:"province"        orm:"province"          description:""`
	City            string      `json:"city"            orm:"city"              description:""`
	Status          int         `json:"status"          orm:"status"            description:""`
	Visibility      string      `json:"visibility"      orm:"visibility"        description:""`
	ReviewStatus    string      `json:"reviewStatus"    orm:"review_status"     description:""`
	TaskStatus      string      `json:"taskStatus"      orm:"task_status"       description:""`
	CoverMediaId    int64       `json:"coverMediaId"    orm:"cover_media_id"    description:""`
	PublishedAt     *gtime.Time `json:"publishedAt"     orm:"published_at"      description:""`
	SourceUpdatedAt *gtime.Time `json:"sourceUpdatedAt" orm:"source_updated_at" description:""`
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"        description:""`
	UpdatedAt       *gtime.Time `json:"updatedAt"       orm:"updated_at"        description:""`
	DeletedAt       *gtime.Time `json:"deletedAt"       orm:"deleted_at"        description:""`
}
