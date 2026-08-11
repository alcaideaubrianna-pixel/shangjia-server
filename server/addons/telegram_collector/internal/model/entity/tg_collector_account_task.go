// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/os/gtime"
)

// TgCollectorAccountTask is the golang structure for table tg_collector_account_task.
type TgCollectorAccountTask struct {
	Id           int64       `json:"id"           orm:"id"            description:""`
	TenantId     int64       `json:"tenantId"     orm:"tenant_id"     description:""`
	AccountId    int64       `json:"accountId"    orm:"account_id"    description:""`
	TaskType     string      `json:"taskType"     orm:"task_type"     description:""`
	TaskKey      string      `json:"taskKey"      orm:"task_key"      description:""`
	Priority     int         `json:"priority"     orm:"priority"      description:""`
	Status       string      `json:"status"       orm:"status"        description:""`
	Payload      *gjson.Json `json:"payload"      orm:"payload"       description:""`
	Result       *gjson.Json `json:"result"       orm:"result"        description:""`
	AttemptCount int         `json:"attemptCount" orm:"attempt_count" description:""`
	MaxAttempts  int         `json:"maxAttempts"  orm:"max_attempts"  description:""`
	NextRunAt    *gtime.Time `json:"nextRunAt"    orm:"next_run_at"   description:""`
	LeaseOwner   string      `json:"leaseOwner"   orm:"lease_owner"   description:""`
	LeaseEpoch   int64       `json:"leaseEpoch"   orm:"lease_epoch"   description:""`
	LeaseUntil   *gtime.Time `json:"leaseUntil"   orm:"lease_until"   description:""`
	ErrorMessage string      `json:"errorMessage" orm:"error_message" description:""`
	CompletedAt  *gtime.Time `json:"completedAt"  orm:"completed_at"  description:""`
	CreatedAt    *gtime.Time `json:"createdAt"    orm:"created_at"    description:""`
	UpdatedAt    *gtime.Time `json:"updatedAt"    orm:"updated_at"    description:""`
}
