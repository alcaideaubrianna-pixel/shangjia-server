// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishMessagePushPlan is the golang structure for table youban_publish_message_push_plan.
type YoubanPublishMessagePushPlan struct {
	Id              int64       `json:"id"              orm:"id"               description:""`
	TenantId        int64       `json:"tenantId"        orm:"tenant_id"        description:""`
	Name            string      `json:"name"            orm:"name"             description:""`
	AccountId       int64       `json:"accountId"       orm:"account_id"       description:""`
	TemplateIds     string      `json:"templateIds"     orm:"template_ids"     description:""`
	TargetChatIds   string      `json:"targetChatIds"   orm:"target_chat_ids"  description:""`
	Times           string      `json:"times"           orm:"times"            description:""`
	IntervalSeconds int         `json:"intervalSeconds" orm:"interval_seconds" description:""`
	Status          int         `json:"status"          orm:"status"           description:""`
	NextRunAt       *gtime.Time `json:"nextRunAt"       orm:"next_run_at"      description:""`
	LastRunAt       *gtime.Time `json:"lastRunAt"       orm:"last_run_at"      description:""`
	LastResult      string      `json:"lastResult"      orm:"last_result"      description:""`
	LockedAt        *gtime.Time `json:"lockedAt"        orm:"locked_at"        description:""`
	CreatedBy       int64       `json:"createdBy"       orm:"created_by"       description:""`
	UpdatedBy       int64       `json:"updatedBy"       orm:"updated_by"       description:""`
	DeletedBy       int64       `json:"deletedBy"       orm:"deleted_by"       description:""`
	CreatedAt       *gtime.Time `json:"createdAt"       orm:"created_at"       description:""`
	UpdatedAt       *gtime.Time `json:"updatedAt"       orm:"updated_at"       description:""`
	DeletedAt       *gtime.Time `json:"deletedAt"       orm:"deleted_at"       description:""`
}
