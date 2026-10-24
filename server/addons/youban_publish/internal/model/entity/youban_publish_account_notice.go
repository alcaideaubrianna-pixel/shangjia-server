// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishAccountNotice is the golang structure for table youban_publish_account_notice.
type YoubanPublishAccountNotice struct {
	Id           int64       `json:"id"           orm:"id"            description:""`
	TenantId     int64       `json:"tenantId"     orm:"tenant_id"     description:""`
	AccountId    int64       `json:"accountId"    orm:"account_id"    description:""`
	EventKey     string      `json:"eventKey"     orm:"event_key"     description:""`
	Category     string      `json:"category"     orm:"category"      description:""`
	TextContent  string      `json:"textContent"  orm:"text_content"  description:""`
	Status       string      `json:"status"       orm:"status"        description:""`
	Attempts     int         `json:"attempts"     orm:"attempts"      description:""`
	NextRetryAt  *gtime.Time `json:"nextRetryAt"  orm:"next_retry_at" description:""`
	LeaseUntil   *gtime.Time `json:"leaseUntil"   orm:"lease_until"   description:""`
	LeaseToken   string      `json:"leaseToken"   orm:"lease_token"   description:""`
	ErrorMessage string      `json:"errorMessage" orm:"error_message" description:""`
	CreatedAt    *gtime.Time `json:"createdAt"    orm:"created_at"    description:""`
	UpdatedAt    *gtime.Time `json:"updatedAt"    orm:"updated_at"    description:""`
}
