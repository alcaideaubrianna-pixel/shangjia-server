// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectHistoryLog is the golang structure for table youban_publish_collect_history_log.
type YoubanPublishCollectHistoryLog struct {
	Id        int64       `json:"id"        orm:"id"         description:""`
	TaskId    int64       `json:"taskId"    orm:"task_id"    description:""`
	TenantId  int64       `json:"tenantId"  orm:"tenant_id"  description:""`
	AccountId int64       `json:"accountId" orm:"account_id" description:""`
	Level     string      `json:"level"     orm:"level"      description:""`
	Stage     string      `json:"stage"     orm:"stage"      description:""`
	Message   string      `json:"message"   orm:"message"    description:""`
	MetaJson  string      `json:"metaJson"  orm:"meta_json"  description:""`
	CreatedAt *gtime.Time `json:"createdAt" orm:"created_at" description:""`
}
