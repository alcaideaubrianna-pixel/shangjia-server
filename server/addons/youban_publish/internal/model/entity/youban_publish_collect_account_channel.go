// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectAccountChannel is the golang structure for table youban_publish_collect_account_channel.
type YoubanPublishCollectAccountChannel struct {
	Id             int64       `json:"id"             orm:"id"              description:""`
	TenantId       int64       `json:"tenantId"       orm:"tenant_id"       description:""`
	AccountId      int64       `json:"accountId"      orm:"account_id"      description:""`
	ChannelId      int64       `json:"channelId"      orm:"channel_id"      description:""`
	SourceId       int64       `json:"sourceId"       orm:"source_id"       description:""`
	CollectEnabled int         `json:"collectEnabled" orm:"collect_enabled" description:""`
	Status         string      `json:"status"         orm:"status"          description:""`
	ApprovalStatus string      `json:"approvalStatus" orm:"approval_status" description:""`
	ApprovedAt     *gtime.Time `json:"approvedAt"     orm:"approved_at"     description:""`
	DisabledAt     *gtime.Time `json:"disabledAt"     orm:"disabled_at"     description:""`
	DisabledReason string      `json:"disabledReason" orm:"disabled_reason" description:""`
	CreatedAt      *gtime.Time `json:"createdAt"      orm:"created_at"      description:""`
	UpdatedAt      *gtime.Time `json:"updatedAt"      orm:"updated_at"      description:""`
	DeletedAt      *gtime.Time `json:"deletedAt"      orm:"deleted_at"      description:""`
}
