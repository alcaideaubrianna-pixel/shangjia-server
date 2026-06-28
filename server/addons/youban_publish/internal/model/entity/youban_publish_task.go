// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishTask is the golang structure for table youban_publish_task.
type YoubanPublishTask struct {
	Id              int64       `json:"id"              orm:"id"                description:""`
	MerchantId      int64       `json:"merchantId"      orm:"merchant_id"       description:""`
	AccountId       int64       `json:"accountId"       orm:"account_id"        description:""`
	ProfileId       int64       `json:"profileId"       orm:"profile_id"        description:""`
	ClientRequestId string      `json:"clientRequestId" orm:"client_request_id" description:""`
	Title           string      `json:"title"           orm:"title"             description:""`
	Province        string      `json:"province"        orm:"province"          description:""`
	City            string      `json:"city"            orm:"city"              description:""`
	PlainText       string      `json:"plainText"       orm:"plain_text"        description:""`
	MediaCount      int         `json:"mediaCount"      orm:"media_count"       description:""`
	TgPushEnabled   int         `json:"tgPushEnabled"   orm:"tg_push_enabled"   description:""`
	TgStatus        string      `json:"tgStatus"        orm:"tg_status"         description:""`
	Status          string      `json:"status"          orm:"status"            description:""`
	ErrorMessage    string      `json:"errorMessage"    orm:"error_message"     description:""`
	SubmittedAt     *gtime.Time `json:"submittedAt"     orm:"submitted_at"      description:""`
	PublishedAt     *gtime.Time `json:"publishedAt"     orm:"published_at"      description:""`
	CreatedBy       int64       `json:"createdBy"       orm:"created_by"        description:""`
	UpdatedBy       int64       `json:"updatedBy"       orm:"updated_by"        description:""`
	DeletedBy       int64       `json:"deletedBy"       orm:"deleted_by"        description:""`
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"        description:""`
	UpdatedAt       *gtime.Time `json:"updatedAt"       orm:"updated_at"        description:""`
	DeletedAt       *gtime.Time `json:"deletedAt"       orm:"deleted_at"        description:""`
	TenantId        int64       `json:"tenantId"        orm:"tenant_id"         description:""`
}
