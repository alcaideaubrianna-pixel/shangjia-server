// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCycleRun is the golang structure for table youban_publish_cycle_run.
type YoubanPublishCycleRun struct {
	Id           int64       `json:"id"           orm:"id"            description:""`
	PlanId       int64       `json:"planId"       orm:"plan_id"       description:""`
	TenantId     int64       `json:"tenantId"     orm:"tenant_id"     description:""`
	AccountId    int64       `json:"accountId"    orm:"account_id"    description:""`
	ProfileId    int64       `json:"profileId"    orm:"profile_id"    description:""`
	TaskId       int64       `json:"taskId"       orm:"task_id"       description:""`
	Status       string      `json:"status"       orm:"status"        description:""`
	Stage        string      `json:"stage"        orm:"stage"         description:""`
	ScheduledAt  *gtime.Time `json:"scheduledAt"  orm:"scheduled_at"  description:""`
	StartedAt    *gtime.Time `json:"startedAt"    orm:"started_at"    description:""`
	FinishedAt   *gtime.Time `json:"finishedAt"   orm:"finished_at"   description:""`
	ErrorMessage string      `json:"errorMessage" orm:"error_message" description:""`
	RetryCount   int         `json:"retryCount"   orm:"retry_count"   description:""`
	CreatedAt    *gtime.Time `json:"createdAt"    orm:"created_at"    description:""`
	UpdatedAt    *gtime.Time `json:"updatedAt"    orm:"updated_at"    description:""`
}
