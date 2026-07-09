// =================================================================================
// Code generated and maintained by GoFrame CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/gogf/gf/v2/os/gtime"
)

// YoubanPublishCollectEventLog is the golang structure for table youban_publish_collect_event_log.
type YoubanPublishCollectEventLog struct {
	Id         int64       `json:"id"         orm:"id"          description:""`
	TenantId   int64       `json:"tenantId"   orm:"tenant_id"   description:""`
	AccountId  int64       `json:"accountId"  orm:"account_id"  description:""`
	EventId    int64       `json:"eventId"    orm:"event_id"    description:""`
	DispatchId int64       `json:"dispatchId" orm:"dispatch_id" description:""`
	Stage      string      `json:"stage"      orm:"stage"       description:""`
	Status     string      `json:"status"     orm:"status"      description:""`
	Message    string      `json:"message"    orm:"message"     description:""`
	MetaText   string      `json:"metaText"   orm:"meta_text"   description:""`
	CreatedAt  *gtime.Time `json:"createdAt"  orm:"created_at"  description:""`
}
