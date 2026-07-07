// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCycleRunLog is the golang structure for table youban_publish_cycle_run_log.
type YoubanPublishCycleRunLog struct {
	Id          int64       `json:"id"          orm:"id"           description:""`
	RunId       int64       `json:"runId"       orm:"run_id"       description:""`
	PlanId      int64       `json:"planId"      orm:"plan_id"      description:""`
	TenantId    int64       `json:"tenantId"    orm:"tenant_id"    description:""`
	AccountId   int64       `json:"accountId"   orm:"account_id"   description:""`
	ProfileId   int64       `json:"profileId"   orm:"profile_id"   description:""`
	Level       string      `json:"level"       orm:"level"        description:""`
	Stage       string      `json:"stage"       orm:"stage"        description:""`
	Message     string      `json:"message"     orm:"message"      description:""`
	ContextJson *gjson.Json `json:"contextJson" orm:"context_json" description:""`
	CreatedAt   *gtime.Time `json:"createdAt"   orm:"created_at"   description:""`
}
