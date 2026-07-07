// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCyclePlan is the golang structure for table youban_publish_cycle_plan.
type YoubanPublishCyclePlan struct {
	Id               int64       `json:"id"               orm:"id"                 description:""`
	TenantId         int64       `json:"tenantId"         orm:"tenant_id"          description:""`
	AccountId        int64       `json:"accountId"        orm:"account_id"         description:""`
	ProfileId        int64       `json:"profileId"        orm:"profile_id"         description:""`
	TaskId           int64       `json:"taskId"           orm:"task_id"            description:""`
	Enabled          int         `json:"enabled"          orm:"enabled"            description:""`
	IntervalSeconds  int         `json:"intervalSeconds"  orm:"interval_seconds"   description:""`
	PublishTime      string      `json:"publishTime"      orm:"publish_time"       description:""`
	NextRunAt        *gtime.Time `json:"nextRunAt"        orm:"next_run_at"        description:""`
	LastRunAt        *gtime.Time `json:"lastRunAt"        orm:"last_run_at"        description:""`
	LastRunId        int64       `json:"lastRunId"        orm:"last_run_id"        description:""`
	Status           string      `json:"status"           orm:"status"             description:""`
	Source           string      `json:"source"           orm:"source"             description:""`
	LockedAt         *gtime.Time `json:"lockedAt"         orm:"locked_at"          description:""`
	LastErrorMessage string      `json:"lastErrorMessage" orm:"last_error_message" description:""`
	CreatedAt        *gtime.Time `json:"createdAt"        orm:"created_at"         description:""`
	UpdatedAt        *gtime.Time `json:"updatedAt"        orm:"updated_at"         description:""`
	DeletedAt        *gtime.Time `json:"deletedAt"        orm:"deleted_at"         description:""`
}
